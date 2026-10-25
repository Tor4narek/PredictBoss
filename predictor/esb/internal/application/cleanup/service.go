package cleanup

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type Config struct {
	Enabled                 bool
	StatusCheckInterval     time.Duration
	RetentionCheckInterval  time.Duration
	BatchSize               int
	QueuedStaleAfter        time.Duration
	RunningStaleAfter       time.Duration
	ActiveMaxRetries        int
	CallPayloadRetention    domain.RetentionPolicy
	CallRecordRetention     domain.RetentionPolicy
	ContextSummaryRetention domain.RetentionPolicy
	MessagesRetention       map[domain.AgentType]domain.RetentionPolicy
	Analytics               domain.AnalyticsRetention
}

type Service struct {
	calls      domain.StaleCallReconciler
	repository domain.CleanupRepository
	analytics  domain.AnalyticsCleanupRepository
	settings   Config
}

func NewService(calls domain.StaleCallReconciler, repository domain.CleanupRepository, analytics domain.AnalyticsCleanupRepository, settings Config) *Service {
	return &Service{calls: calls, repository: repository, analytics: analytics, settings: settings}
}

// Run owns two independent maintenance schedules: stale-run reconciliation
// and age-based retention. The PostgreSQL advisory lock prevents two ESB
// replicas from mutating the same candidate sets concurrently.
func (s *Service) Run(ctx context.Context) {
	if !s.settings.Enabled {
		return
	}
	statusTicker := time.NewTicker(s.settings.StatusCheckInterval)
	defer statusTicker.Stop()
	retentionTicker := time.NewTicker(s.settings.RetentionCheckInterval)
	defer retentionTicker.Stop()
	s.checkStaleCalls(ctx)
	s.pruneExpiredData(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-statusTicker.C:
			s.checkStaleCalls(ctx)
		case <-retentionTicker.C:
			s.pruneExpiredData(ctx)
		}
	}
}

func (s *Service) checkStaleCalls(ctx context.Context) {
	now := time.Now().UTC()
	reconciled := 0
	locked, err := s.repository.WithCleanupLock(ctx, func(lockCtx context.Context) error {
		var reconcileErr error
		reconciled, reconcileErr = s.calls.ReconcileStale(lockCtx,
			now.Add(-s.settings.QueuedStaleAfter), now.Add(-s.settings.RunningStaleAfter),
			s.settings.BatchSize, s.settings.ActiveMaxRetries)
		return reconcileErr
	})
	if err != nil {
		log.Printf("cleanup stale-call pass failed")
		return
	}
	if locked {
		log.Printf("cleanup stale-call pass completed (reconciled=%d)", reconciled)
	}
}

func (s *Service) pruneExpiredData(ctx context.Context) {
	now := time.Now().UTC()
	counts := make(map[string]int)
	locked, err := s.repository.WithCleanupLock(ctx, func(lockCtx context.Context) error {
		var terminalNews []string
		if !s.settings.Analytics.UnprocessedNewsText.Never {
			if s.settings.Analytics.UnprocessedNewsText.Age <= 0 {
				return errors.New("unprocessed news text retention age must be positive or never")
			}
			ids, listErr := s.repository.TerminalNewsIDs(lockCtx,
				now.Add(-s.settings.Analytics.UnprocessedNewsText.Age), s.settings.BatchSize)
			if listErr != nil {
				return listErr
			}
			terminalNews = ids
		}

		for _, agent := range []domain.AgentType{domain.AgentSerezha, domain.AgentSaveliy, domain.AgentValera, domain.AgentKesha, domain.AgentKonstantin} {
			policy, configured := s.settings.MessagesRetention[agent]
			if !configured || policy.Never {
				continue
			}
			if policy.Age <= 0 {
				return errors.New("message retention age must be positive or never")
			}
			count, pruneErr := s.repository.PruneMessages(lockCtx, agent, now.Add(-policy.Age), s.settings.BatchSize)
			counts["messages_"+string(agent)] = count
			if pruneErr != nil {
				return pruneErr
			}
		}
		if policy := s.settings.ContextSummaryRetention; !policy.Never {
			if policy.Age <= 0 {
				return errors.New("context summary retention age must be positive or never")
			}
			count, pruneErr := s.repository.PruneContextSummaries(lockCtx, now.Add(-policy.Age), s.settings.BatchSize)
			counts["context_summaries"] = count
			if pruneErr != nil {
				return pruneErr
			}
		}
		if policy := s.settings.CallPayloadRetention; !policy.Never {
			if policy.Age <= 0 {
				return errors.New("call payload retention age must be positive or never")
			}
			count, pruneErr := s.repository.PruneCallPayloads(lockCtx, now.Add(-policy.Age), s.settings.BatchSize)
			counts["call_payloads"] = count
			if pruneErr != nil {
				return pruneErr
			}
		}
		if policy := s.settings.CallRecordRetention; !policy.Never {
			if policy.Age <= 0 {
				return errors.New("call record retention age must be positive or never")
			}
			count, pruneErr := s.repository.PruneCallRecords(lockCtx, now.Add(-policy.Age), s.settings.BatchSize)
			counts["call_records"] = count
			if pruneErr != nil {
				return pruneErr
			}
		}
		if s.analytics != nil {
			analyticsCounts, pruneErr := s.analytics.Prune(lockCtx, s.settings.Analytics, now, terminalNews, s.settings.BatchSize)
			for category, count := range analyticsCounts {
				counts["clickhouse_"+category] = count
			}
			if pruneErr != nil {
				return pruneErr
			}
		}
		return nil
	})
	if err != nil {
		// Do not log adapter errors: SQL diagnostics can contain data values.
		log.Printf("cleanup retention pass failed (stage=%s, completed_counts=%v)", safeFailureStage(err), counts)
		return
	}
	if locked {
		log.Printf("cleanup retention pass completed (counts=%v)", counts)
	}
}

// safeFailureStage reports only a fixed cleanup category. Adapter error text
// can contain query values, so it must never be copied into operational logs.
func safeFailureStage(err error) string {
	if err == nil {
		return "unknown"
	}
	message := err.Error()
	if strings.HasPrefix(message, "cleanup ") {
		stage, _, ok := strings.Cut(strings.TrimPrefix(message, "cleanup "), ":")
		if ok && stage != "" {
			return stage
		}
	}
	return "postgres_or_clickhouse"
}
