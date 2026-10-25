package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/application/news"
	"github.com/predictboss/predictor/esb/internal/domain"
	"github.com/predictboss/predictor/esb/internal/infrastructure/clickhouse"
	"github.com/predictboss/predictor/esb/internal/infrastructure/domainapi"
	"github.com/predictboss/predictor/esb/internal/infrastructure/hermes"
	"github.com/predictboss/predictor/esb/internal/infrastructure/postgres"
	"github.com/predictboss/predictor/esb/internal/infrastructure/s3"
	"github.com/predictboss/predictor/esb/internal/presentation/httpapi"
	presentationmcp "github.com/predictboss/predictor/esb/internal/presentation/mcp"
)

const defaultHistoryLimit = 40

type agentFile struct {
	Name           string   `json:"name"`
	ScopeField     string   `json:"scope_field"`
	SkillFile      string   `json:"skill_file"`
	RuntimeURLEnv  string   `json:"runtime_url_env"`
	RequiredFields []string `json:"required_fields"`
	MaxHistory     int      `json:"max_history_messages"`
}

type config struct {
	HTTPAddr         string
	DatabaseURL      string
	ESBAPIKey        string
	HermesAPIKey     string
	ClickHouseURL    string
	CHDatabase       string
	CHUser           string
	CHPassword       string
	S3Bucket         string
	S3Region         string
	S3Endpoint       string
	S3AccessKey      string
	S3SecretKey      string
	S3PathStyle      bool
	S3CreateBucket   bool
	DomainAPIBaseURL string
	DomainAPIToken   string
	PollInterval     time.Duration
	Agents           []domain.Agent
}

