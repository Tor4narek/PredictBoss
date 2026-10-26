package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestStorageHandlerBindsMcpSessionToAgent(t *testing.T) {
	tokens := map[domain.AgentType]string{
		domain.AgentSerezha:    "unit-test-serezha-token-00000000000000001",
		domain.AgentSaveliy:    "unit-test-saveliy-token-00000000000000002",
		domain.AgentValera:     "unit-test-valera-token-00000000000000003",
		domain.AgentKesha:      "unit-test-kesha-token-00000000000000004",
		domain.AgentKonstantin: "unit-test-konstantin-token-00000000000000005",
	}
	declaredTools := make(map[domain.AgentType][]string, len(agentTools))
	for agent, tools := range agentTools {
		declaredTools[agent] = tools
	}
	handler, err := NewStorageHandler(nil, nil, tokens, declaredTools)
	if err != nil {
		t.Fatal("construct role-filtered MCP handler")
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-auth-test", Version: "1"}, nil)
	saveliyHTTPClient := &http.Client{Transport: testBearerTransport{token: tokens[domain.AgentSaveliy]}}
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: saveliyHTTPClient, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("initialize MCP session: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close MCP session: %v", err)
		}
	})

	requestBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "save_company_profile_and_metrics",
			"arguments": map[string]any{"company_id": "11111111-1111-4111-8111-111111111111"},
		},
	})
	if err != nil {
		t.Fatal("encode MCP role escalation probe")
	}
	request, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal("create MCP role escalation probe")
	}
	request.Header.Set("Authorization", "Bearer "+tokens[domain.AgentValera])
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Session-Id", session.ID())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal("send MCP role escalation probe")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("read MCP role escalation response")
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("a different agent reused the MCP session (status=%d)", response.StatusCode)
	}
	if strings.Contains(string(responseBody), tokens[domain.AgentValera]) || strings.Contains(string(responseBody), "Bearer ") {
		t.Fatal("MCP role escalation response leaked authorization data")
	}
}

type testBearerTransport struct {
	token string
}

func (transport testBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header = request.Header.Clone()
	request.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(request)
}

func TestAuthenticateRequestResolvesOnlyConfiguredAgentTokens(t *testing.T) {
	tokens := map[domain.AgentType]string{
		domain.AgentSerezha:    "unit-test-serezha-token-00000000000000001",
		domain.AgentSaveliy:    "unit-test-saveliy-token-00000000000000002",
		domain.AgentValera:     "unit-test-valera-token-00000000000000003",
		domain.AgentKesha:      "unit-test-kesha-token-00000000000000004",
		domain.AgentKonstantin: "unit-test-konstantin-token-00000000000000005",
	}
	principals, err := authenticateAgentTokens(tokens)
	if err != nil {
		t.Fatal("valid per-agent MCP tokens were rejected")
	}
	for agent, token := range tokens {
		got, ok := authenticateRequest("Bearer "+token, principals)
		if !ok || got != agent {
			t.Errorf("agent token did not resolve to its configured principal")
		}
	}
	for _, header := range []string{"", "Basic invalid", "Bearer invalid-token", "Bearer " + strings.Repeat("x", 257), "Bearer " + "unit-test-domain-token-00000000000000001"} {
		if _, ok := authenticateRequest(header, principals); ok {
			t.Errorf("unconfigured authorization header was accepted")
		}
	}
}

func TestAuthenticateAgentTokensRejectsReuseAndMissingRoles(t *testing.T) {
	tests := []struct {
		name   string
		tokens map[domain.AgentType]string
	}{
		{name: "missing role", tokens: map[domain.AgentType]string{domain.AgentSerezha: "unit-test-serezha-token-00000000000000001"}},
		{name: "short token", tokens: map[domain.AgentType]string{
			domain.AgentSerezha: "short", domain.AgentSaveliy: "unit-test-saveliy-token-00000000000000002",
			domain.AgentValera: "unit-test-valera-token-00000000000000003", domain.AgentKesha: "unit-test-kesha-token-00000000000000004",
			domain.AgentKonstantin: "unit-test-konstantin-token-00000000000000005",
		}},
		{name: "reused token", tokens: map[domain.AgentType]string{
			domain.AgentSerezha: "unit-test-reused-token-00000000000000001", domain.AgentSaveliy: "unit-test-reused-token-00000000000000001",
			domain.AgentValera: "unit-test-valera-token-00000000000000003", domain.AgentKesha: "unit-test-kesha-token-00000000000000004",
			domain.AgentKonstantin: "unit-test-konstantin-token-00000000000000005",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := authenticateAgentTokens(test.tokens); err == nil {
				t.Fatal("unsafe MCP principal configuration was accepted")
			}
		})
	}
}

func TestMCPPolicyMustExactlyMatchAgentDeclaration(t *testing.T) {
	want := []string{"get_company_profile", "get_company_metrics"}
	for _, test := range []struct {
		declared []string
		allowed  bool
	}{
		{declared: []string{"get_company_profile", "get_company_metrics"}, allowed: true},
		{declared: []string{"get_company_profile", "get_company_profile"}},
		{declared: []string{"get_company_profile"}},
		{declared: []string{"get_company_profile", "get_company_metrics", "save_company_profile_and_metrics"}},
	} {
		if got := sameTools(want, test.declared); got != test.allowed {
			t.Errorf("sameTools(%v, %v) = %t, want %t", want, test.declared, got, test.allowed)
		}
	}
}
