package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/application/cleanup"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/application/news"
	"github.com/predictboss/predictor/esb/internal/config"
	"github.com/predictboss/predictor/esb/internal/infrastructure/clickhouse"
	"github.com/predictboss/predictor/esb/internal/infrastructure/hermes"
	"github.com/predictboss/predictor/esb/internal/infrastructure/postgres"
	"github.com/predictboss/predictor/esb/internal/infrastructure/s3"
	"github.com/predictboss/predictor/esb/internal/presentation/httpapi"
	presentationmcp "github.com/predictboss/predictor/esb/internal/presentation/mcp"
)

func main() {
	if err := runESB(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("ESB stopped with an error (error_type=%T)", err)
		os.Exit(1)
	}
}

func runESB() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := pgxpool.New(ctx, cfg.Connections.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	ch, err := clickhouse.NewClient(cfg.Connections.ClickHouseURL, cfg.Connections.CHDatabase, cfg.Connections.CHUser, cfg.Connections.CHPassword, clickhouse.ClientConfig{
		RequestTimeout: cfg.ClickHouse.RequestTimeout, MaxResponseBytes: cfg.ClickHouse.MaxResponse,
	})
	if err != nil {
		return err
	}
	if err := ch.Ping(ctx); err != nil {
		return fmt.Errorf("ping ClickHouse: %w", err)
	}
	profileStore, err := s3.NewStore(ctx, s3.Config{
		Bucket: cfg.Connections.S3Bucket, Region: cfg.Connections.S3Region, Endpoint: cfg.Connections.S3Endpoint,
		AccessKey: cfg.Connections.S3AccessKey, SecretKey: cfg.Connections.S3SecretKey,
		PathStyle: cfg.Connections.S3PathStyle, CreateBucket: cfg.Connections.S3CreateBucket, MaxProfileBytes: cfg.Payload.MaxProfileBytes,
	})
	if err != nil {
		return err
	}
	callRepository := postgres.NewCalls(db)
	if err := callRepository.Recover(ctx); err != nil {
		return fmt.Errorf("recover PostgreSQL calls: %w", err)
	}
	metricSettings := postgres.NewMetricSettings(db)
	analyticsRepository := clickhouse.NewAnalytics(ch, cfg.Pagination.CompanyReportFactsLimit, cfg.Pagination.CompanyReportAnalysisLimit)
	analyticsService := analytics.NewService(analyticsRepository, metricSettings, analytics.Limits{
		FactsDefaultPageSize: cfg.Pagination.FactsDefaultPageSize, FactsMaxPageSize: cfg.Pagination.FactsMaxPageSize,
		FactsMaxPeriod:              time.Duration(cfg.Pagination.FactsMaxPeriodDays) * 24 * time.Hour,
		CompanyItemsDefaultPageSize: cfg.Pagination.CompanyItemsDefaultPageSize,
		CompanyItemsMaxPageSize:     cfg.Pagination.CompanyItemsMaxPageSize,
		ConsultantDefaultPageSize:   cfg.Pagination.ConsultantDefaultPageSize,
		ConsultantMaxPageSize:       cfg.Pagination.ConsultantMaxPageSize,
		CompanyReportDefaultWindow:  time.Duration(cfg.Pagination.CompanyReportDefaultWindowDays) * 24 * time.Hour,
		MaxFactsPerNews:             cfg.Payload.MaxFactsPerNews, MaxFactCompanyLinks: cfg.Payload.MaxFactCompanyLinks,
		MaxMetricObservations: cfg.Payload.MaxMetricObservations, MaxCompanyMetrics: cfg.Payload.MaxCompanyMetricsPerRequest,
	})
	hermesClient, err := hermes.NewClient(cfg.Connections.HermesAPIKey, hermes.ClientConfig{
		RequestTimeout: cfg.Hermes.RequestTimeout, MaxAttempts: cfg.Hermes.MaxAttempts,
		RetryBaseDelay: cfg.Hermes.RetryBaseDelay, MaxResponseBytes: cfg.Hermes.MaxResponse,
	})
	if err != nil {
		return fmt.Errorf("configure Hermes client: %w", err)
	}
	var runningStaleAfter time.Duration
	var activeMaxRetries int
	if cfg.Cleanup.Enabled {
		runningStaleAfter = cfg.Cleanup.RunningStaleAfter
		activeMaxRetries = cfg.Cleanup.ActiveMaxRetries
	}
	completionSignal := make(chan struct{}, 1)
	callService := calls.NewService(callRepository, hermesClient, cfg.Agents, calls.Config{
		PollInterval: cfg.Queue.HermesPollInterval, DispatchBatchSize: cfg.Queue.DispatchBatchSize,
		ContextCompactionTimeout:         cfg.Queue.ContextCompactionTimeout,
		ValeraProfileSyncMaxRetries:      cfg.Queue.ValeraProfileSyncRetries,
		RunningStaleAfter:                runningStaleAfter,
		ActiveMaxRetries:                 activeMaxRetries,
		KonstantinHistoryDefaultPageSize: cfg.Pagination.KonstantinHistoryDefaultPageSize,
		KonstantinHistoryMaxPageSize:     cfg.Pagination.KonstantinHistoryMaxPageSize,
		KonstantinDefaultWindow:          time.Duration(cfg.Pagination.KonstantinDefaultWindowDays) * 24 * time.Hour,
		KonstantinMaxWindow:              time.Duration(cfg.Pagination.KonstantinMaxWindowDays) * 24 * time.Hour,
		KonstantinEventPollInterval:      cfg.WebSocket.EventPollInterval, MaxQuestionBytes: cfg.Payload.MaxQuestionBytes,
		CompletionSignal: completionSignal,
	}, analyticsService)
	cleanupService := cleanup.NewService(callService, callRepository, analyticsRepository, cleanup.Config{
		Enabled: cfg.Cleanup.Enabled, StatusCheckInterval: cfg.Cleanup.StatusCheckInterval,
		RetentionCheckInterval: cfg.Cleanup.RetentionCheckInterval, BatchSize: cfg.Cleanup.BatchSize,
		QueuedStaleAfter: cfg.Cleanup.QueuedStaleAfter, RunningStaleAfter: cfg.Cleanup.RunningStaleAfter,
		ActiveMaxRetries: cfg.Cleanup.ActiveMaxRetries, CallPayloadRetention: cfg.Cleanup.CallPayloadRetention,
		CallRecordRetention: cfg.Cleanup.CallRecordRetention, ContextSummaryRetention: cfg.Cleanup.ContextSummaryRetention,
		MessagesRetention: cfg.Cleanup.MessagesRetention, Analytics: cfg.Cleanup.Analytics,
		CompletionSignal: completionSignal,
	})
	newsService := news.NewService(callRepository, callService, cfg.Payload.MaxNewsTextBytes)
	companyContextService := companycontext.NewService(profileStore, metricSettings, companycontext.Limits{
		DefaultPageSize:      cfg.Pagination.CompanyItemsDefaultPageSize,
		MaxPageSize:          cfg.Pagination.CompanyItemsMaxPageSize,
		MaxMetricsPerRequest: cfg.Payload.MaxCompanyMetricsPerRequest,
	})

	ready := func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return err
		}
		if err := ch.Ping(ctx); err != nil {
			return err
		}
		for _, agent := range cfg.Agents {
			checkCtx, cancel := context.WithTimeout(ctx, cfg.Hermes.HealthTimeout)
			err := hermesClient.Health(checkCtx, agent)
			cancel()
			if err != nil {
				return err
			}
		}
		return nil
	}
	api := httpapi.NewHandler(cfg.Connections.ESBAPIKey, callService, analyticsService, companyContextService, newsService, ready, httpapi.Config{
		MaxJSONBodyBytes: cfg.HTTP.MaxJSONBodyBytes,
		MaxFrameBytes:    cfg.WebSocket.MaxFrameBytes, ReadTimeout: cfg.WebSocket.ReadTimeout,
		WriteTimeout:  cfg.WebSocket.WriteTimeout,
		OutgoingQueue: cfg.WebSocket.OutgoingQueue, ReplayLimit: cfg.WebSocket.ReplayLimit,
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", presentationmcp.NewStorageHandler(analyticsService, companyContextService, cfg.Connections.ESBAPIKey))
	mux.Handle("/", api)
	server := &http.Server{Addr: cfg.Connections.HTTPAddr, Handler: http.MaxBytesHandler(mux, cfg.HTTP.MaxJSONBodyBytes),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout, IdleTimeout: cfg.HTTP.IdleTimeout}
	var background sync.WaitGroup
	background.Add(3)
	go func() {
		defer background.Done()
		callService.Run(ctx)
	}()
	go func() {
		defer background.Done()
		cleanupService.Run(ctx)
	}()
	go func() {
		defer background.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown failed (error_type=%T)", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("HTTP server forced close failed (error_type=%T)", closeErr)
			}
		}
	}()
	log.Printf("ESB listening on %s", cfg.Connections.HTTPAddr)
	err = server.ListenAndServe()
	stop()
	background.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
