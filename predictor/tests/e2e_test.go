package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	apiKey       = "test-esb-api-key"
	companyID    = "11111111-1111-4111-8111-111111111111"
	newsSourceID = "22222222-2222-4222-8222-222222222222"
	bankSourceID = "33333333-3333-4333-8333-333333333333"
	newSourceID  = "44444444-4444-4444-8444-444444444444"
)

type response struct {
	status int
	body   map[string]any
	raw    []byte
}

func TestESBEndToEndHTTPWebSocketAndMCP(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ESB_E2E_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:18081"
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	waitForHealth(t, httpClient, baseURL)

	// Public health and protected readiness checks.
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/healthz", nil, false, http.StatusOK, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/readyz", nil, false, http.StatusUnauthorized, "unauthorized")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/readyz", nil, true, http.StatusOK, "")
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=", map[string]any{}, true, http.StatusBadRequest, "")
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=missing", map[string]any{"company_id": companyID}, true, http.StatusBadRequest, "")
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=serezha", map[string]any{"company_id": companyID}, true, http.StatusBadRequest, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", nil, true, http.StatusBadRequest, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/stream?agent=konstantin", nil, true, http.StatusBadRequest, "stream_does_not_accept_agent")

	// Connect to the real Streamable HTTP MCP endpoint and enumerate every tool.
	mcpCtx, cancelMCP := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancelMCP()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "predictor-esb-e2e", Version: "1.0.0"}, nil)
	mcpHTTP := &http.Client{Transport: bearerTransport{key: apiKey, base: http.DefaultTransport}}
	mcpSession, err := mcpClient.Connect(mcpCtx, &mcp.StreamableClientTransport{
		Endpoint: baseURL + "/mcp", HTTPClient: mcpHTTP, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect to MCP endpoint: %v", err)
	}
	defer mcpSession.Close()
	tools, err := mcpSession.ListTools(mcpCtx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	wantTools := []string{
		"get_company_profile", "save_company_profile_and_metrics", "save_news_and_facts", "get_facts",
		"save_fact_company_links", "get_company_facts", "save_company_metrics", "get_company_metrics",
		"save_metric_observations", "calculate_metric_total", "calculate_all_metric_totals",
		"save_company_analysis", "get_company_report", "get_consultant_context",
	}
	toolNames := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range wantTools {
		if !toolNames[name] {
			t.Errorf("MCP tools/list is missing %q", name)
		}
	}

	// Valera profile call: enqueue, read its status, atomically persist profile+metrics over MCP.
	profilePayload := valeraPayload(1)
	valera := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", profilePayload, true, http.StatusAccepted, "")
	valeraCallID := stringField(t, valera.body, "callId")
	waitForCall(t, httpClient, baseURL, "valera", valeraCallID)
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{"companyId": companyID}, true, http.StatusConflict, "profile_update_pending")
	metric := metricSetting()
	profileMarkdown := "# Example company\n\nA manufacturer with a growing domestic market.\n"
	savedProfile := callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1, "markdown": profileMarkdown,
		"metrics": []any{metric},
	})
	if got := stringField(t, savedProfile, "profile_key"); got != "companies/"+companyID+"/profile.md" {
		t.Fatalf("unexpected S3 profile key %q", got)
	}
	if got := intField(t, savedProfile, "metrics_saved"); got != 1 {
		t.Fatalf("expected one saved metric, got %d", got)
	}
	// Exact retries are safe; a changed payload at the same revision is rejected.
	callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1, "markdown": profileMarkdown,
		"metrics": []any{metric},
	})
	changedProfile := callToolRaw(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1, "markdown": "# Changed at same revision\n",
		"metrics": []any{metric},
	})
	if !changedProfile.IsError {
		t.Fatal("MCP accepted a conflicting profile payload at the same revision")
	}
	profile := callTool(t, mcpCtx, mcpSession, "get_company_profile", map[string]any{"company_id": companyID})
	if !strings.Contains(stringField(t, profile, "markdown"), "Example company") {
		t.Fatal("S3-compatible profile read did not return the saved Markdown")
	}
	waitForKonstantinReady(t, httpClient, baseURL, companyID)
	callTool(t, mcpCtx, mcpSession, "save_company_metrics", map[string]any{"company_id": companyID, "metrics": []any{metric}})
	metrics := callTool(t, mcpCtx, mcpSession, "get_company_metrics", map[string]any{"company_id": companyID, "limit": 100})
	if len(arrayField(t, metrics, "metrics")) != 1 {
		t.Fatal("PostgreSQL metric settings were not returned through MCP")
	}
	profileRetry := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", profilePayload, true, http.StatusAccepted, "")
	if !boolField(t, profileRetry.body, "reused") {
		t.Fatal("same Valera profile revision was not idempotent")
	}
	changedRevision := valeraPayload(1)
	changedRevision["company"] = map[string]any{"city": "different payload"}
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", changedRevision, true, http.StatusConflict, "profile_revision_conflict")

	// News intake uses Domain metadata and queues Serezha. Both source kinds are covered.
	newsCall := postNews(t, httpClient, baseURL, newsSourceID, "Cleaned external news text.")
	if stringField(t, newsCall.body, "sourceId") != newsSourceID {
		t.Fatal("POST /news returned the wrong source ID")
	}
	waitForCall(t, httpClient, baseURL, "serezha", stringField(t, newsCall.body, "callId"))
	bankCall := postNews(t, httpClient, baseURL, bankSourceID, "Cleaned Alpha analysis text.")
	waitForCall(t, httpClient, baseURL, "serezha", stringField(t, bankCall.body, "callId"))
	duplicateNews := postNews(t, httpClient, baseURL, newsSourceID, "Cleaned external news text.")
	if !boolField(t, duplicateNews.body, "reused") || stringField(t, duplicateNews.body, "callId") != stringField(t, newsCall.body, "callId") {
		t.Fatal("repeated sourceId created a duplicate call")
	}
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/news", map[string]any{"sourceId": "99999999-9999-4999-8999-999999999999", "text": "unknown"}, true, http.StatusNotFound, "news_source_not_found")
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/news", map[string]any{"sourceId": newsSourceID, "text": strings.Repeat("x", (1<<20)+1)}, true, http.StatusBadRequest, "invalid_news_request")

	// Exercise all ClickHouse-facing MCP tools against the real ClickHouse service.
	newsFacts := saveNewsFacts(t, mcpCtx, mcpSession, newsSourceID, "external_news", "text/html", "Market change", "https://example.test/news/market-change", "A market shift affects manufacturers.")
	bankFacts := saveNewsFacts(t, mcpCtx, mcpSession, bankSourceID, "alpha_analysis", "application/pdf", "Alpha market analysis", "https://example.test/alpha/market-analysis.pdf", "Costs are likely to change.")
	factIDs := append(newsFacts, bankFacts...)
	links := make([]any, 0, len(factIDs))
	for _, factID := range factIDs {
		links = append(links, map[string]any{"fact_id": factID, "relevance": "high", "confidence": 0.95})
	}
	callTool(t, mcpCtx, mcpSession, "save_fact_company_links", map[string]any{"company_id": companyID, "links": links})
	facts := callTool(t, mcpCtx, mcpSession, "get_facts", map[string]any{"news_id": newsSourceID, "limit": 100})
	if len(arrayField(t, facts, "facts")) != 1 {
		t.Fatalf("expected one fact for news source, got %d", len(arrayField(t, facts, "facts")))
	}
	companyFacts := callTool(t, mcpCtx, mcpSession, "get_company_facts", map[string]any{"company_id": companyID, "limit": 100})
	if len(arrayField(t, companyFacts, "facts")) != 2 {
		t.Fatalf("expected two linked company facts, got %d", len(arrayField(t, companyFacts, "facts")))
	}
	observations := []any{
		map[string]any{"company_id": companyID, "metric_id": "revenue_impact", "news_id": newsSourceID, "value": 2000000.0, "probability": 0.8, "period_from": "2026-10-01", "period_to": "2026-10-31", "source_fact_ids": []string{factIDs[0]}},
		map[string]any{"company_id": companyID, "metric_id": "revenue_impact", "news_id": bankSourceID, "value": 500000.0, "probability": 0.6, "period_from": "2026-10-01", "period_to": "2026-10-31", "source_fact_ids": []string{factIDs[1]}},
	}
	callTool(t, mcpCtx, mcpSession, "save_metric_observations", map[string]any{"company_id": companyID, "observations": observations})
	totalArgs := map[string]any{"company_id": companyID, "metric_id": "revenue_impact", "period_from": "2026-10-01", "period_to": "2026-10-31"}
	total := callTool(t, mcpCtx, mcpSession, "calculate_metric_total", totalArgs)
	if got := floatField(t, total, "total"); math.Abs(got-1900000) > 0.01 {
		t.Fatalf("probability-weighted total = %v, want 1900000", got)
	}
	totals := callTool(t, mcpCtx, mcpSession, "calculate_all_metric_totals", map[string]any{"company_id": companyID, "period_from": "2026-10-01", "period_to": "2026-10-31"})
	if len(arrayField(t, totals, "totals")) != 1 {
		t.Fatal("calculate_all_metric_totals did not return the configured metric")
	}
	callTool(t, mcpCtx, mcpSession, "save_company_analysis", map[string]any{
		"company_id": companyID, "period_from": "2026-10-01", "period_to": "2026-10-31",
		"probability": 0.9, "metric_ids": []string{"revenue_impact"}, "text": "The evidence suggests a moderate revenue impact.",
	})
	companyReport := callTool(t, mcpCtx, mcpSession, "get_company_report", map[string]any{"company_id": companyID, "period_from": "2026-10-01", "period_to": "2026-10-31"})
	if len(arrayField(t, companyReport, "facts")) != 2 || len(arrayField(t, companyReport, "metrics")) != 1 || len(arrayField(t, companyReport, "totals")) == 0 || len(arrayField(t, companyReport, "analyses")) != 1 {
		t.Fatalf("company report is missing ClickHouse or PostgreSQL data: %#v", companyReport)
	}
	consultantContext := callTool(t, mcpCtx, mcpSession, "get_consultant_context", map[string]any{"company_id": companyID, "limit": 100})
	if len(arrayField(t, consultantContext, "facts")) != 2 || len(arrayField(t, consultantContext, "observations")) != 2 || len(arrayField(t, consultantContext, "totals")) == 0 {
		t.Fatalf("consultant context is incomplete: %#v", consultantContext)
	}

	// Exercise the ordinary asynchronous calls and call-status GET for each remaining HTTP agent.
	savelyCall := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=saveliy", map[string]any{"company_id": companyID, "news_id": newsSourceID}, true, http.StatusAccepted, "")
	waitForCall(t, httpClient, baseURL, "saveliy", stringField(t, savelyCall.body, "call_id"))
	keshaCall := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=kesha", map[string]any{"company_id": companyID}, true, http.StatusAccepted, "")
	waitForCall(t, httpClient, baseURL, "kesha", stringField(t, keshaCall.body, "call_id"))
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=kesha", map[string]any{"companyId": companyID, "period_from": "2026-10-01", "period_to": "2026-10-31"}, true, http.StatusOK, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=saveliy", map[string]any{"call_id": stringField(t, savelyCall.body, "call_id")}, true, http.StatusOK, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=kesha", map[string]any{"call_id": stringField(t, keshaCall.body, "call_id")}, true, http.StatusOK, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=kesha", map[string]any{"call_id": stringField(t, savelyCall.body, "call_id")}, true, http.StatusNotFound, "call_not_found")

	// POST starts analysis; GET by company is the subscription/status operation.
	konstantinCall := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=konstantin", map[string]any{"companyId": companyID}, true, http.StatusAccepted, "")
	konstantinCallID := stringField(t, konstantinCall.body, "call_id")
	waitForCall(t, httpClient, baseURL, "konstantin", konstantinCallID)
	analysisStatus := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{"companyId": companyID}, true, http.StatusOK, "")
	if stringField(t, analysisStatus.body, "status") != "ready" {
		t.Fatalf("expected ready analysis after completed call, got %#v", analysisStatus.body)
	}
	if !strings.HasPrefix(stringField(t, analysisStatus.body, "streamUrl"), "/stream?") {
		t.Fatalf("GET /call did not provide /stream subscription URL: %#v", analysisStatus.body)
	}
	analysisID := stringField(t, analysisStatus.body, "analysisId")
	conversationID := stringField(t, analysisStatus.body, "conversationId")
	analysisMessageID := stringField(t, analysisStatus.body, "messageId")
	if !strings.Contains(stringField(t, analysisStatus.body, "text"), "E2E analysis") {
		t.Fatalf("GET /call did not return the full saved analysis: %#v", analysisStatus.body)
	}
	assertEvidence(t, analysisStatus.body["evidence"], newsSourceID, bankSourceID)

	streamURL := baseURL + stringField(t, analysisStatus.body, "streamUrl")
	ws := dialWebSocket(t, streamURL)
	analysisFrame := readUntilAnalysis(t, ws, analysisID)
	if stringField(t, analysisFrame, "kind") != "analysis_result" {
		t.Fatalf("WebSocket did not replay the persisted analysis: %#v", analysisFrame)
	}

	// A user question is accepted and answered only over the WebSocket.
	questionRequestID := "77777777-7777-4777-8777-777777777777"
	question := map[string]any{"kind": "question", "requestId": questionRequestID, "companyId": companyID,
		"conversationId": conversationID, "analysisId": analysisID, "text": "Why is this news relevant?"}
	writeFrame(t, ws, question)
	accepted := readFrame(t, ws)
	if stringField(t, accepted, "kind") != "accepted" || stringField(t, accepted, "requestId") != questionRequestID {
		t.Fatalf("WebSocket did not acknowledge question: %#v", accepted)
	}
	answer := readFrame(t, ws)
	if stringField(t, answer, "kind") != "assistant_answer" || stringField(t, answer, "requestId") != questionRequestID {
		t.Fatalf("WebSocket did not return a complete answer: %#v", answer)
	}
	answerMessageID := stringField(t, answer, "messageId")
	assertEvidence(t, answer["evidence"], newsSourceID, bankSourceID)
	_ = ws.Close(websocket.StatusNormalClosure, "")

	// GET /call returns paginated company-scoped chat history from PostgreSQL.
	historyFrom := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	historyTo := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	historyResponse := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin",
		map[string]any{"companyId": companyID, "historyLimit": 2, "historyFrom": historyFrom, "historyTo": historyTo}, true, http.StatusOK, "")
	history := arrayField(t, historyResponse.body, "history")
	if len(history) != 2 || !boolField(t, historyResponse.body, "historyHasMore") {
		t.Fatalf("GET /call did not return the latest page of chat history: %#v", historyResponse.body)
	}
	latestQuestion, ok := history[0].(map[string]any)
	if !ok || stringField(t, latestQuestion, "role") != "user" || stringField(t, latestQuestion, "text") != "Why is this news relevant?" {
		t.Fatalf("chat history did not include the user's question: %#v", history[0])
	}
	latestAnswer, ok := history[1].(map[string]any)
	if !ok || stringField(t, latestAnswer, "role") != "assistant" || stringField(t, latestAnswer, "messageId") != answerMessageID {
		t.Fatalf("chat history did not include the assistant's answer: %#v", history[1])
	}
	olderHistory := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin",
		map[string]any{"companyId": companyID, "historyLimit": 2, "historyFrom": historyFrom, "historyTo": historyTo,
			"beforeMessageId": stringField(t, historyResponse.body, "historyNextBeforeMessageId")},
		true, http.StatusOK, "")
	olderMessages := arrayField(t, olderHistory.body, "history")
	if len(olderMessages) != 2 || boolField(t, olderHistory.body, "historyHasMore") {
		t.Fatalf("chat history pagination did not return the earlier analysis messages: %#v", olderHistory.body)
	}
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin",
		map[string]any{"companyId": companyID, "beforeMessageId": "not-a-uuid"}, true, http.StatusBadRequest, "invalid_history_request")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin",
		map[string]any{"companyId": companyID, "historyFrom": time.Now().UTC().Add(-91 * 24 * time.Hour).Format(time.RFC3339Nano),
			"historyTo": time.Now().UTC().Format(time.RFC3339Nano)}, true, http.StatusBadRequest, "invalid_history_request")

	// Reconnect from the analysis cursor and verify that the saved answer is replayed.
	replayURL := streamURL + "&afterMessageId=" + url.QueryEscape(analysisMessageID)
	replayWS := dialWebSocket(t, replayURL)
	replayedAnswer := readFrame(t, replayWS)
	if stringField(t, replayedAnswer, "messageId") != answerMessageID {
		t.Fatalf("WebSocket replay returned the wrong message: %#v", replayedAnswer)
	}
	_ = replayWS.Close(websocket.StatusNormalClosure, "")

	// Same request ID is idempotent; on a new connection, the stored response is replayed.
	duplicateWS := dialWebSocket(t, streamURL+"&afterMessageId="+url.QueryEscape(answerMessageID))
	writeFrame(t, duplicateWS, question)
	duplicateAccepted := readFrame(t, duplicateWS)
	if stringField(t, duplicateAccepted, "kind") != "accepted" || !boolField(t, duplicateAccepted, "reused") {
		t.Fatalf("same WebSocket requestId was not idempotent: %#v", duplicateAccepted)
	}
	duplicateAnswer := readFrame(t, duplicateWS)
	if stringField(t, duplicateAnswer, "messageId") != answerMessageID {
		t.Fatalf("idempotent retry returned a different message: %#v", duplicateAnswer)
	}
	_ = duplicateWS.Close(websocket.StatusNormalClosure, "")

	// New source and profile revisions independently invalidate saved analysis.
	newSourceCall := postNews(t, httpClient, baseURL, newSourceID, "A newly published source.")
	waitForCall(t, httpClient, baseURL, "serezha", stringField(t, newSourceCall.body, "callId"))
	staleQuestionWS := dialWebSocket(t, streamURL+"&afterMessageId="+url.QueryEscape(answerMessageID))
	writeFrame(t, staleQuestionWS, map[string]any{"kind": "question", "requestId": "88888888-8888-4888-8888-888888888888",
		"companyId": companyID, "conversationId": conversationID, "analysisId": analysisID, "text": "Use the stale analysis"})
	staleError := readFrame(t, staleQuestionWS)
	if stringField(t, staleError, "error") != "analysis_not_ready" {
		t.Fatalf("stale analysis was allowed for a new question: %#v", staleError)
	}
	_ = staleQuestionWS.Close(websocket.StatusNormalClosure, "")
	newSourceStatus := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{"companyId": companyID}, true, http.StatusAccepted, "")
	newSourceURL := baseURL + stringField(t, newSourceStatus.body, "streamUrl")
	newSourceWS := dialWebSocket(t, newSourceURL)
	newSourceAnalysis := readUntilRequest(t, newSourceWS, stringField(t, newSourceStatus.body, "requestId"), "analysis_result")
	if stringField(t, newSourceAnalysis, "analysisId") == analysisID {
		t.Fatal("new sourceId did not invalidate the previous analysis")
	}
	_ = newSourceWS.Close(websocket.StatusNormalClosure, "")

	updatedProfilePayload := valeraPayload(2)
	updatedValera := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", updatedProfilePayload, true, http.StatusAccepted, "")
	waitForCall(t, httpClient, baseURL, "valera", stringField(t, updatedValera.body, "callId"))
	updatedProfileMarkdown := "# Example company\n\nUpdated company context at revision two.\n"
	callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 2, "markdown": updatedProfileMarkdown,
		"metrics": []any{metric},
	})
	profileStaleStatus := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{"companyId": companyID}, true, http.StatusAccepted, "")
	profileRevisionURL := baseURL + stringField(t, profileStaleStatus.body, "streamUrl")
	profileRevisionWS := dialWebSocket(t, profileRevisionURL)
	profileRevisionAnalysis := readUntilRequest(t, profileRevisionWS, stringField(t, profileStaleStatus.body, "requestId"), "analysis_result")
	if stringField(t, profileRevisionAnalysis, "analysisId") == stringField(t, newSourceAnalysis, "analysisId") {
		t.Fatal("profileRevision did not invalidate the previous analysis")
	}
	_ = profileRevisionWS.Close(websocket.StatusNormalClosure, "")

	// Invalid replay cursor is reported as a WebSocket protocol error.
	badCursorWS := dialWebSocket(t, streamURL+"&afterMessageId=not-a-uuid")
	badCursor := readFrame(t, badCursorWS)
	if stringField(t, badCursor, "error") != "invalid_replay_cursor" {
		t.Fatalf("invalid replay cursor was not rejected: %#v", badCursor)
	}
	_ = badCursorWS.Close(websocket.StatusNormalClosure, "")
}

