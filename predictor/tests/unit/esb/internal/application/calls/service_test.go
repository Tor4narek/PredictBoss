package calls

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type completionRepository struct {
	domain.CallRepository
	domain.CallExecutionRepository
	active          []domain.Call
	profileState    domain.CompanyProfileSyncState
	completeValera  bool
	retryScheduled  bool
	retryCalls      int
	retriedRevision int64
	maxRetries      int
	claimLimit      int
	status          domain.CallStatus
	statusMessage   string
	historyLimit    int
	startedRunID    string
}

func testCallConfig(pollInterval time.Duration) Config {
	if pollInterval <= 0 {
		pollInterval = time.Millisecond
	}
	return Config{
		PollInterval: pollInterval, DispatchBatchSize: 100,
		ContextCompactionTimeout:         30 * time.Second,
		ValeraProfileSyncMaxRetries:      1,
		CallExecutionTimeout:             10 * time.Minute,
		StopRequestTimeout:               5 * time.Second,
		KonstantinHistoryDefaultPageSize: 100, KonstantinHistoryMaxPageSize: 100,
		KonstantinDefaultWindow: 30 * 24 * time.Hour, KonstantinMaxWindow: 90 * 24 * time.Hour,
		KonstantinEventPollInterval: 300 * time.Millisecond, MaxQuestionBytes: 1 << 20,
	}
}

func (*completionRepository) BeginHermesLaunch(context.Context, string, time.Time) (bool, error) {
	return true, nil
}

func (*completionRepository) RequestExpiredCalls(context.Context, time.Time, int) ([]domain.Call, error) {
	return nil, nil
}

func (r *completionRepository) ListActive(context.Context) ([]domain.Call, error) {
	return r.active, nil
}

func (r *completionRepository) ClaimQueued(_ context.Context, limit int) ([]domain.Call, error) {
	r.claimLimit = limit
	return nil, nil
}

func (r *completionRepository) Complete(context.Context, string, string) error {
	return nil
}

func (r *completionRepository) CompleteValera(_ context.Context, _, _, _ string, _ int64) (domain.CompanyProfileSyncState, bool, error) {
	return r.profileState, r.completeValera, nil
}

func (r *completionRepository) RetryProfileSave(_ context.Context, _, _ string, revision int64, maxRetries int) (bool, error) {
	r.retryCalls++
	r.retriedRevision = revision
	r.maxRetries = maxRetries
	return r.retryScheduled, nil
}

func (r *completionRepository) SetStatus(_ context.Context, _ string, status domain.CallStatus, message string) error {
	r.status, r.statusMessage = status, message
	return nil
}

func (r *completionRepository) LoadHistory(_ context.Context, _ string, limit int) ([]domain.Message, error) {
	r.historyLimit = limit
	return []domain.Message{}, nil
}

func (r *completionRepository) SetRun(_ context.Context, _, runID string) error {
	r.startedRunID = runID
	return nil
}

type completedRuntime struct{}

func (completedRuntime) Start(context.Context, domain.Agent, string, []domain.Message, string) (string, error) {
	return "", nil
}

func (completedRuntime) Status(context.Context, domain.Agent, string) (domain.RunState, error) {
	return domain.RunState{Status: "completed", Output: "done"}, nil
}

type capturingRuntime struct {
	input   string
	key     string
	history []domain.Message
}

func (r *capturingRuntime) Start(_ context.Context, _ domain.Agent, input string, history []domain.Message, key string) (string, error) {
	r.input, r.key, r.history = input, key, history
	return "run-2", nil
}

func (*capturingRuntime) Status(context.Context, domain.Agent, string) (domain.RunState, error) {
	return domain.RunState{}, nil
}

