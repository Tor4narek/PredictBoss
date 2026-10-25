package calls

import (
	"context"
	"encoding/json"
	"testing"

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
	status          domain.CallStatus
	statusMessage   string
	historyLimit    int
	startedRunID    string
}

func (r *completionRepository) ListActive(context.Context) ([]domain.Call, error) {
	return r.active, nil
}

func (r *completionRepository) Complete(context.Context, string, string) error {
	return nil
}

func (r *completionRepository) CompleteValera(_ context.Context, _, _, _ string, _ int64) (domain.CompanyProfileSyncState, bool, error) {
	return r.profileState, r.completeValera, nil
}

func (r *completionRepository) RetryProfileSave(_ context.Context, _, _ string, revision int64) (bool, error) {
	r.retryCalls++
	r.retriedRevision = revision
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
			service := NewService(repo, completedRuntime{}, []domain.Agent{{Type: domain.AgentValera}}, 0)

			service.pollActive(context.Background())

			if repo.retryCalls != tt.wantRetryCalls {
				t.Fatalf("retry calls = %d, want %d", repo.retryCalls, tt.wantRetryCalls)
			}
			if tt.wantRetryCalls > 0 && repo.retriedRevision != 4 {
				t.Errorf("retried revision = %d, want 4", repo.retriedRevision)
			}
			if repo.status != tt.wantStatus || repo.statusMessage != tt.wantStatusMessage {
				t.Errorf("status = (%q, %q), want (%q, %q)", repo.status, repo.statusMessage, tt.wantStatus, tt.wantStatusMessage)
			}
		})
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
	service := NewService(repo, runtime, []domain.Agent{{Type: domain.AgentValera, MaxHistory: 0}}, 0)
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

func activeValeraCall(retryCount int) domain.Call {
	return domain.Call{
		ID: "call-1", Agent: domain.AgentValera, ScopeID: "company-1", Status: domain.CallRunning,
		HermesRunID: stringPointer("run-1"), RetryCount: retryCount, Input: json.RawMessage(`{"profile_revision":4}`),
	}
}

func stringPointer(value string) *string { return &value }
