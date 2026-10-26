package cleanup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type cleanupRepoFake struct {
	mu              sync.Mutex
	locked          bool
	lockBusy        bool
	payloadCalls    int
	recordCalls     int
	summaryCalls    int
	messageCalls    map[domain.AgentType]int
	terminalCalls   int
	lastNewsCutoff  time.Time
	payloadErr      error
	lockEntered     chan struct{}
	allowLockFinish chan struct{}
}

func (r *cleanupRepoFake) WithCleanupLock(ctx context.Context, work func(context.Context) error) (bool, error) {
	r.mu.Lock()
	if r.lockBusy {
		r.mu.Unlock()
		return false, nil
	}
	r.lockBusy = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.lockBusy = false
		r.mu.Unlock()
	}()
	if r.lockEntered != nil {
		select {
		case r.lockEntered <- struct{}{}:
		default:
		}
	}
	if r.allowLockFinish != nil {
		select {
		case <-r.allowLockFinish:
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}
	return true, work(ctx)
}
func (r *cleanupRepoFake) RecoverStaleSubmissions(context.Context, time.Time, int) (int, error) {
	return 2, nil
}
func (r *cleanupRepoFake) ListStaleActive(context.Context, time.Time, int) ([]domain.Call, error) {
	return nil, nil
}
func (r *cleanupRepoFake) RetryStaleActive(context.Context, domain.Call, int) (bool, error) {
	return false, nil
}
func (r *cleanupRepoFake) PruneCallPayloads(context.Context, time.Time, int) (int, error) {
	r.payloadCalls++
	return 1, r.payloadErr
}
func (r *cleanupRepoFake) PruneCallRecords(context.Context, time.Time, int) (int, error) {
	r.recordCalls++
	return 3, nil
}
func (r *cleanupRepoFake) PruneMessages(_ context.Context, agent domain.AgentType, _ time.Time, _ int) (int, error) {
	if r.messageCalls == nil {
		r.messageCalls = make(map[domain.AgentType]int)
	}
	r.messageCalls[agent]++
	return 1, nil
}
func (r *cleanupRepoFake) PruneContextSummaries(context.Context, time.Time, int) (int, error) {
	r.summaryCalls++
	return 1, nil
}
func (r *cleanupRepoFake) TerminalNewsIDs(_ context.Context, before time.Time, _ int) ([]string, error) {
	r.terminalCalls++
	r.lastNewsCutoff = before
	return []string{"news-1"}, nil
}

type staleFake struct {
	called        int
	queuedBefore  time.Time
	runningBefore time.Time
	batch         int
	retries       int
}

func (f *staleFake) ReconcileStale(_ context.Context, queuedBefore, runningBefore time.Time, batch, retries int) (int, error) {
	f.called++
	f.queuedBefore, f.runningBefore, f.batch, f.retries = queuedBefore, runningBefore, batch, retries
	return 2, nil
}

type analyticsFake struct {
	calls    int
	policy   domain.AnalyticsRetention
	newsIDs  []string
	batch    int
	err      error
	policies chan domain.AnalyticsRetention
}

func (f *analyticsFake) Prune(_ context.Context, policy domain.AnalyticsRetention, _ time.Time, ids []string, batch int) (map[string]int, error) {
	f.calls++
	f.policy, f.newsIDs, f.batch = policy, ids, batch
	if f.policies != nil {
		f.policies <- policy
	}
	return map[string]int{"processed_news_text": 1}, f.err
}

func inertMessages() map[domain.AgentType]domain.RetentionPolicy {
	return map[domain.AgentType]domain.RetentionPolicy{
		domain.AgentSerezha: {Never: true}, domain.AgentSaveliy: {Never: true},
		domain.AgentValera: {Never: true}, domain.AgentKesha: {Never: true}, domain.AgentKonstantin: {Never: true},
	}
}

func inertAnalyticsRetention() domain.AnalyticsRetention {
	return domain.AnalyticsRetention{
		ProcessedNewsText: domain.RetentionPolicy{Never: true}, UnprocessedNewsText: domain.RetentionPolicy{Never: true},
		NewsMetadata: domain.RetentionPolicy{Never: true}, NewsMarker: domain.RetentionPolicy{Never: true},
		Facts: domain.RetentionPolicy{Never: true}, FactCompany: domain.RetentionPolicy{Never: true},
		MetricObservations: domain.RetentionPolicy{Never: true}, MetricTotals: domain.RetentionPolicy{Never: true},
		CompanyAnalyses: domain.RetentionPolicy{Never: true},
	}
}