func TestValeraCompletionRequiresSavedProfileRevision(t *testing.T) {
	tests := []struct {
		name              string
		call              domain.Call
		profileState      domain.CompanyProfileSyncState
		completeValera    bool
		retryScheduled    bool
		wantRetryCalls    int
		wantStatus        domain.CallStatus
		wantStatusMessage string
	}{
		{
			name:           "matching synced revision completes",
			call:           activeValeraCall(0),
			profileState:   domain.CompanyProfileSyncState{Revision: 4, Synced: true},
			completeValera: true,
		},
		{
			name:           "unsynced revision schedules one retry",
			call:           activeValeraCall(0),
			profileState:   domain.CompanyProfileSyncState{Revision: 4, Synced: false},
			retryScheduled: true,
			wantRetryCalls: 1,
		},
		{
			name:              "second unsynced completion fails",
			call:              activeValeraCall(1),
			profileState:      domain.CompanyProfileSyncState{Revision: 4, Synced: false},
			wantStatus:        domain.CallFailed,
			wantStatusMessage: "company_profile_not_synced",
		},
		{
			name:              "superseded revision fails without retry",
			call:              activeValeraCall(0),
			profileState:      domain.CompanyProfileSyncState{Revision: 5, Synced: false},
			wantStatus:        domain.CallFailed,
			wantStatusMessage: "profile_revision_superseded",
		},
		{
			name: "invalid revision fails",
			call: domain.Call{ID: "call-1", Agent: domain.AgentValera, ScopeID: "company-1", Status: domain.CallRunning,
				HermesRunID: stringPointer("run-1"), Input: json.RawMessage(`{"profile_revision":"4"}`)},
			wantStatus:        domain.CallFailed,
			wantStatusMessage: "invalid_profile_revision",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &completionRepository{
				active:         []domain.Call{tt.call},
				profileState:   tt.profileState,
				completeValera: tt.completeValera,
				retryScheduled: tt.retryScheduled,
			}
			service := NewService(repo, completedRuntime{}, []domain.Agent{{Type: domain.AgentValera}}, testCallConfig(0))

			service.pollActive(context.Background())

			if repo.retryCalls != tt.wantRetryCalls {
				t.Fatalf("retry calls = %d, want %d", repo.retryCalls, tt.wantRetryCalls)
			}
			if tt.wantRetryCalls > 0 && repo.retriedRevision != 4 {
				t.Errorf("retried revision = %d, want 4", repo.retriedRevision)
			}
			if tt.wantRetryCalls > 0 && repo.maxRetries != 1 {
				t.Errorf("repository retry ceiling = %d, want 1", repo.maxRetries)
			}
			if repo.status != tt.wantStatus || repo.statusMessage != tt.wantStatusMessage {
				t.Errorf("status = (%q, %q), want (%q, %q)", repo.status, repo.statusMessage, tt.wantStatus, tt.wantStatusMessage)
			}
		})
	}
}

func TestValeraProfileSyncRetryCanBeDisabled(t *testing.T) {
	repo := &completionRepository{
		active:       []domain.Call{activeValeraCall(0)},
		profileState: domain.CompanyProfileSyncState{Revision: 4, Synced: false},
	}
	settings := testCallConfig(0)
	settings.ValeraProfileSyncMaxRetries = 0
	service := NewService(repo, completedRuntime{}, []domain.Agent{{Type: domain.AgentValera}}, settings)

	service.pollActive(context.Background())

	if repo.retryCalls != 0 || repo.status != domain.CallFailed || repo.statusMessage != "company_profile_not_synced" {
		t.Fatalf("retry-disabled Valera call: retries=%d status=(%q, %q)", repo.retryCalls, repo.status, repo.statusMessage)
	}
}

func TestQueueDispatchUsesConfiguredBatchSize(t *testing.T) {
	repo := &completionRepository{}
	settings := testCallConfig(0)
	settings.DispatchBatchSize = 7
	service := NewService(repo, completedRuntime{}, nil, settings)

	service.dispatchQueued(context.Background())

	if repo.claimLimit != 7 {
		t.Fatalf("queue claim limit = %d, want configured batch 7", repo.claimLimit)
	}
}

func TestCancelKonstantinStopsActiveRunOnce(t *testing.T) {
	runID := "hermes-run-1"
	call := domain.Call{ID: "11111111-1111-4111-8111-111111111111", Agent: domain.AgentKonstantin,
		ScopeID: "22222222-2222-4222-8222-222222222222", ConversationID: "33333333-3333-4333-8333-333333333333",
		Status: domain.CallRunning, HermesRunID: &runID}
	repository := &cancellationRepository{
		cancelResult: domain.CancelResult{Call: call, Disposition: domain.CancelRequested},
		claimedCall:  call, claimStop: true,
	}
	runtime := &cancellationRuntime{}
	service := NewService(repository, runtime, []domain.Agent{{Type: domain.AgentKonstantin, RuntimeURL: "http://hermes"}}, testCallConfig(0))

	result, err := service.CancelKonstantin(context.Background(), call.ScopeID, call.ConversationID, "44444444-4444-4444-8444-444444444444")
	if err != nil {
		t.Fatalf("CancelKonstantin returned error: %v", err)
	}
	if result.Disposition != domain.CancelRequested || runtime.stopCalls != 1 || runtime.stoppedRunID != runID {
		t.Fatalf("cancel result=%q stop calls=%d run=%q", result.Disposition, runtime.stopCalls, runtime.stoppedRunID)
	}
}

