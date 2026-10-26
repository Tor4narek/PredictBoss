package mcpcontext

import (
	"strings"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestSignerBindsContextToAgentCallAndCompany(t *testing.T) {
	instance, err := NewSigner(testTokens())
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}
	signer := instance.(*signer)
	callContext := domain.MCPCallContext{
		Agent: domain.AgentKonstantin, CallID: "call-7", CompanyID: "11111111-1111-4111-8111-111111111111",
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
	}
	token, err := signer.IssueMCPCallContext(callContext)
	if err != nil {
		t.Fatalf("IssueMCPCallContext returned error: %v", err)
	}
	verified, err := signer.VerifyMCPCallContext(token, domain.AgentKonstantin)
	if err != nil || verified.Agent != callContext.Agent || verified.CallID != callContext.CallID || verified.CompanyID != callContext.CompanyID {
		t.Fatalf("VerifyMCPCallContext = %#v, %v; want issued call binding", verified, err)
	}
	if _, err := signer.VerifyMCPCallContext(token, domain.AgentKesha); err == nil {
		t.Fatal("context token was accepted for a different agent")
	}
	body, signature, ok := strings.Cut(token, ".")
	if !ok || body == "" || signature == "" {
		t.Fatal("issued context token has an invalid format")
	}
	first := byte('A')
	if body[0] == first {
		first = 'B'
	}
	tampered := string(first) + body[1:] + "." + signature
	if _, err := signer.VerifyMCPCallContext(tampered, domain.AgentKonstantin); err == nil {
		t.Fatal("tampered context signature was accepted")
	}
}

func TestSignerRejectsExpiredAndOverlongContexts(t *testing.T) {
	instance, err := NewSigner(testTokens())
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}
	signer := instance.(*signer)
	now := time.Now().UTC()
	signer.now = func() time.Time { return now }
	value := domain.MCPCallContext{Agent: domain.AgentKesha, CallID: "call-8", CompanyID: "11111111-1111-4111-8111-111111111111", ExpiresAt: now.Add(time.Minute)}
	token, err := signer.IssueMCPCallContext(value)
	if err != nil {
		t.Fatalf("IssueMCPCallContext returned error: %v", err)
	}
	signer.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := signer.VerifyMCPCallContext(token, domain.AgentKesha); err == nil {
		t.Fatal("expired context token was accepted")
	}
	value.ExpiresAt = now.Add(time.Hour)
	if _, err := signer.IssueMCPCallContext(value); err == nil {
		t.Fatal("context longer than the signing maximum was issued")
	}
}

func testTokens() map[domain.AgentType]string {
	return map[domain.AgentType]string{
		domain.AgentSerezha:    "test-only-serezha-agent-token-0000000001",
		domain.AgentSaveliy:    "test-only-saveliy-agent-token-0000000002",
		domain.AgentValera:     "test-only-valera-agent-token-0000000003",
		domain.AgentKesha:      "test-only-kesha-agent-token-0000000004",
		domain.AgentKonstantin: "test-only-konstantin-agent-token-0000000005",
	}
}