func TestCheckStaleCallsUsesItsOwnScheduleAndBoundedPass(t *testing.T) {
	repo := &cleanupRepoFake{}
	stale := &staleFake{}
	now := time.Now().UTC()
	service := NewService(stale, repo, &analyticsFake{}, Config{
		Enabled: true, BatchSize: 17, QueuedStaleAfter: 10 * time.Minute,
		RunningStaleAfter: 30 * time.Minute, ActiveMaxRetries: 1,
	})
	service.checkStaleCalls(context.Background())
	if stale.called != 1 || stale.batch != 17 || stale.retries != 1 {
		t.Fatalf("stale pass args = %#v", stale)
	}
	if time.Since(stale.queuedBefore.Add(10*time.Minute)) > time.Second || time.Since(stale.runningBefore.Add(30*time.Minute)) > time.Second {
		t.Fatalf("stale cutoffs were not derived from current time: %#v", stale)
	}
	if repo.payloadCalls != 0 || repo.recordCalls != 0 || repo.terminalCalls != 0 {
		t.Fatalf("status pass performed retention work: %#v", repo)
	}
	_ = now
}

func TestRetentionPoliciesAreIndependentAndUseTerminalNewsIDs(t *testing.T) {
	repo := &cleanupRepoFake{}
	analytics := &analyticsFake{}
	messages := inertMessages()
	messages[domain.AgentSerezha] = domain.RetentionPolicy{Age: time.Hour}
	messages[domain.AgentKonstantin] = domain.RetentionPolicy{Age: 7 * 24 * time.Hour}
	policy := domain.AnalyticsRetention{
		ProcessedNewsText:   domain.RetentionPolicy{Age: 168 * time.Hour},
		UnprocessedNewsText: domain.RetentionPolicy{Age: 48 * time.Hour},
		NewsMetadata:        domain.RetentionPolicy{Never: true}, NewsMarker: domain.RetentionPolicy{Never: true},
		Facts: domain.RetentionPolicy{Never: true}, FactCompany: domain.RetentionPolicy{Never: true},
		MetricObservations: domain.RetentionPolicy{Never: true}, MetricTotals: domain.RetentionPolicy{Never: true},
		CompanyAnalyses: domain.RetentionPolicy{Never: true},
	}
	service := NewService(&staleFake{}, repo, analytics, Config{
		Enabled: true, BatchSize: 12, MessagesRetention: messages,
		CallPayloadRetention: domain.RetentionPolicy{Never: true}, CallRecordRetention: domain.RetentionPolicy{Never: true},
		ContextSummaryRetention: domain.RetentionPolicy{Never: true}, Analytics: policy,
	})
	service.pruneExpiredData(context.Background())
	if repo.messageCalls[domain.AgentSerezha] != 1 || repo.messageCalls[domain.AgentKonstantin] != 1 || len(repo.messageCalls) != 2 {
		t.Fatalf("message policies were not applied independently: %#v", repo.messageCalls)
	}
	if repo.terminalCalls != 1 || analytics.calls != 1 || analytics.batch != 12 || len(analytics.newsIDs) != 1 || analytics.newsIDs[0] != "news-1" {
		t.Fatalf("analytics cleanup inputs = %#v, terminal lookups=%d", analytics, repo.terminalCalls)
	}
	if analytics.policy.ProcessedNewsText.Age != 168*time.Hour || analytics.policy.NewsMetadata.Never != true {
		t.Fatalf("analytics retention policy changed: %#v", analytics.policy)
	}
}

func TestRetentionPartialFailureIsRetryableAndDoesNotAdvanceClickHouse(t *testing.T) {
	repo := &cleanupRepoFake{payloadErr: errors.New("simulated database error")}
	analytics := &analyticsFake{}
	service := NewService(&staleFake{}, repo, analytics, Config{
		Enabled: true, BatchSize: 3, MessagesRetention: inertMessages(),
		CallPayloadRetention: domain.RetentionPolicy{Age: time.Hour}, CallRecordRetention: domain.RetentionPolicy{Never: true},
		ContextSummaryRetention: domain.RetentionPolicy{Never: true}, Analytics: inertAnalyticsRetention(),
	})
	service.pruneExpiredData(context.Background())
	if repo.payloadCalls != 1 || analytics.calls != 0 {
		t.Fatalf("partial failure should stop before ClickHouse: payload=%d analytics=%d", repo.payloadCalls, analytics.calls)
	}
	repo.payloadErr = nil
	service.pruneExpiredData(context.Background())
	if repo.payloadCalls != 2 || analytics.calls != 1 {
		t.Fatalf("repeat did not resume idempotently: payload=%d analytics=%d", repo.payloadCalls, analytics.calls)
	}
}