func TestCancelKonstantinReportsStopFailureWithoutCancellingCall(t *testing.T) {
	runID := "hermes-run-1"
	call := domain.Call{ID: "11111111-1111-4111-8111-111111111111", Agent: domain.AgentKonstantin,
		ScopeID: "22222222-2222-4222-8222-222222222222", ConversationID: "33333333-3333-4333-8333-333333333333",
		Status: domain.CallRunning, HermesRunID: &runID}
	repository := &cancellationRepository{
		cancelResult: domain.CancelResult{Call: call, Disposition: domain.CancelRequested},
		claimedCall:  call, claimStop: true,
	}
	runtime := &cancellationRuntime{stopErr: errors.New("sensitive Hermes response")}
	service := NewService(repository, runtime, []domain.Agent{{Type: domain.AgentKonstantin, RuntimeURL: "http://hermes"}}, testCallConfig(0))

	result, err := service.CancelKonstantin(context.Background(), call.ScopeID, call.ConversationID, "44444444-4444-4444-8444-444444444444")
	if err != nil {
		t.Fatalf("CancelKonstantin returned error instead of a safe stop outcome: %v", err)
	}
	if result.Disposition != domain.CancelFailed || result.Call.Status != domain.CallRunning || result.Call.Error == nil || *result.Call.Error != "stop_failed" {
		t.Fatalf("stop failure changed call to a terminal cancellation or leaked details: %#v", result)
	}
	if runtime.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want one bounded attempt", runtime.stopCalls)
	}
}

type cancellationRepository struct {
	domain.CallRepository
	domain.CallExecutionRepository
	cancelResult domain.CancelResult
	claimedCall  domain.Call
	claimStop    bool
}

func (r *cancellationRepository) CancelKonstantin(context.Context, string, string, string, time.Time) (domain.CancelResult, error) {
	return r.cancelResult, nil
}

func (r *cancellationRepository) ClaimHermesStop(context.Context, string, time.Time) (domain.Call, bool, error) {
	return r.claimedCall, r.claimStop, nil
}

func (r *cancellationRepository) MarkHermesStopFailed(_ context.Context, _ string, reason string, at time.Time) (domain.Call, error) {
	r.claimedCall.StopFailedAt = &at
	r.claimedCall.StopFailureSeq = 2
	r.claimedCall.Error = &reason
	return r.claimedCall, nil
}

func (r *cancellationRepository) RequestStatus(context.Context, string) (domain.Call, error) {
	return r.claimedCall, nil
}

type cancellationRuntime struct {
	stopCalls    int
	stoppedRunID string
	stopErr      error
}

func (*cancellationRuntime) Start(context.Context, domain.Agent, string, []domain.Message, string) (string, error) {
	return "", nil
}

func (*cancellationRuntime) Status(context.Context, domain.Agent, string) (domain.RunState, error) {
	return domain.RunState{}, nil
}

func (r *cancellationRuntime) Stop(_ context.Context, _ domain.Agent, runID string) error {
	r.stopCalls++
	r.stoppedRunID = runID
	return r.stopErr
}

func TestProfileRevisionRequiresPositiveInteger(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		valid bool
	}{
		{input: `{"profile_revision":1}`, want: 1, valid: true},
		{input: `{"profile_revision":0}`},
		{input: `{"profile_revision":-1}`},
		{input: `{"profile_revision":1.5}`},
		{input: `{"profile_revision":"1"}`},
		{input: `{}`},
		{input: `not-json`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, valid := profileRevision(json.RawMessage(tt.input))
			if got != tt.want || valid != tt.valid {
				t.Errorf("profileRevision(%s) = (%d, %t), want (%d, %t)", tt.input, got, valid, tt.want, tt.valid)
			}
		})
	}
}