func valeraPayload(revision int) map[string]any {
	return map[string]any{
		"companyId": companyID, "profileRevision": revision,
		"company": map[string]any{"syntheticU": "U_80", "orgType": "КОММ_ОРГ", "city": "Екатеринбург", "revenueCompany": 1423218135},
	}
}

func metricSetting() map[string]any {
	return map[string]any{
		"metric_id": "revenue_impact", "name": "Revenue impact", "description": "Expected revenue change",
		"metric_type": "currency", "characteristic": "Probability-weighted change in revenue", "unit": "RUB",
		"parameters": map[string]any{"currency": "RUB"},
	}
}

func postNews(t *testing.T, client *http.Client, baseURL, sourceID, text string) response {
	t.Helper()
	return assertRequest(t, client, baseURL, http.MethodPost, "/news", map[string]any{"sourceId": sourceID, "text": text}, true, http.StatusAccepted, "")
}

func saveNewsFacts(t *testing.T, ctx context.Context, session *mcp.ClientSession, sourceID, materialKind, mediaType, title, sourceURL, text string) []string {
	t.Helper()
	result := callTool(t, ctx, session, "save_news_and_facts", map[string]any{
		"news_id": sourceID, "source_id": sourceID, "source_url": sourceURL, "company_id": companyID,
		"material_kind": materialKind, "media_type": mediaType, "title": title,
		"published_at": "2026-10-25T09:00:00Z", "news_text": text,
		"facts": []any{map[string]any{"fact_text": text, "category": "market", "entities": []string{"manufacturing"},
			"evidence": "source text", "occurred_at": "2026-10-25T09:00:00Z", "confidence": 0.9}},
	})
	ids := stringArrayField(t, result, "fact_ids")
	if len(ids) != 1 {
		t.Fatalf("save_news_and_facts returned %d fact IDs, want one", len(ids))
	}
	return ids
}

