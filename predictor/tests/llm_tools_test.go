package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type observedToolCall struct {
	Tool          string `json:"tool"`
	Agent         string `json:"agent"`
	CorrelationID string `json:"correlation_id"`
}

const llmSafeInsufficientEvidence = "Недостаточно подтверждённых источников в данных этого вызова, чтобы дать подтверждённую рекомендацию."

func TestRealLLMProfileMCPCalls(t *testing.T) {
	if os.Getenv("ESB_LLM_TOOLS_ENABLED") != "1" {
		t.Skip("opt-in test; run make test-llm-tools to use isolated services and a configured LLM key")
	}
	baseURL := strings.TrimRight(os.Getenv("ESB_E2E_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:18081"
	}
	observerURL := strings.TrimRight(os.Getenv("ESB_LLM_TOOLS_OBSERVER_URL"), "/")
	if observerURL == "" {
		t.Fatal("MCP observer URL was not configured")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	waitForReadiness(t, client, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sessions := connectAgentMCPs(t, ctx, baseURL)
	defer sessions.close(t)
	companyID := llmTestCompanyID()
	newsID := llmTestNewsID()
	sourceID := llmTestSourceID()
	factID := seedLLMTestCompany(t, ctx, client, baseURL, sessions, companyID, newsID, sourceID)

	for _, agent := range []string{"saveliy", "kesha", "konstantin"} {
		agent := agent
		t.Run(agent, func(t *testing.T) {
			correlationID := os.Getenv("LLM_TEST_CORRELATION_ID_" + strings.ToUpper(agent))
			if correlationID == "" {
				t.Fatal("test correlation ID was not configured")
			}
			before := observedCalls(t, client, observerURL, correlationID)

			switch agent {
			case "saveliy":
				body := map[string]any{
					"company_id": companyID,
					"news_id":    newsID,
					"facts": []any{map[string]any{
						"fact_id":   factID,
						"fact_text": "The synthetic company operates three factories assembling copper widgets for industrial customers.",
					}},
				}
				call := assertRequest(t, client, baseURL, http.MethodPost, "/call?agent=saveliy", body, true, http.StatusAccepted, "")
				waitForLLMCall(t, client, baseURL, "saveliy", stringField(t, call.body, "call_id"))
				after := observedCalls(t, client, observerURL, correlationID)
				logObservedToolCalls(t, agent, after[len(before):])
				assertProfileToolCall(t, after[len(before):], agent, correlationID, "save_fact_company_links")
				linked := callTool(t, ctx, sessions, "get_company_facts", map[string]any{"company_id": companyID, "limit": 10})
				if len(arrayField(t, linked, "facts")) != 1 {
					t.Fatal("Saveliy completed without persisting the expected company-fact link")
				}

			case "kesha":
				call := assertRequest(t, client, baseURL, http.MethodPost, "/call?agent=kesha", map[string]any{
					"company_id": companyID, "period_from": "2026-01-01", "period_to": "2027-01-01",
				}, true, http.StatusAccepted, "")
				waitForLLMCall(t, client, baseURL, "kesha", stringField(t, call.body, "call_id"))
				after := observedCalls(t, client, observerURL, correlationID)
				logObservedToolCalls(t, agent, after[len(before):])
				events := after[len(before):]
				assertProfileToolCall(t, events, agent, correlationID, "")
				if !hasObservedToolCall(events, "save_company_analysis") {
					t.Fatal("Kesha completed without calling save_company_analysis through ESB")
				}

			case "konstantin":
				processing := performRequest(t, client, baseURL, http.MethodGet, "/call?agent=konstantin",
					map[string]any{"companyId": companyID}, true)
				if processing.status != http.StatusAccepted && processing.status != http.StatusOK {
					t.Fatalf("Konstantin stream subscription returned HTTP %d", processing.status)
				}
				streamURL := stringField(t, processing.body, "streamUrl")
				if streamURL == "" {
					t.Fatal("Konstantin GET /call response did not include the stream URL")
				}
				ws := dialWebSocket(t, baseURL+streamURL)
				analysis := readConsultantEvent(t, ws, "analysis_result", 5*time.Minute)
				if !hasLLMEvidence(analysis["evidence"], sourceID) {
					t.Errorf("real Konstantin analysis omitted the current source ID")
					assertSafeInsufficientEvidence(t, analysis)
				}
				initialEvents := observedCalls(t, client, observerURL, correlationID)
				logObservedToolCalls(t, agent+" analysis", initialEvents[len(before):])
				assertProfileToolCall(t, initialEvents[len(before):], agent, correlationID, "")
				analysisID := stringField(t, analysis, "analysisId")
				conversationID := stringField(t, analysis, "conversationId")
				beforeQuestion := observedCalls(t, client, observerURL, correlationID)
				requestID := "cccccccc-0000-4000-8000-000000000001"
				writeFrame(t, ws, map[string]any{
					"kind": "question", "requestId": requestID, "companyId": companyID,
					"conversationId": conversationID, "analysisId": analysisID,
					"text": "Using the saved company profile and the reported 125 RUB per widget price, explain whether this source supports the configured price metric. Cite the exact source_id from ESB in the required evidence array and return the required JSON object.",
				})
				readConsultantEvent(t, ws, "accepted", 30*time.Second)
				answer := readConsultantEvent(t, ws, "assistant_answer", 5*time.Minute)
				_ = ws.Close(websocket.StatusNormalClosure, "")
				if stringField(t, answer, "requestId") != requestID {
					t.Errorf("Konstantin WebSocket answer is missing its request ID")
				}
				if !hasLLMEvidence(answer["evidence"], sourceID) {
					t.Errorf("real Konstantin answer omitted the current source ID")
					assertSafeInsufficientEvidence(t, answer)
				}
				afterQuestion := observedCalls(t, client, observerURL, correlationID)
				logObservedToolCalls(t, agent+" question", afterQuestion[len(beforeQuestion):])
				assertProfileToolCall(t, afterQuestion[len(beforeQuestion):], agent, correlationID, "")
				status := assertRequest(t, client, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{
					"companyId": companyID, "historyLimit": 10,
				}, true, http.StatusOK, "")
				if len(arrayField(t, status.body, "history")) < 2 {
					t.Fatal("Konstantin WebSocket interaction was not persisted in PostgreSQL chat history")
				}
			}
		})
	}
}

func hasLLMEvidence(raw any, expectedSourceID string) bool {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var evidence []struct {
		SourceID   string `json:"sourceId"`
		OriginKind string `json:"originKind"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		return false
	}
	for _, item := range evidence {
		if item.SourceID == expectedSourceID && item.OriginKind == "fact" {
			return true
		}
	}
	return false
}

func assertSafeInsufficientEvidence(t *testing.T, answer map[string]any) {
	t.Helper()
	if stringField(t, answer, "text") != llmSafeInsufficientEvidence || len(arrayField(t, answer, "evidence")) != 0 {
		t.Errorf("ESB did not replace unsupported real output with the safe insufficient-evidence response")
	}
}

func hasObservedToolCall(events []observedToolCall, name string) bool {
	for _, event := range events {
		if event.Tool == name {
			return true
		}
	}
	return false
}

func seedLLMTestCompany(t *testing.T, ctx context.Context, client *http.Client, baseURL string,
	sessions *mcpSessions, companyID, newsID, sourceID string) string {
	t.Helper()
	profileCall := startHeldMCPCall(t, client, baseURL, sessions, "valera", companyID, map[string]any{
		"companyId": companyID, "profileRevision": 1,
		"company": map[string]any{"syntheticU": "U_80", "orgType": "industrial", "city": "Ekaterinburg"},
	})
	callToolAs(t, ctx, sessions, "valera", "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1,
		"markdown": "# Synthetic Copper Widget Company\n\nThe company manufactures copper widgets for industrial equipment. Its configured price metric tracks the selling price of one copper widget.\n",
		"metrics": []any{map[string]any{
			"type_code": "цена", "name": "цена", "description": "Selling price of one copper widget",
			"metric_type": "currency", "characteristic": "one copper widget", "unit": "RUB/widget",
		}},
	})
	finishHeldMCPCall(t, client, baseURL, sessions, profileCall)
	sourceCall := assertRequest(t, client, baseURL, http.MethodPost, "/call?agent=serezha", map[string]any{
		"company_id": companyID, "news_id": newsID, "source_id": sourceID,
		"source_url": "https://example.test/synthetic-widget-news", "material_kind": "external_news",
		"media_type": "text/html", "title": "Synthetic widget factory report",
		"published_at": "2026-10-01T12:00:00Z",
		"news_text":    "Synthetic test data: on 2026-10-01 the company sold copper widgets for 125 RUB per widget.",
	}, true, http.StatusAccepted, "")
	waitForCall(t, client, baseURL, "serezha", stringField(t, sourceCall.body, "call_id"))
	news := callToolAs(t, ctx, sessions, "saveliy", "get_facts", map[string]any{"news_id": newsID, "limit": 10})
	facts := arrayField(t, news, "facts")
	if len(facts) != 1 {
		t.Fatalf("isolated news fixture returned %d facts, want one", len(facts))
	}
	factID := stringField(t, facts[0].(map[string]any), "fact_id")
	if factID == "" {
		t.Fatal("isolated news fixture returned an empty fact ID")
	}
	return factID
}

func observedCalls(t *testing.T, client *http.Client, observerURL, correlationID string) []observedToolCall {
	t.Helper()
	requestURL := observerURL + "/events?correlation_id=" + url.QueryEscape(correlationID)
	response, err := client.Get(requestURL)
	if err != nil {
		t.Fatalf("read sanitized MCP observer events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("MCP observer returned HTTP %d", response.StatusCode)
	}
	var body struct {
		Events []observedToolCall `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode sanitized MCP observer events: %v", err)
	}
	return body.Events
}

func assertProfileToolCall(t *testing.T, events []observedToolCall, agent, correlationID, laterTool string) {
	t.Helper()
	profileIndex := -1
	laterIndex := -1
	for index, event := range events {
		if event.Agent != agent || event.CorrelationID != correlationID {
			t.Fatalf("MCP observer event is not correlated to the active %s scenario", agent)
		}
		if event.Tool == "get_company_profile" && profileIndex < 0 {
			profileIndex = index
		}
		if event.Tool == laterTool && laterIndex < 0 {
			laterIndex = index
		}
	}
	if profileIndex < 0 {
		t.Fatalf("Hermes did not call get_company_profile for %s (observed %d tool calls)", agent, len(events))
	}
	if laterTool != "" && (laterIndex < 0 || profileIndex >= laterIndex) {
		t.Fatalf("%s did not read the profile before %s", agent, laterTool)
	}
}

func logObservedToolCalls(t *testing.T, scenario string, events []observedToolCall) {
	t.Helper()
	tools := make([]string, 0, len(events))
	for _, event := range events {
		tools = append(tools, event.Tool)
	}
	t.Logf("observed MCP tools for %s: %s", scenario, strings.Join(tools, ", "))
}

func waitForReadiness(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequest(http.MethodGet, baseURL+"/readyz", nil)
		if err != nil {
			t.Fatal("create readiness request")
		}
		request.Header.Set("Authorization", "Bearer "+apiKey)
		response, err := client.Do(request)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("isolated ESB readiness did not pass")
		case <-ticker.C:
		}
	}
}

func waitForLLMCall(t *testing.T, client *http.Client, baseURL, agent, callID string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		result := performRequest(t, client, baseURL, http.MethodGet, "/call?agent="+url.QueryEscape(agent),
			map[string]any{"call_id": callID}, true)
		if result.status == http.StatusOK {
			switch stringField(t, result.body, "status") {
			case "completed":
				return
			case "failed", "cancelled", "interrupted":
				t.Fatalf("real %s call ended with status %s", agent, result.body["status"])
			}
		} else {
			t.Fatalf("read real %s call status returned HTTP %d", agent, result.status)
		}
		select {
		case <-deadline.C:
			t.Fatalf("real %s call %s did not complete before the test deadline", agent, callID)
		case <-ticker.C:
		}
	}
}

func readConsultantEvent(t *testing.T, conn *websocket.Conn, kind string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		_, payload, err := conn.Read(ctx)
		cancel()
		if err != nil {
			t.Fatalf("read Konstantin WebSocket event: %v", err)
		}
		var event map[string]any
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal("decode Konstantin WebSocket event")
		}
		if event["kind"] == kind {
			return event
		}
		if event["kind"] == "error" {
			t.Fatalf("Konstantin WebSocket returned an error event: %s", fmt.Sprint(event["error"]))
		}
		select {
		case <-deadline.C:
			t.Fatalf("Konstantin did not emit %s before the test deadline", kind)
		default:
		}
	}
}

func llmTestCompanyID() string {
	return "dddddddd-0000-4000-8000-000000000001"
}

func llmTestNewsID() string {
	return "dddddddd-0000-4000-8000-000000000002"
}

func llmTestSourceID() string {
	return "dddddddd-0000-4000-8000-000000000003"
}