func TestRetrySubmitUsesFreshHermesIdempotencyKeyAndNoValeraHistory(t *testing.T) {
	repo := &completionRepository{}
	runtime := &capturingRuntime{}
	service := NewService(repo, runtime, []domain.Agent{{Type: domain.AgentValera, MaxHistory: 0}}, testCallConfig(0))
	call := activeValeraCall(1)

	service.submit(context.Background(), call)

	if repo.historyLimit != 0 || len(runtime.history) != 0 {
		t.Fatalf("Valera history limit/messages = %d/%d, want 0/0", repo.historyLimit, len(runtime.history))
	}
	if want := call.ID + ":profile-sync-retry:1"; runtime.key != want {
		t.Errorf("retry idempotency key = %q, want %q", runtime.key, want)
	}
	if repo.startedRunID != "run-2" {
		t.Errorf("saved run ID = %q, want run-2", repo.startedRunID)
	}
}

type contextRepositoryFake struct {
	completionRepository
	window       domain.ContextWindow
	conflicts    int
	saveCount    int
	savedSummary string
	loads        int
	saveErr      error
}

func (r *contextRepositoryFake) LoadContextWindow(context.Context, string, int, int, int) (domain.ContextWindow, error) {
	r.loads++
	return r.window, nil
}

func (r *contextRepositoryFake) SaveContextSummary(_ context.Context, _ string, expectedID string, _ time.Time, throughID string, throughAt time.Time, summary string) (bool, error) {
	r.saveCount++
	if r.saveErr != nil {
		return false, r.saveErr
	}
	if r.conflicts > 0 {
		r.conflicts--
		r.window.Summary = "Concurrent summary with decision 42."
		r.window.SummaryThroughCallID = "other-call"
		r.window.SummaryThroughCreatedAt = throughAt.Add(-time.Second)
		r.window.Pending = nil
		r.window.Recent = []domain.Message{{Role: "user", Content: "Current request"}}
		return false, nil
	}
	if expectedID != r.window.SummaryThroughCallID {
		return false, errors.New("stale summary cursor")
	}
	r.window.Summary = summary
	r.window.SummaryThroughCallID = throughID
	r.window.SummaryThroughCreatedAt = throughAt
	r.window.Pending = nil
	r.savedSummary = summary
	return true, nil
}

type contextRuntimeFake struct {
	starts       []runtimeStart
	failSummary  bool
	summaryState string
}

type runtimeStart struct {
	agent   domain.Agent
	input   string
	history []domain.Message
	key     string
}

func (r *contextRuntimeFake) Start(_ context.Context, agent domain.Agent, input string, history []domain.Message, key string) (string, error) {
	r.starts = append(r.starts, runtimeStart{agent: agent, input: input, history: append([]domain.Message(nil), history...), key: key})
	if agent.Instructions == contextCompactionInstructions {
		if r.failSummary {
			return "", errors.New("mock Hermes unavailable")
		}
		return "summary-run", nil
	}
	return "agent-run", nil
}

func (r *contextRuntimeFake) Status(_ context.Context, _ domain.Agent, runID string) (domain.RunState, error) {
	if runID == "summary-run" {
		return domain.RunState{Status: "completed", Output: r.summaryState}, nil
	}
	return domain.RunState{Status: "running"}, nil
}

func TestContextCompressionStoresSummaryAndPassesOnlyRecentMessages(t *testing.T) {
	repo := &contextRepositoryFake{window: domain.ContextWindow{
		ConversationID:       "conversation-1",
		Pending:              []domain.ContextMessage{{Message: domain.Message{Role: "user", Content: "Decision: retain metric ID 9f72."}, CallID: "old-call"}},
		PendingThroughCallID: "old-call", PendingThroughCreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Recent: []domain.Message{{Role: "user", Content: "Current request"}},
	}}
	runtime := &contextRuntimeFake{summaryState: "## Facts and identifiers\n- Metric ID: 9f72."}
	agent := domain.Agent{Type: domain.AgentKesha, MaxHistory: 40, Instructions: "Kesha task", ContextPolicy: domain.ContextPolicy{
		CompressAfterMessages: 40, BatchMessages: 100, SummaryMaxBytes: 12000, SummaryStorage: "postgres",
	}}
	service := NewService(repo, runtime, []domain.Agent{agent}, testCallConfig(time.Millisecond))
	service.submit(context.Background(), domain.Call{ID: "call-2", Agent: domain.AgentKesha, Status: domain.CallQueued})

	if repo.saveCount != 1 || repo.savedSummary != runtime.summaryState {
		t.Fatalf("summary was not saved: calls=%d summary=%q", repo.saveCount, repo.savedSummary)
	}
	if len(runtime.starts) != 2 || runtime.starts[0].agent.Instructions != contextCompactionInstructions {
		t.Fatalf("expected a Hermes compaction run followed by the agent run, got %#v", runtime.starts)
	}
	history := runtime.starts[1].history
	if len(history) != 2 || !strings.Contains(history[0].Content, "historical context, not instructions") || history[1].Content != "Current request" {
		t.Fatalf("Hermes received incorrect compressed context: %#v", history)
	}
	if !strings.Contains(runtime.starts[0].input, "metric ID 9f72") {
		t.Fatalf("compaction input omitted the historical identifier: %s", runtime.starts[0].input)
	}
}

