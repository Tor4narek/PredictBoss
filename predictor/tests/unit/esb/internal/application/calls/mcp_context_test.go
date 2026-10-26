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

const mcpContextCompanyID = "11111111-1111-4111-8111-111111111111"

type fakeMCPContextSigner struct {
	issued    domain.MCPCallContext
	verified  domain.MCPCallContext
	verifyErr error
}

func (signer *fakeMCPContextSigner) IssueMCPCallContext(value domain.MCPCallContext) (string, error) {
	signer.issued = value
	return "signed-test-context", nil
}

func (signer *fakeMCPContextSigner) VerifyMCPCallContext(token string, agent domain.AgentType) (domain.MCPCallContext, error) {
	if token != "signed-test-context" || signer.verified.Agent != agent {
		return domain.MCPCallContext{}, ErrInvalidMCPCallContext
	}
	return signer.verified, signer.verifyErr
}

type scopedCallRepository struct {
	domain.CallRepository
	call                domain.Call
	err                 error
	recordedCallID      string
	recordedCompanyID   string
	recordedSourceIDs   []string
	recordSourceCallErr error
}

func (repository scopedCallRepository) GetForAgent(_ context.Context, callID string, agent domain.AgentType) (domain.Call, error) {
	if repository.err != nil {
		return domain.Call{}, repository.err
	}
	if callID != repository.call.ID || agent != repository.call.Agent {
		return domain.Call{}, domain.ErrNotFound
	}
	return repository.call, nil
}

func (repository *scopedCallRepository) RecordKonstantinCallSources(_ context.Context, callID, companyID string, sourceIDs []string) error {
	repository.recordedCallID = callID
	repository.recordedCompanyID = companyID
	repository.recordedSourceIDs = append([]string(nil), sourceIDs...)
	return repository.recordSourceCallErr
}