func assertEvidence(t *testing.T, value any, sourceIDs ...string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	var evidence []map[string]any
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode evidence %s: %v", encoded, err)
	}
	got := map[string]string{}
	for _, entry := range evidence {
		got[fmt.Sprint(entry["sourceId"])] = fmt.Sprint(entry["originKind"])
	}
	for _, id := range sourceIDs {
		if _, ok := got[id]; !ok {
			t.Errorf("evidence is missing sourceId %q: %s", id, encoded)
		}
	}
	if got[newsSourceID] != "fact" || got[bankSourceID] != "bank_view" {
		t.Errorf("source origin kinds were not preserved: %#v", got)
	}
}

func waitForHealth(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("ESB health endpoint did not become ready")
}

func assertRequest(t *testing.T, client *http.Client, baseURL, method, path string, body any, authorized bool, status int, errorCode string) response {
	t.Helper()
	result := performRequest(t, client, baseURL, method, path, body, authorized)
	if result.status != status {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, result.status, status, result.raw)
	}
	if errorCode != "" && stringField(t, result.body, "error") != errorCode {
		t.Fatalf("%s %s returned error %q, want %q: %s", method, path, result.body["error"], errorCode, result.raw)
	}
	return result
}

func performRequest(t *testing.T, client *http.Client, baseURL, method, path string, body any, authorized bool) response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatalf("create %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorized {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s response: %v", method, path, err)
	}
	result := response{status: resp.StatusCode, raw: data, body: map[string]any{}}
	_ = json.Unmarshal(data, &result.body)
	return result
}