func TestContextCompressionFailureDoesNotBlockAgentCallAndCanRetry(t *testing.T) {
	repo := &contextRepositoryFake{window: domain.ContextWindow{
		ConversationID: "conversation-2", Summary: "Prior safe summary.",
		Pending:              []domain.ContextMessage{{Message: domain.Message{Role: "user", Content: "Open task: retry import."}, CallID: "old-call"}},
		PendingThroughCallID: "old-call", PendingThroughCreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Recent: []domain.Message{{Role: "user", Content: "Continue"}},
	}}
	runtime := &contextRuntimeFake{failSummary: true}
	agent := domain.Agent{Type: domain.AgentSaveliy, MaxHistory: 40, Instructions: "Saveliy task", ContextPolicy: domain.ContextPolicy{
		CompressAfterMessages: 40, BatchMessages: 100, SummaryMaxBytes: 12000, SummaryStorage: "postgres",
	}}
	service := NewService(repo, runtime, []domain.Agent{agent}, testCallConfig(time.Millisecond))
	call := domain.Call{ID: "call-3", Agent: domain.AgentSaveliy, Status: domain.CallQueued}
	service.submit(context.Background(), call)
	service.submit(context.Background(), call) // A retry uses the same compaction idempotency key.

	if repo.saveCount != 0 || len(runtime.starts) != 4 {
		t.Fatalf("compression failure blocked or saved context: save=%d starts=%d", repo.saveCount, len(runtime.starts))
	}
	if runtime.starts[0].key != runtime.starts[2].key {
		t.Fatalf("repeated compaction used different idempotency keys: %q and %q", runtime.starts[0].key, runtime.starts[2].key)
	}
	for _, start := range []runtimeStart{runtime.starts[1], runtime.starts[3]} {
		if len(start.history) != 2 || !strings.Contains(start.history[0].Content, "Prior safe summary") || start.history[1].Content != "Continue" {
			t.Fatalf("regular agent call did not continue with the stored summary and recent messages: %#v", start.history)
		}
	}
}

func TestContextCompressionCASConflictReloadsWithoutOverwritingConcurrentSummary(t *testing.T) {
	repo := &contextRepositoryFake{conflicts: 1, window: domain.ContextWindow{
		ConversationID:       "conversation-cas",
		Pending:              []domain.ContextMessage{{Message: domain.Message{Role: "user", Content: "Keep decision 42."}, CallID: "old-call"}},
		PendingThroughCallID: "old-call", PendingThroughCreatedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
		Recent: []domain.Message{{Role: "user", Content: "Current request"}},
	}}
	runtime := &contextRuntimeFake{summaryState: "Stale replacement summary"}
	agent := domain.Agent{Type: domain.AgentKesha, MaxHistory: 40, Instructions: "Kesha task", ContextPolicy: domain.ContextPolicy{
		CompressAfterMessages: 40, BatchMessages: 100, SummaryMaxBytes: 12000, SummaryStorage: "postgres",
	}}
	service := NewService(repo, runtime, []domain.Agent{agent}, testCallConfig(time.Millisecond))
	service.submit(context.Background(), domain.Call{ID: "call-cas", Agent: domain.AgentKesha, Status: domain.CallQueued})

	if repo.saveCount != 1 || repo.loads != 2 {
		t.Fatalf("CAS conflict was not reloaded: saves=%d loads=%d", repo.saveCount, repo.loads)
	}
	if repo.window.Summary != "Concurrent summary with decision 42." {
		t.Fatalf("concurrent summary was overwritten: %q", repo.window.Summary)
	}
	if len(runtime.starts) != 2 {
		t.Fatalf("expected one compaction attempt and one normal run, got %d starts", len(runtime.starts))
	}
	if got := runtime.starts[1].history[0].Content; !strings.Contains(got, "Concurrent summary with decision 42.") {
		t.Fatalf("Hermes did not receive the concurrent summary: %q", got)
	}
}