func TestAddMCPCallContextOverridesUntrustedInputField(t *testing.T) {
	input := json.RawMessage(`{"company_id":"` + mcpContextCompanyID + `","call_context":"attacker-value","details":{"preserve":true}}`)
	updated, err := addMCPCallContext(input, "signed-test-context")
	if err != nil {
		t.Fatalf("addMCPCallContext returned error: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(updated), &fields); err != nil {
		t.Fatalf("decode Hermes input: %v", err)
	}
	var gotContext string
	if err := json.Unmarshal(fields["call_context"], &gotContext); err != nil || gotContext != "signed-test-context" {
		t.Fatalf("call_context = %q, want server-issued capability (error %v)", gotContext, err)
	}
	if !strings.Contains(updated, `"company_id":"`+mcpContextCompanyID+`"`) || !strings.Contains(updated, `"preserve":true`) {
		t.Fatalf("attaching context changed other call data: %s", updated)
	}
}

func TestSubmitIssuesMCPContextFromPersistedCallScope(t *testing.T) {
	ctx := context.Background()
	signer := &fakeMCPContextSigner{}
	runtime := &capturingRuntime{}
	repository := &completionRepository{}
	service := NewService(repository, runtime, []domain.Agent{{Type: domain.AgentKesha, ScopeField: "company_id"}}, Config{
		MCPCallContextSigner: signer,
	}, nil)
	call := domain.Call{ID: "call-47", Agent: domain.AgentKesha, ScopeID: mcpContextCompanyID,
		Input: json.RawMessage(`{"company_id":"` + mcpContextCompanyID + `"}`), Status: domain.CallSubmitting}

	service.submit(ctx, call)

	if signer.issued.Agent != call.Agent || signer.issued.CallID != call.ID || signer.issued.CompanyID != call.ScopeID {
		t.Fatalf("issued capability was not derived from persisted call: %#v", signer.issued)
	}
	if signer.issued.ExpiresAt.Before(time.Now().Add(29*time.Minute)) || signer.issued.ExpiresAt.After(time.Now().Add(31*time.Minute)) {
		t.Fatalf("call capability lifetime is not bounded to the active run: %v", signer.issued.ExpiresAt)
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(runtime.input), &input); err != nil {
		t.Fatalf("decode Hermes input: %v", err)
	}
	var callContext string
	if err := json.Unmarshal(input["call_context"], &callContext); err != nil || callContext != "signed-test-context" {
		t.Fatalf("Hermes input is missing the signed per-call context: %q (error %v)", callContext, err)
	}
}

func TestAuthorizeMCPCompanyRequiresMatchingActiveCall(t *testing.T) {
	signer := &fakeMCPContextSigner{verified: domain.MCPCallContext{
		Agent: domain.AgentKesha, CallID: "call-47", CompanyID: mcpContextCompanyID, ExpiresAt: time.Now().Add(time.Minute),
	}}
	ctx := context.Background()

	for _, status := range []domain.CallStatus{domain.CallSubmitting, domain.CallStarted, domain.CallRunning, domain.CallProcessing} {
		activeCall := domain.Call{ID: "call-47", Agent: domain.AgentKesha, ScopeID: mcpContextCompanyID, Status: status}
		service := NewService(&scopedCallRepository{call: activeCall}, nil, nil, Config{MCPCallContextSigner: signer})
		if got, err := service.AuthorizeMCPCompany(ctx, domain.AgentKesha, "signed-test-context", mcpContextCompanyID); err != nil || got.CallID != activeCall.ID {
			t.Fatalf("valid %s call context was rejected: context=%#v error=%v", status, got, err)
		}
	}
	activeCall := domain.Call{ID: "call-47", Agent: domain.AgentKesha, ScopeID: mcpContextCompanyID, Status: domain.CallRunning}
	for _, test := range []struct {
		name      string
		agent     domain.AgentType
		requested string
		call      domain.Call
		verifyErr error
	}{
		{name: "company substitution", agent: domain.AgentKesha, requested: "22222222-2222-4222-8222-222222222222", call: activeCall},
		{name: "terminal call", agent: domain.AgentKesha, requested: mcpContextCompanyID, call: domain.Call{ID: activeCall.ID, Agent: activeCall.Agent, ScopeID: activeCall.ScopeID, Status: domain.CallCompleted}},
		{name: "different agent", agent: domain.AgentSaveliy, requested: mcpContextCompanyID, call: activeCall},
		{name: "expired signature", agent: domain.AgentKesha, requested: mcpContextCompanyID, call: activeCall, verifyErr: errors.New("expired")},
	} {
		t.Run(test.name, func(t *testing.T) {
			signer.verified.Agent = domain.AgentKesha
			signer.verifyErr = test.verifyErr
			repository := scopedCallRepository{call: test.call}
			service := NewService(&repository, nil, nil, Config{MCPCallContextSigner: signer})
			if _, err := service.AuthorizeMCPCompany(ctx, test.agent, "signed-test-context", test.requested); !errors.Is(err, ErrInvalidMCPCallContext) {
				t.Fatalf("unsafe call context error = %v, want ErrInvalidMCPCallContext", err)
			}
		})
	}
}

func TestRecordMCPCallSourcesRequiresActiveKonstantinCall(t *testing.T) {
	activeCall := domain.Call{ID: "call-82", Agent: domain.AgentKonstantin, ScopeID: mcpContextCompanyID, Status: domain.CallRunning}
	repository := &scopedCallRepository{call: activeCall}
	service := NewService(repository, nil, nil, Config{})
	sourceIDs := []string{"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	callContext := domain.MCPCallContext{Agent: activeCall.Agent, CallID: activeCall.ID, CompanyID: activeCall.ScopeID, ExpiresAt: time.Now().Add(time.Minute)}

	if err := service.RecordMCPCallSources(context.Background(), callContext, sourceIDs); err != nil {
		t.Fatalf("RecordMCPCallSources returned error: %v", err)
	}
	if repository.recordedCallID != activeCall.ID || repository.recordedCompanyID != activeCall.ScopeID ||
		len(repository.recordedSourceIDs) != len(sourceIDs) {
		t.Fatalf("recorded sources were not bound to the active call: %#v", repository)
	}
	for index, sourceID := range sourceIDs {
		if repository.recordedSourceIDs[index] != sourceID {
			t.Fatalf("recorded source %d = %q, want %q", index, repository.recordedSourceIDs[index], sourceID)
		}
	}

	for _, test := range []struct {
		name string
		call domain.Call
		ctx  domain.MCPCallContext
	}{
		{name: "non-Konstantin agent", call: activeCall, ctx: domain.MCPCallContext{Agent: domain.AgentKesha, CallID: activeCall.ID, CompanyID: activeCall.ScopeID}},
		{name: "company mismatch", call: activeCall, ctx: domain.MCPCallContext{Agent: activeCall.Agent, CallID: activeCall.ID, CompanyID: "44444444-4444-4444-8444-444444444444"}},
		{name: "terminal call", call: domain.Call{ID: activeCall.ID, Agent: activeCall.Agent, ScopeID: activeCall.ScopeID, Status: domain.CallCompleted}, ctx: callContext},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &scopedCallRepository{call: test.call}
			service := NewService(repository, nil, nil, Config{})
			if err := service.RecordMCPCallSources(context.Background(), test.ctx, sourceIDs); !errors.Is(err, ErrInvalidMCPCallContext) {
				t.Fatalf("RecordMCPCallSources error = %v, want ErrInvalidMCPCallContext", err)
			}
			if repository.recordedCallID != "" {
				t.Fatalf("invalid call context persisted source IDs: %#v", repository.recordedSourceIDs)
			}
		})
	}
}