func waitForKonstantinReady(t *testing.T, client *http.Client, baseURL, companyID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		result := performRequest(t, client, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{"companyId": companyID}, true)
		if result.status == http.StatusOK && result.body["status"] == "ready" {
			return result.body
		}
		if result.status != http.StatusAccepted {
			t.Fatalf("GET /call?agent=konstantin returned %d while waiting for ready, want 202: %s", result.status, result.raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("Konstantin profile analysis did not become ready")
	return nil
}

func waitForCall(t *testing.T, client *http.Client, baseURL, agent, callID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp := assertRequest(t, client, baseURL, http.MethodGet, "/call?agent="+url.QueryEscape(agent), map[string]any{"call_id": callID}, true, http.StatusOK, "")
		status := stringField(t, resp.body, "status")
		if status == "completed" {
			return resp.body
		}
		if status == "failed" || status == "cancelled" || status == "interrupted" {
			t.Fatalf("agent %s call %s ended with status %s: %#v", agent, callID, status, resp.body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("agent %s call %s did not complete", agent, callID)
	return nil
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result := callToolRaw(t, ctx, session, name, args)
	if result.IsError {
		t.Fatalf("MCP tool %s returned an error: %s", name, result.Content)
	}
	return structuredMap(t, result)
}

func callToolRaw(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call MCP tool %s: %v", name, err)
	}
	return result
}

func structuredMap(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal MCP structured result: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil || value == nil {
		t.Fatalf("MCP tool did not return structured JSON: %s (%v)", encoded, err)
	}
	return value
}

func stringField(t *testing.T, value map[string]any, name string) string {
	t.Helper()
	result, ok := value[name].(string)
	if !ok {
		t.Fatalf("field %q is not a string in %#v", name, value)
	}
	return result
}

func boolField(t *testing.T, value map[string]any, name string) bool {
	t.Helper()
	result, ok := value[name].(bool)
	if !ok {
		t.Fatalf("field %q is not a bool in %#v", name, value)
	}
	return result
}

func intField(t *testing.T, value map[string]any, name string) int {
	t.Helper()
	result, ok := value[name].(float64)
	if !ok {
		t.Fatalf("field %q is not a number in %#v", name, value)
	}
	return int(result)
}

func floatField(t *testing.T, value map[string]any, name string) float64 {
	t.Helper()
	result, ok := value[name].(float64)
	if !ok {
		t.Fatalf("field %q is not a number in %#v", name, value)
	}
	return result
}

func arrayField(t *testing.T, value map[string]any, name string) []any {
	t.Helper()
	result, ok := value[name].([]any)
	if !ok {
		t.Fatalf("field %q is not an array in %#v", name, value)
	}
	return result
}

func stringArrayField(t *testing.T, value map[string]any, name string) []string {
	t.Helper()
	items := arrayField(t, value, name)
	result := make([]string, 0, len(items))
	for _, item := range items {
		stringValue, ok := item.(string)
		if !ok {
			t.Fatalf("array field %q contains non-string value %#v", name, item)
		}
		result = append(result, stringValue)
	}
	return result
}

type bearerTransport struct {
	key  string
	base http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.key)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func dialWebSocket(t *testing.T, streamURL string) *websocket.Conn {
	t.Helper()
	wsURL := strings.Replace(streamURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if response != nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("WebSocket dial %s: %v (HTTP %d: %s)", wsURL, err, response.StatusCode, body)
		}
		t.Fatalf("WebSocket dial %s: %v", wsURL, err)
	}
	return conn
}

func writeFrame(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal WebSocket frame: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write WebSocket frame: %v", err)
	}
}

func readFrame(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read WebSocket frame: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("WebSocket message type = %v, want text", messageType)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode WebSocket frame %s: %v", data, err)
	}
	return value
}

func readUntilRequest(t *testing.T, conn *websocket.Conn, requestID, kind string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		frame := readFrame(t, conn)
		if stringField(t, frame, "requestId") == requestID && stringField(t, frame, "kind") == kind {
			return frame
		}
	}
	t.Fatalf("did not receive WebSocket kind %q for request %s", kind, requestID)
	return nil
}

func readUntilAnalysis(t *testing.T, conn *websocket.Conn, analysisID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		frame := readFrame(t, conn)
		if stringField(t, frame, "analysisId") == analysisID {
			return frame
		}
	}
	t.Fatalf("did not receive analysis %s", analysisID)
	return nil
}