func TestContextSummaryStorageFailureDoesNotBlockAgentRun(t *testing.T) {
	repo := &contextRepositoryFake{saveErr: errors.New("summary store unavailable"), window: domain.ContextWindow{
		ConversationID: "conversation-store-error", Summary: "Prior stored summary.",
		Pending:              []domain.ContextMessage{{Message: domain.Message{Role: "user", Content: "Keep source ID src-7."}, CallID: "old-call"}},
		PendingThroughCallID: "old-call", PendingThroughCreatedAt: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		Recent: []domain.Message{{Role: "user", Content: "Continue"}},
	}}
	runtime := &contextRuntimeFake{summaryState: "Summary that could not be stored."}
	agent := domain.Agent{Type: domain.AgentSaveliy, MaxHistory: 40, Instructions: "Saveliy task", ContextPolicy: domain.ContextPolicy{
		CompressAfterMessages: 40, BatchMessages: 100, SummaryMaxBytes: 12000, SummaryStorage: "postgres",
	}}
	service := NewService(repo, runtime, []domain.Agent{agent}, testCallConfig(time.Millisecond))
	service.submit(context.Background(), domain.Call{ID: "call-store-error", Agent: domain.AgentSaveliy, Status: domain.CallQueued})

	if len(runtime.starts) != 2 || repo.saveCount != 1 || repo.savedSummary != "" {
		t.Fatalf("summary persistence error blocked the normal run or saved incomplete state: starts=%d saves=%d summary=%q", len(runtime.starts), repo.saveCount, repo.savedSummary)
	}
	history := runtime.starts[1].history
	if len(history) != 2 || !strings.Contains(history[0].Content, "Prior stored summary") || history[1].Content != "Continue" {
		t.Fatalf("normal run did not retain the last safe context after summary storage failed: %#v", history)
	}
}

func TestZeroHistoryDoesNotLoadContextOrPassSummary(t *testing.T) {
	repo := &contextRepositoryFake{window: domain.ContextWindow{Summary: "must not be passed"}}
	runtime := &contextRuntimeFake{}
	service := NewService(repo, runtime, []domain.Agent{{Type: domain.AgentValera, MaxHistory: 0}}, testCallConfig(time.Millisecond))
	service.submit(context.Background(), domain.Call{ID: "valera-no-history", Agent: domain.AgentValera, Status: domain.CallQueued})

	if repo.loads != 0 || repo.historyLimit != 0 {
		t.Fatalf("zero history accessed stored context: loads=%d history_limit=%d", repo.loads, repo.historyLimit)
	}
	if len(runtime.starts) != 1 || len(runtime.starts[0].history) != 0 {
		t.Fatalf("zero history passed context to Hermes: %#v", runtime.starts)
	}
}

func activeValeraCall(retryCount int) domain.Call {
	return domain.Call{
		ID: "call-1", Agent: domain.AgentValera, ScopeID: "company-1", Status: domain.CallRunning,
		HermesRunID: stringPointer("run-1"), RetryCount: retryCount, Input: json.RawMessage(`{"profile_revision":4}`),
	}
}

func stringPointer(value string) *string { return &value }

type staleCleanupRepository struct {
	completionRepository
	staleCalls  []domain.Call
	recovered   int
	retryCount  int
	retryMax    int
	retryResult bool
}