func main() {
	if err := runESB(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func runESB() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	ch, err := clickhouse.NewClient(cfg.ClickHouseURL, cfg.CHDatabase, cfg.CHUser, cfg.CHPassword)
	if err != nil {
		return err
	}
	if err := ch.Ping(ctx); err != nil {
		return fmt.Errorf("ping ClickHouse: %w", err)
	}
	profileStore, err := s3.NewStore(ctx, s3.Config{
		Bucket: cfg.S3Bucket, Region: cfg.S3Region, Endpoint: cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle, CreateBucket: cfg.S3CreateBucket,
	})
	if err != nil {
		return err
	}
	callRepository := postgres.NewCalls(db)
	if err := callRepository.Recover(ctx); err != nil {
		return fmt.Errorf("recover PostgreSQL calls: %w", err)
	}
	hermesClient := hermes.NewClient(cfg.HermesAPIKey)
	callService := calls.NewService(callRepository, hermesClient, cfg.Agents, cfg.PollInterval)
	var newsService *news.Service
	if cfg.DomainAPIBaseURL != "" || cfg.DomainAPIToken != "" {
		metadataClient, err := domainapi.NewClient(cfg.DomainAPIBaseURL, cfg.DomainAPIToken)
		if err != nil {
			return err
		}
		newsService = news.NewService(metadataClient, callRepository, callService)
	}
	metricSettings := postgres.NewMetricSettings(db)
	companyContextService := companycontext.NewService(profileStore, metricSettings)
	analyticsService := analytics.NewService(clickhouse.NewAnalytics(ch), metricSettings)

	ready := func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return err
		}
		if err := ch.Ping(ctx); err != nil {
			return err
		}
		for _, agent := range cfg.Agents {
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := hermesClient.Health(checkCtx, agent)
			cancel()
			if err != nil {
				return err
			}
		}
		return nil
	}
	api := httpapi.NewHandler(cfg.ESBAPIKey, callService, analyticsService, companyContextService, newsService, ready)
	mux := http.NewServeMux()
	mux.Handle("/mcp", presentationmcp.NewStorageHandler(analyticsService, companyContextService, cfg.ESBAPIKey))
	mux.Handle("/", api)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go callService.Run(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("ESB listening on %s", cfg.HTTPAddr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func loadConfig() (config, error) {
	cfg := config{
		HTTPAddr: env("HTTP_ADDR", ":8081"), DatabaseURL: os.Getenv("DATABASE_URL"), ESBAPIKey: os.Getenv("ESB_API_KEY"),
		HermesAPIKey: os.Getenv("HERMES_API_KEY"), ClickHouseURL: strings.TrimRight(env("CLICKHOUSE_URL", "http://localhost:8123"), "/"),
		CHDatabase: env("CLICKHOUSE_DATABASE", "default"), CHUser: env("CLICKHOUSE_USER", "default"), CHPassword: os.Getenv("CLICKHOUSE_PASSWORD"),
		S3Bucket: os.Getenv("S3_BUCKET"), S3Region: env("S3_REGION", "us-east-1"), S3Endpoint: os.Getenv("S3_ENDPOINT"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY_ID"), S3SecretKey: os.Getenv("S3_SECRET_ACCESS_KEY"), S3PathStyle: strings.EqualFold(os.Getenv("S3_FORCE_PATH_STYLE"), "true"),
		S3CreateBucket: strings.EqualFold(os.Getenv("S3_CREATE_BUCKET"), "true"),
		DomainAPIBaseURL: strings.TrimRight(os.Getenv("DOMAIN_API_BASE_URL"), "/"), DomainAPIToken: os.Getenv("DOMAIN_API_TOKEN"),
		PollInterval: 2 * time.Second,
	}
	if cfg.DatabaseURL == "" || cfg.ESBAPIKey == "" || cfg.HermesAPIKey == "" || cfg.S3Bucket == "" {
		return config{}, errors.New("DATABASE_URL, ESB_API_KEY, HERMES_API_KEY and S3_BUCKET are required")
	}
	if raw := os.Getenv("HERMES_POLL_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil || interval <= 0 {
			return config{}, fmt.Errorf("invalid HERMES_POLL_INTERVAL %q", raw)
		}
		cfg.PollInterval = interval
	}
	root := env("AGENTS_DIR", "../agents")
	for _, name := range []string{"serezha", "saveliy", "valera", "kesha", "konstantin"} {
		definitionPath := filepath.Join(root, name, "agent.json")
		data, err := os.ReadFile(definitionPath)
		if err != nil {
			return config{}, fmt.Errorf("read agent definition %s: %w", definitionPath, err)
		}
		var definition agentFile
		if err := json.Unmarshal(data, &definition); err != nil {
			return config{}, fmt.Errorf("parse agent definition %s: %w", definitionPath, err)
		}
		if definition.Name != name || definition.ScopeField == "" || definition.SkillFile == "" || definition.RuntimeURLEnv == "" {
			return config{}, fmt.Errorf("agent definition %s is incomplete", definitionPath)
		}
		skillPath := filepath.Join(root, name, definition.SkillFile)
		skill, err := os.ReadFile(skillPath)
		if err != nil {
			return config{}, fmt.Errorf("read skill for %s: %w", name, err)
		}
		instructions := skillInstructions(string(skill))
		runtimeURL := strings.TrimRight(os.Getenv(definition.RuntimeURLEnv), "/")
		if runtimeURL == "" {
			return config{}, fmt.Errorf("%s is required", definition.RuntimeURLEnv)
		}
		if definition.MaxHistory <= 0 {
			definition.MaxHistory = defaultHistoryLimit
		}
		cfg.Agents = append(cfg.Agents, domain.Agent{Type: domain.AgentType(name), ScopeField: definition.ScopeField, RequiredFields: definition.RequiredFields, MaxHistory: definition.MaxHistory, RuntimeURL: runtimeURL, Instructions: instructions})
	}
	return cfg, nil
}

func skillInstructions(markdown string) string {
	markdown = strings.TrimSpace(markdown)
	if strings.HasPrefix(markdown, "---\n") {
		if end := strings.Index(markdown[4:], "\n---"); end >= 0 {
			markdown = markdown[4+end+4:]
		}
	}
	return strings.TrimSpace(markdown)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
