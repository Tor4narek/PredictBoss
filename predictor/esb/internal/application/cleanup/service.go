package cleanup

import (
	"context"
	"errors"
	"log"
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
	CompletionSignal        <-chan struct{}
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
	if s.settings.StatusCheckInterval <= 0 || s.settings.RetentionCheckInterval <= 0 || s.settings.BatchSize <= 0 {
		log.Printf("cleanup service stopped (stage=invalid_configuration)")
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
		case <-s.settings.CompletionSignal:
			s.pruneImmediateData(ctx)
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
	s.pruneData(ctx, false)
}

func (s *Service) pruneImmediateData(ctx context.Context) {
	s.pruneData(ctx, true)
}

func (s *Service) pruneData(ctx context.Context, immediateOnly bool) {
	now := time.Now().UTC()
	counts := make(map[string]int)
	locked, err := s.repository.WithCleanupLock(ctx, func(lockCtx context.Context) error {
		var terminalNews []string
		if cutoff, enabled, policyErr := retentionCutoff(s.settings.Analytics.UnprocessedNewsText, now, immediateOnly); policyErr != nil {
			return cleanupStageError("unprocessed_news_text", policyErr)
		} else if enabled {
			ids, listErr := s.repository.TerminalNewsIDs(lockCtx,
				cutoff, s.settings.BatchSize)
			if listErr != nil {
				return cleanupStageError("unprocessed_news_text", listErr)
			}
			terminalNews = ids
		}

		for _, agent := range []domain.AgentType{domain.AgentSerezha, domain.AgentSaveliy, domain.AgentValera, domain.AgentKesha, domain.AgentKonstantin} {
			policy, configured := s.settings.MessagesRetention[agent]
			if !configured {
				continue
			}
			cutoff, enabled, policyErr := retentionCutoff(policy, now, immediateOnly)
			if policyErr != nil {
				return cleanupStageError("messages_"+string(agent), policyErr)
			}
			if !enabled {
				continue
			}
			count, pruneErr := s.repository.PruneMessages(lockCtx, agent, cutoff, s.settings.BatchSize)
			counts["messages_"+string(agent)] = count
			if pruneErr != nil {
				return cleanupStageError("messages_"+string(agent), pruneErr)
			}
		}
		if cutoff, enabled, policyErr := retentionCutoff(s.settings.ContextSummaryRetention, now, immediateOnly); policyErr != nil {
			return cleanupStageError("context_summaries", policyErr)
		} else if enabled {
			count, pruneErr := s.repository.PruneContextSummaries(lockCtx, cutoff, s.settings.BatchSize)
			counts["context_summaries"] = count
			if pruneErr != nil {
				return cleanupStageError("context_summaries", pruneErr)
			}
		}
		if cutoff, enabled, policyErr := retentionCutoff(s.settings.CallPayloadRetention, now, immediateOnly); policyErr != nil {
			return cleanupStageError("call_payloads", policyErr)
		} else if enabled {
			count, pruneErr := s.repository.PruneCallPayloads(lockCtx, cutoff, s.settings.BatchSize)
			counts["call_payloads"] = count
			if pruneErr != nil {
				return cleanupStageError("call_payloads", pruneErr)
			}
		}
		if cutoff, enabled, policyErr := retentionCutoff(s.settings.CallRecordRetention, now, immediateOnly); policyErr != nil {
			return cleanupStageError("call_records", policyErr)
		} else if enabled {
			count, pruneErr := s.repository.PruneCallRecords(lockCtx, cutoff, s.settings.BatchSize)
			counts["call_records"] = count
			if pruneErr != nil {
				return cleanupStageError("call_records", pruneErr)
			}
		}
		if s.analytics != nil {
			policy := s.settings.Analytics
			if immediateOnly {
				policy = immediateAnalyticsRetention(policy)
			}
			analyticsCounts, pruneErr := s.analytics.Prune(lockCtx, policy, now, terminalNews, s.settings.BatchSize)
			for category, count := range analyticsCounts {
				counts["clickhouse_"+category] = count
			}
			if pruneErr != nil {
				return cleanupStageError("clickhouse", pruneErr)
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

type stageError struct {
	stage string
	err   error
}

func (e *stageError) Error() string { return e.stage + ": " + e.err.Error() }
func (e *stageError) Unwrap() error { return e.err }

func cleanupStageError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &stageError{stage: stage, err: err}
}

func retentionCutoff(policy domain.RetentionPolicy, now time.Time, immediateOnly bool) (time.Time, bool, error) {
	if policy.Never || immediateOnly && !policy.Immediate {
		return time.Time{}, false, nil
	}
	if policy.Immediate {
		return now, true, nil
	}
	if policy.Age <= 0 {
		return time.Time{}, false, errors.New("retention must be positive, immediate, or never")
	}
	return now.Add(-policy.Age), true, nil
}

func immediateAnalyticsRetention(policy domain.AnalyticsRetention) domain.AnalyticsRetention {
	return domain.AnalyticsRetention{
		ProcessedNewsText:   immediatePolicy(policy.ProcessedNewsText),
		UnprocessedNewsText: immediatePolicy(policy.UnprocessedNewsText),
		NewsMetadata:        immediatePolicy(policy.NewsMetadata),
		NewsMarker:          immediatePolicy(policy.NewsMarker),
		Facts:               immediatePolicy(policy.Facts),
		FactCompany:         immediatePolicy(policy.FactCompany),
		MetricObservations:  immediatePolicy(policy.MetricObservations),
		MetricTotals:        immediatePolicy(policy.MetricTotals),
		CompanyAnalyses:     immediatePolicy(policy.CompanyAnalyses),
	}
}

func immediatePolicy(policy domain.RetentionPolicy) domain.RetentionPolicy {
	if !policy.Immediate {
		return domain.RetentionPolicy{Never: true}
	}
	return policy
}

// safeFailureStage reports only a fixed cleanup category. Adapter error text
// can contain query values, so it must never be copied into operational logs.
func safeFailureStage(err error) string {
	if err == nil {
		return "unknown"
	}
	var failure *stageError
	if errors.As(err, &failure) {
		return failure.stage
	}
	return "postgres_or_clickhouse"
}