func (r *staleCleanupRepository) RecoverStaleSubmissions(context.Context, time.Time, int) (int, error) {
	return r.recovered, nil
}
func (r *staleCleanupRepository) ListStaleActive(context.Context, time.Time, int) ([]domain.Call, error) {
	return r.staleCalls, nil
}
func (r *staleCleanupRepository) RetryStaleActive(_ context.Context, _ domain.Call, maxRetries int) (bool, error) {
	r.retryCount++
	r.retryMax = maxRetries
	return r.retryResult, nil
}
func (*staleCleanupRepository) WithCleanupLock(context.Context, func(context.Context) error) (bool, error) {
	return false, nil
}
func (*staleCleanupRepository) PruneCallPayloads(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (*staleCleanupRepository) PruneCallRecords(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (*staleCleanupRepository) PruneMessages(context.Context, domain.AgentType, time.Time, int) (int, error) {
	return 0, nil
}
func (*staleCleanupRepository) PruneContextSummaries(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (*staleCleanupRepository) TerminalNewsIDs(context.Context, time.Time, int) ([]string, error) {
	return nil, nil
}

type fixedStateRuntime struct {
	state domain.RunState
	err   error
}

func (r fixedStateRuntime) Start(context.Context, domain.Agent, string, []domain.Message, string) (string, error) {
	return "", nil
}
func (r fixedStateRuntime) Status(context.Context, domain.Agent, string) (domain.RunState, error) {
	return r.state, r.err
}

func TestReconcileStaleRetriesOnlyConfirmedRetryableRuns(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	runID := "hermes-run"
	repo := &staleCleanupRepository{
		staleCalls: []domain.Call{{ID: "call-stale", Agent: domain.AgentKesha, Status: domain.CallRunning,
			HermesRunID: &runID, RetryCount: 0, UpdatedAt: old}},
		recovered: 2, retryResult: true,
	}
	service := NewService(repo, fixedStateRuntime{state: domain.RunState{Status: "failed", Error: "temporary"}},
		[]domain.Agent{{Type: domain.AgentKesha}}, testCallConfig(time.Second))
	settings := testCallConfig(time.Second)
	settings.ActiveMaxRetries = 1
	service.settings = settings

	count, err := service.ReconcileStale(context.Background(), old, old, 10, 1)
	if err != nil || count != 3 || repo.retryCount != 1 || repo.retryMax != 1 {
		t.Fatalf("reconcile = count %d, err %v, retry count/max %d/%d", count, err, repo.retryCount, repo.retryMax)
	}
	if repo.status != "" {
		t.Fatalf("confirmed failed run was marked terminal after successful retry: %s", repo.status)
	}
}

func TestReconcileStalePreservesUnknownAndUnreachableRuns(t *testing.T) {
	for _, test := range []struct {
		name  string
		state domain.RunState
		err   error
	}{
		{name: "status endpoint unavailable", err: errors.New("network unavailable")},
		{name: "unknown run", state: domain.RunState{Status: "unknown"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runID := "unresolved-run"
			repo := &staleCleanupRepository{staleCalls: []domain.Call{{ID: "call-active", Agent: domain.AgentKesha,
				Status: domain.CallRunning, HermesRunID: &runID, UpdatedAt: time.Now().Add(-time.Hour)}}}
			service := NewService(repo, fixedStateRuntime{state: test.state, err: test.err}, []domain.Agent{{Type: domain.AgentKesha}}, testCallConfig(time.Second))
			if _, err := service.ReconcileStale(context.Background(), time.Now(), time.Now(), 10, 1); err != nil {
				t.Fatal(err)
			}
			if repo.retryCount != 0 || repo.status != "" {
				t.Fatalf("unresolved run changed state: retries=%d status=%s", repo.retryCount, repo.status)
			}
		})
	}
}

func TestReconcileStaleDoesNotRetryAfterRetryBudgetOrCancellation(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    string
		retries   int
		wantRetry int
		wantState domain.CallStatus
	}{
		{name: "retry budget exhausted", status: "interrupted", retries: 1, wantState: domain.CallInterrupted},
		{name: "cancelled", status: "cancelled", retries: 0, wantState: domain.CallCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			runID := "terminal-run"
			repo := &staleCleanupRepository{staleCalls: []domain.Call{{ID: "call-terminal", Agent: domain.AgentKesha,
				Status: domain.CallRunning, HermesRunID: &runID, RetryCount: test.retries, UpdatedAt: time.Now().Add(-time.Hour)}}}
			service := NewService(repo, fixedStateRuntime{state: domain.RunState{Status: test.status}}, []domain.Agent{{Type: domain.AgentKesha}}, testCallConfig(time.Second))
			if _, err := service.ReconcileStale(context.Background(), time.Now(), time.Now(), 10, 1); err != nil {
				t.Fatal(err)
			}
			if repo.retryCount != test.wantRetry || repo.status != test.wantState {
				t.Fatalf("retries/status = %d/%s, want %d/%s", repo.retryCount, repo.status, test.wantRetry, test.wantState)
			}
		})
	}
}