func TestImmediatePassRunsOnlyImmediatePolicies(t *testing.T) {
	repo := &cleanupRepoFake{}
	analytics := &analyticsFake{}
	messages := inertMessages()
	messages[domain.AgentSaveliy] = domain.RetentionPolicy{Immediate: true}
	policy := inertAnalyticsRetention()
	policy.ProcessedNewsText = domain.RetentionPolicy{Immediate: true}
	policy.UnprocessedNewsText = domain.RetentionPolicy{Immediate: true}
	policy.Facts = domain.RetentionPolicy{Age: time.Hour}
	service := NewService(&staleFake{}, repo, analytics, Config{
		Enabled: true, BatchSize: 7, MessagesRetention: messages,
		CallPayloadRetention:    domain.RetentionPolicy{Immediate: true},
		CallRecordRetention:     domain.RetentionPolicy{Never: true},
		ContextSummaryRetention: domain.RetentionPolicy{Never: true}, Analytics: policy,
	})

	service.pruneImmediateData(context.Background())

	if repo.payloadCalls != 1 || repo.recordCalls != 0 || repo.messageCalls[domain.AgentSaveliy] != 1 ||
		len(repo.messageCalls) != 1 || repo.terminalCalls != 1 {
		t.Fatalf("immediate policies were not isolated: repo=%#v", repo)
	}
	if !analytics.policy.ProcessedNewsText.Immediate || !analytics.policy.UnprocessedNewsText.Immediate ||
		!analytics.policy.Facts.Never || analytics.batch != 7 || len(analytics.newsIDs) != 1 {
		t.Fatalf("ClickHouse immediate policies = %#v", analytics)
	}
}

func TestCompletionSignalTriggersImmediateCleanupPass(t *testing.T) {
	repo := &cleanupRepoFake{}
	policyEvents := make(chan domain.AnalyticsRetention, 2)
	analytics := &analyticsFake{policies: policyEvents}
	completion := make(chan struct{}, 1)
	retention := inertAnalyticsRetention()
	retention.ProcessedNewsText = domain.RetentionPolicy{Immediate: true}
	retention.Facts = domain.RetentionPolicy{Age: time.Hour}
	settings := Config{
		Enabled: true, StatusCheckInterval: time.Hour, RetentionCheckInterval: time.Hour, BatchSize: 5,
		MessagesRetention: inertMessages(), CallPayloadRetention: domain.RetentionPolicy{Never: true},
		CallRecordRetention: domain.RetentionPolicy{Never: true}, ContextSummaryRetention: domain.RetentionPolicy{Never: true},
		Analytics:        retention,
		CompletionSignal: completion,
	}
	service := NewService(&staleFake{}, repo, analytics, settings)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Run(ctx)
	}()

	firstPolicy := receivePolicy(t, policyEvents)
	if firstPolicy.Facts.Age != time.Hour {
		t.Fatalf("initial retention pass lost its age policy: %#v", firstPolicy)
	}
	completion <- struct{}{}
	secondPolicy := receivePolicy(t, policyEvents)
	if !secondPolicy.ProcessedNewsText.Immediate || !secondPolicy.Facts.Never {
		t.Fatalf("completion pass did not select immediate-only policy: %#v", secondPolicy)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup service did not stop after cancellation")
	}
}

func receivePolicy(t *testing.T, events <-chan domain.AnalyticsRetention) domain.AnalyticsRetention {
	t.Helper()
	select {
	case policy := <-events:
		return policy
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for retention pass")
		return domain.AnalyticsRetention{}
	}
}

func TestConcurrentRetentionPassSkipsWhenReplicaLockIsBusy(t *testing.T) {
	repo := &cleanupRepoFake{lockEntered: make(chan struct{}, 1), allowLockFinish: make(chan struct{})}
	analytics := &analyticsFake{}
	service := NewService(&staleFake{}, repo, analytics, Config{
		Enabled: true, BatchSize: 5, MessagesRetention: inertMessages(),
		CallPayloadRetention: domain.RetentionPolicy{Never: true}, CallRecordRetention: domain.RetentionPolicy{Never: true},
		ContextSummaryRetention: domain.RetentionPolicy{Never: true}, Analytics: inertAnalyticsRetention(),
	})
	var first sync.WaitGroup
	first.Add(1)
	go func() {
		defer first.Done()
		service.pruneExpiredData(context.Background())
	}()
	<-repo.lockEntered
	service.pruneExpiredData(context.Background())
	if analytics.calls != 0 {
		t.Fatal("second pass ran without acquiring the shared lock")
	}
	close(repo.allowLockFinish)
	first.Wait()
	if analytics.calls != 1 {
		t.Fatalf("first pass should run once after releasing its lock, got %d", analytics.calls)
	}
}

func TestSafeFailureStageNeverIncludesAdapterDetails(t *testing.T) {
	if got := safeFailureStage(cleanupStageError("metric_totals", fmt.Errorf("query contains private values"))); got != "metric_totals" {
		t.Fatalf("safe stage = %q", got)
	}
	if got := safeFailureStage(errors.New("database diagnostic with values")); got != "postgres_or_clickhouse" {
		t.Fatalf("fallback stage = %q", got)
	}
}
