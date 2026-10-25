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
		PollInterval: pollInterval, DispatchBatchSize: 100, HistoryCleanupInterval: 24 * time.Hour,
		ContextCompactionTimeout:         30 * time.Second,
		ValeraProfileSyncMaxRetries:      1,
		KonstantinHistoryDefaultPageSize: 100, KonstantinHistoryMaxPageSize: 100,
		KonstantinDefaultWindow: 30 * 24 * time.Hour, KonstantinMaxWindow: 90 * 24 * time.Hour,
		KonstantinEventPollInterval: 300 * time.Millisecond, MaxQuestionBytes: 1 << 20,
	}
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
	key     string
	history []domain.Message
}

func (r *capturingRuntime) Start(_ context.Context, _ domain.Agent, _ string, history []domain.Message, key string) (string, error) {
	r.key, r.history = key, history
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

func (r *contextRepositoryFake) PruneHistory(context.Context, domain.AgentType, int) error {
	return nil
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
