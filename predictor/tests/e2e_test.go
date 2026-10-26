package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	apiKey                 = "test-esb-api-key"
	serezhaMCPKey          = "test-only-serezha-mcp-token-000000000001"
	saveliyMCPKey          = "test-only-saveliy-mcp-token-000000000002"
	valeraMCPKey           = "test-only-valera-mcp-token-000000000003"
	keshaMCPKey            = "test-only-kesha-mcp-token-000000000004"
	konstantinMCPKey       = "test-only-konstantin-mcp-token-000000000005"
	companyID              = "11111111-1111-4111-8111-111111111111"
	companyIDTwo           = "19717000-0000-4000-8000-000000000001"
	failedProfileCompanyID = "19717000-0000-4000-8000-000000000002"
	concurrentID           = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	newsSourceID           = "22222222-2222-4222-8222-222222222222"
	bankSourceID           = "33333333-3333-4333-8333-333333333333"
	newsID                 = "55555555-5555-4555-8555-555555555555"
	bankNewsID             = "66666666-6666-4666-8666-666666666666"
	partialNewsID          = "abababab-abab-4bab-8bab-abababababab"
	partialSourceID        = "acacacac-acac-4cac-8cac-acacacacacac"
	foreignSourceID        = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	unavailableSourceID    = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeef"
)

var agentMCPKeys = map[string]string{
	"serezha": serezhaMCPKey, "saveliy": saveliyMCPKey, "valera": valeraMCPKey,
	"kesha": keshaMCPKey, "konstantin": konstantinMCPKey,
}

var mcpContextSequence atomic.Uint64

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
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=serezha", map[string]any{
		"news_id": "env-limit-news", "source_id": newsSourceID, "news_text": strings.Repeat("x", 1025),
	}, true, http.StatusBadRequest, "invalid_news_request")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", nil, true, http.StatusBadRequest, "")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/stream?agent=konstantin", nil, true, http.StatusBadRequest, "stream_does_not_accept_agent")

	// MCP roles are authenticated independently from Domain HTTP and WebSocket.
	mcpCtx, cancelMCP := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancelMCP()
	assertMCPUnauthorized(t, httpClient, baseURL, "")
	assertMCPUnauthorized(t, httpClient, baseURL, "invalid-e2e-mcp-token")
	assertMCPUnauthorized(t, httpClient, baseURL, apiKey)
	assertMCPCallUnauthorized(t, httpClient, baseURL, "")
	assertMCPCallUnauthorized(t, httpClient, baseURL, "invalid-e2e-mcp-token")
	assertMCPCallUnauthorized(t, httpClient, baseURL, apiKey)
	mcpSession := connectAgentMCPs(t, mcpCtx, baseURL)
	defer mcpSession.close(t)
	wantTools := map[string][]string{
		"serezha":    {"save_news_and_facts"},
		"saveliy":    {"get_company_profile", "get_facts", "save_fact_company_links"},
		"valera":     {"get_company_metrics", "save_company_profile_and_metrics"},
		"kesha":      {"get_company_profile", "get_company_facts", "get_company_metrics", "save_metric_observations", "calculate_all_metric_totals", "save_company_analysis", "get_company_report"},
		"konstantin": {"get_company_profile", "get_company_metrics", "get_consultant_context"},
	}
	for agent, expected := range wantTools {
		assertAgentTools(t, mcpCtx, mcpSession.sessions[agent], agent, expected)
	}
	assertAgentToolsThroughSpoofedQuery(t, mcpCtx, baseURL, "saveliy", "/mcp?agent=valera", wantTools["saveliy"])
	assertMCPRoleCannotReuseAnotherAgentsSession(t, httpClient, baseURL,
		mcpSession.sessions["saveliy"].ID(), valeraMCPKey)
	assertForbiddenMCPCallWithoutList(t, mcpCtx, baseURL, saveliyMCPKey,
		"save_company_profile_and_metrics", map[string]any{"agent": "valera", "company_id": companyID})
	assertForbiddenMCPCallWithoutList(t, mcpCtx, baseURL, keshaMCPKey,
		"save_company_metrics", map[string]any{"agent": "valera", "company_id": companyID})
	if got := clickHouseCount(t, "SELECT count() FROM system.tables WHERE database = 'default' AND name IN ('company_metric_settings', 'company_metrics')"); got != 0 {
		t.Fatalf("ClickHouse contains %d metric catalog/settings tables; those belong only in PostgreSQL", got)
	}

	// Identical concurrent submissions must resolve to one PostgreSQL call.
	const concurrentRequestCount = 8
	concurrentBody, err := json.Marshal(map[string]any{"company_id": concurrentID, "request_sequence": 1})
	if err != nil {
		t.Fatalf("encode concurrent call body: %v", err)
	}
	type concurrentResult struct {
		response response
		err      error
	}
	startConcurrentRequests := make(chan struct{})
	concurrentResults := make(chan concurrentResult, concurrentRequestCount)
	for range concurrentRequestCount {
		go func() {
			<-startConcurrentRequests
			request, requestErr := http.NewRequest(http.MethodPost, baseURL+"/call?agent=kesha", bytes.NewReader(concurrentBody))
			if requestErr != nil {
				concurrentResults <- concurrentResult{err: requestErr}
				return
			}
			request.Header.Set("Authorization", "Bearer "+apiKey)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "e2e-concurrent-idempotency")
			httpResponse, requestErr := httpClient.Do(request)
			if requestErr != nil {
				concurrentResults <- concurrentResult{err: requestErr}
				return
			}
			defer httpResponse.Body.Close()
			body, readErr := io.ReadAll(httpResponse.Body)
			if readErr != nil {
				concurrentResults <- concurrentResult{err: readErr}
				return
			}
			result := response{status: httpResponse.StatusCode, raw: body, body: map[string]any{}}
			if decodeErr := json.Unmarshal(body, &result.body); decodeErr != nil {
				concurrentResults <- concurrentResult{err: decodeErr}
				return
			}
			concurrentResults <- concurrentResult{response: result}
		}()
	}
	close(startConcurrentRequests)
	var concurrentCallID string
	reusedConcurrentCalls := 0
	for range concurrentRequestCount {
		result := <-concurrentResults
		if result.err != nil {
			t.Fatalf("concurrent idempotent request failed: %v", result.err)
		}
		if result.response.status != http.StatusAccepted {
			t.Fatalf("concurrent idempotent request returned %d, want %d: %s", result.response.status, http.StatusAccepted, result.response.raw)
		}
		callID := stringField(t, result.response.body, "call_id")
		if concurrentCallID == "" {
			concurrentCallID = callID
		} else if callID != concurrentCallID {
			t.Fatalf("same concurrent idempotency key created calls %q and %q", concurrentCallID, callID)
		}
		if boolField(t, result.response.body, "reused") {
			reusedConcurrentCalls++
		}
	}
	if reusedConcurrentCalls != concurrentRequestCount-1 {
		t.Fatalf("concurrent retries reused %d times, want %d", reusedConcurrentCalls, concurrentRequestCount-1)
	}
	waitForCall(t, httpClient, baseURL, "kesha", concurrentCallID)

	// Valera profile call: enqueue, read its status, atomically persist profile+metrics over MCP.
	profilePayload := valeraPayloadForCompany(failedProfileCompanyID, 1)
	valera := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", profilePayload, true, http.StatusAccepted, "")
	valeraCallID := stringField(t, valera.body, "call_id")
	valeraFailure := waitForFailedCall(t, httpClient, baseURL, "valera", valeraCallID)
	if stringField(t, valeraFailure.body, "error") != "company_profile_not_synced" {
		t.Fatalf("Valera completed without saving its profile but returned a different error: %#v", valeraFailure.body)
	}
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin", map[string]any{"companyId": failedProfileCompanyID}, true, http.StatusConflict, "profile_update_pending")
	valeraProfileCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "valera", companyID, valeraPayload(1))
	metric := metricSetting()
	profileMarkdown := "# Example company\n\nA manufacturer with a growing domestic market.\n"
	vectorMetrics := metricSettingsForVector()
	savedProfile := callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1, "markdown": profileMarkdown,
		"metrics": vectorMetrics,
	})
	if got := stringField(t, savedProfile, "profile_key"); got != "companies/"+companyID+"/profile.md" {
		t.Fatalf("unexpected S3 profile key %q", got)
	}
	if got := intField(t, savedProfile, "metrics_saved"); got != 8 {
		t.Fatalf("expected eight saved metrics, got %d", got)
	}
	// Exact retries are safe; a changed payload at the same revision is rejected.
	callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1, "markdown": profileMarkdown,
		"metrics": vectorMetrics,
	})
	changedProfile := callToolRaw(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 1, "markdown": "# Changed at same revision\n",
		"metrics": []any{metric},
	})
	if !changedProfile.IsError {
		t.Fatal("MCP accepted a conflicting profile payload at the same revision")
	}
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, valeraProfileCall)
	keshaCompanyCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "kesha", companyID, map[string]any{"company_id": companyID})
	profile := callTool(t, mcpCtx, mcpSession, "get_company_profile", map[string]any{"company_id": companyID})
	if !strings.Contains(stringField(t, profile, "markdown"), "Example company") {
		t.Fatal("S3-compatible profile read did not return the saved Markdown")
	}
	waitForKonstantinReady(t, httpClient, baseURL, companyID)
	metrics := callTool(t, mcpCtx, mcpSession, "get_company_metrics", map[string]any{"company_id": companyID, "limit": 100})
	metricRows := arrayField(t, metrics, "metrics")
	if len(metricRows) != 8 {
		t.Fatalf("PostgreSQL metric settings were not returned through MCP: got %d rows, want 8", len(metricRows))
	}
	metricIDs := make(map[string]string, len(metricRows))
	for _, rawMetric := range metricRows {
		row := rawMetric.(map[string]any)
		metricIDs[stringField(t, row, "type_code")] = stringField(t, row, "metric_id")
		if stringField(t, row, "type_code") == "цена" && stringField(t, row, "unit") != "₽/шт." {
			t.Fatalf("metric measurement unit was not retained by PostgreSQL/MCP: %#v", row)
		}
	}
	profileRetry := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", profilePayload, true, http.StatusAccepted, "")
	if !boolField(t, profileRetry.body, "reused") {
		t.Fatal("same Valera profile revision was not idempotent")
	}
	changedRevision := valeraPayload(1)
	changedRevision["company"] = map[string]any{"city": "different payload"}
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", changedRevision, true, http.StatusConflict, "profile_revision_conflict")

	// Seed the result through ESB MCP, then verify that Hermes completion alone
	// is insufficient and that confirmed processing retains source text until
	// its independent post-processing retention period expires.
	newsFacts := saveNewsFacts(t, mcpCtx, mcpSession, companyID, newsID, newsSourceID, "external_news", "text/html", "Market change", "https://example.test/news/market-change", "Cleaned external news text.")
	bankFacts := saveNewsFacts(t, mcpCtx, mcpSession, companyID, bankNewsID, bankSourceID, "alpha_analysis", "application/pdf", "Alpha market analysis", "https://example.test/alpha/market-analysis.pdf", "Cleaned Alpha analysis text.")
	newsCall := postSerezha(t, httpClient, baseURL, newsSourceID, newsID, "external_news", "text/html", "Market change", "https://example.test/news/market-change", "Cleaned external news text.")
	waitForCall(t, httpClient, baseURL, "serezha", stringField(t, newsCall.body, "call_id"))
	bankCall := postSerezha(t, httpClient, baseURL, bankSourceID, bankNewsID, "alpha_analysis", "application/pdf", "Alpha market analysis", "https://example.test/alpha/market-analysis.pdf", "Cleaned Alpha analysis text.")
	waitForCall(t, httpClient, baseURL, "serezha", stringField(t, bankCall.body, "call_id"))
	if got := clickHouseCount(t, "SELECT count() FROM news_content FINAL WHERE news_id = '"+newsID+"'"); got != 1 {
		t.Fatalf("confirmed news raw text rows = %d, want 1 before retention expires", got)
	}
	duplicateNews := postSerezha(t, httpClient, baseURL, newsSourceID, newsID, "external_news", "text/html", "Market change", "https://example.test/news/market-change", "Cleaned external news text.")
	if !boolField(t, duplicateNews.body, "reused") || stringField(t, duplicateNews.body, "call_id") != stringField(t, newsCall.body, "call_id") {
		t.Fatal("repeated sourceId created a duplicate call")
	}
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/news", map[string]any{}, true, http.StatusNotFound, "")
	largeNews := serezhaPayload(newsSourceID, newsID, "external_news", "text/html", "Market change", "https://example.test/news/market-change", strings.Repeat("x", (1<<20)+1))
	// The E2E Compose override caps the entire JSON body at 8 KiB, so this
	// oversized payload is rejected by the HTTP body limit before news validation.
	assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=serezha", largeNews, true, http.StatusBadRequest, "invalid_json_body")

	// Exercise all ClickHouse-facing MCP tools while source-text retention is active.
	globalNewsID := "88888888-8888-4888-8888-888888888888"
	globalNews := callTool(t, mcpCtx, mcpSession, "save_news_and_facts", map[string]any{
		"news_id": globalNewsID, "source_id": "99999999-9999-4999-8999-999999999999",
		"source_url": "https://example.test/news/global-market", "material_kind": "external_news",
		"media_type": "text/html", "title": "Global market trend", "news_text": "A global trend affects manufacturing.",
		"facts": []any{map[string]any{"fact_text": "Manufacturing output is increasing globally.", "category": "market",
			"entities": []string{"manufacturing"}, "evidence": "manufacturing", "occurred_at": "2026-10-25T09:00:00Z", "confidence": "0.9"}},
	})
	if len(stringArrayField(t, globalNews, "fact_ids")) != 1 {
		t.Fatal("saving company-independent news did not create its universal fact")
	}
	factIDs := append(newsFacts, bankFacts...)
	links := make([]any, 0, len(factIDs))
	for _, factID := range factIDs {
		links = append(links, map[string]any{"fact_id": factID, "relevance": "high", "confidence": "0.95"})
	}
	saveliyCompanyCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "saveliy", companyID,
		map[string]any{"company_id": companyID, "news_id": newsID})
	callTool(t, mcpCtx, mcpSession, "save_fact_company_links", map[string]any{"company_id": companyID, "links": links})
	facts := callTool(t, mcpCtx, mcpSession, "get_facts", map[string]any{"news_id": newsID, "limit": 100})
	newsFactRows := arrayField(t, facts, "facts")
	if len(newsFactRows) != 1 {
		t.Fatalf("expected one fact for news source, got %d", len(arrayField(t, facts, "facts")))
	}
	newsFact, ok := newsFactRows[0].(map[string]any)
	if !ok || stringField(t, newsFact, "fact_id") == "" || stringField(t, newsFact, "source_id") != newsSourceID {
		t.Fatalf("get_facts lost a fact or source ID after joining news metadata: %#v", newsFactRows[0])
	}
	bySource := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"sourceId": newsSourceID, "materialKind": "external_news", "category": "market", "entity": "manufacturing", "limit": 10},
		true, http.StatusOK, "")
	if len(arrayField(t, bySource.body, "facts")) != 1 || stringField(t, bySource.body, "data") != "facts" {
		t.Fatalf("GET /call?data=facts did not return facts for the source: %#v", bySource.body)
	}
	byNews := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"news_id": newsID, "limit": 10}, true, http.StatusOK, "")
	if len(arrayField(t, byNews.body, "facts")) != 1 {
		t.Fatalf("news ID filter did not return its fact: %#v", byNews.body)
	}
	companyFactPage := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"companyId": companyID, "limit": 1}, true, http.StatusOK, "")
	if len(arrayField(t, companyFactPage.body, "facts")) != 1 || !boolField(t, companyFactPage.body, "hasMore") {
		t.Fatalf("company fact query did not return a bounded first page: %#v", companyFactPage.body)
	}
	companyFactNext := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"companyId": companyID, "limit": 1, "beforeFactId": stringField(t, companyFactPage.body, "nextBeforeFactId")},
		true, http.StatusOK, "")
	if len(arrayField(t, companyFactNext.body, "facts")) != 1 || boolField(t, companyFactNext.body, "hasMore") {
		t.Fatalf("company fact cursor did not return the next page: %#v", companyFactNext.body)
	}
	globalFacts := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"periodFrom": time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano),
			"periodTo": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano), "category": "market", "limit": 10},
		true, http.StatusOK, "")
	if len(arrayField(t, globalFacts.body, "facts")) != 3 {
		t.Fatalf("global anonymized facts query returned wrong count: %#v", globalFacts.body)
	}
	globalHasUnlinkedFact := false
	for _, rawFact := range arrayField(t, globalFacts.body, "facts") {
		fact := rawFact.(map[string]any)
		if _, exists := fact["companyId"]; exists {
			t.Fatal("global facts response leaked a company identifier")
		}
		if stringField(t, fact, "news_id") == globalNewsID {
			globalHasUnlinkedFact = true
		}
	}
	if !globalHasUnlinkedFact {
		t.Fatal("global facts query omitted the company-independent news fact")
	}
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts", map[string]any{}, true, http.StatusBadRequest, "invalid_facts_query")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"companyId": companyID, "periodFrom": time.Now().UTC().Add(-91 * 24 * time.Hour).Format(time.RFC3339Nano),
			"periodTo": time.Now().UTC().Format(time.RFC3339Nano)}, true, http.StatusBadRequest, "invalid_facts_query")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"companyId": companyID, "limit": 101}, true, http.StatusBadRequest, "invalid_facts_query")
	assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?data=facts",
		map[string]any{"companyId": companyID, "beforeFactId": "not-a-uuid"}, true, http.StatusBadRequest, "invalid_facts_query")
	companyFacts := callTool(t, mcpCtx, mcpSession, "get_company_facts", map[string]any{"company_id": companyID, "limit": 100})
	companyFactRows := arrayField(t, companyFacts, "facts")
	if len(companyFactRows) != 2 {
		t.Fatalf("expected two linked company facts, got %d", len(arrayField(t, companyFacts, "facts")))
	}
	validSources := map[string]bool{newsSourceID: true, bankSourceID: true}
	for index := 1; index <= 10; index++ {
		validSources[testSourceID(index)] = true
	}
	validSources[testSourceID(11)] = true
	validSources[testSourceID(100)] = true
	for _, rawFact := range companyFactRows {
		fact, ok := rawFact.(map[string]any)
		if !ok || stringField(t, fact, "fact_id") == "" || !validSources[stringField(t, fact, "source_id")] {
			t.Fatalf("get_company_facts lost a fact or source ID after joining links: %#v", rawFact)
		}
	}
	// Reproduce the source document's September vector. News was recorded through
	// ESB after the fact range but the aggregate must use news.published_at.
	vectorNews := []struct {
		id, at, url, kind string
	}{
		{testNewsID(1), "2026-09-11T12:00:00Z", "https://example.test/news/N1", "alpha_analysis"},
		{testNewsID(2), "2026-09-14T09:00:00Z", "", "alpha_analysis"},
		{testNewsID(3), "2026-09-30T23:59:59Z", "https://example.test/news/N3", "external_news"},
		{testNewsID(4), "2026-08-31T23:59:59Z", "https://example.test/news/N0", "external_news"},
		{testNewsID(5), "2026-10-01T00:00:00Z", "https://example.test/news/N6", "external_news"},
		{testNewsID(6), "2026-09-15T10:00:00Z", "https://example.test/news/N7", "external_news"},
		{testNewsID(7), "2026-09-01T00:00:00Z", "https://example.test/news/N4", "external_news"},
		{testNewsID(8), "2026-09-05T11:00:00Z", "https://example.test/news/L1", "external_news"},
		{testNewsID(9), "2026-09-06T11:00:00Z", "https://example.test/news/L2", "external_news"},
		{testNewsID(10), "2026-09-07T11:00:00Z", "https://example.test/news/L3", "external_news"},
		{testNewsID(12), "2026-10-02T00:00:00Z", "https://example.test/news/N12", "external_news"},
	}
	for i, item := range vectorNews {
		saveNewsAt(t, httpClient, baseURL, mcpCtx, mcpSession, companyID, item.id, testSourceID(i+1), item.at, item.url, item.kind)
	}
	newsWithoutTimeID := testNewsID(11)
	saveNewsWithoutTime(t, httpClient, baseURL, mcpCtx, mcpSession, companyID, newsWithoutTimeID)
	observations := []any{
		metricObservation(metricIDs["ставка"], testNewsID(1), 2.0, 0.8),
		metricObservation(metricIDs["ставка"], testNewsID(2), 1.0, 0.6),
		metricObservation(metricIDs["ставка"], testNewsID(3), 0.0, 0.4),
		metricObservation(metricIDs["ставка"], testNewsID(4), 9.0, 0.9),
		metricObservation(metricIDs["ставка"], testNewsID(5), 100.0, 0.9),
		map[string]any{"metric_id": metricIDs["ставка"], "news_id": testNewsID(6), "value": nil, "probability": 0.99, "source_fact_ids": []string{}},
		metricObservation(metricIDs["ставка"], newsWithoutTimeID, 1000.0, 1.0),
		metricObservation(metricIDs["ставка"], testNewsID(12), 10000.0, 1.0),
		metricObservation(metricIDs["курс"], testNewsID(7), 3.0, 0.5),
		metricObservation(metricIDs["логистика"], testNewsID(8), 4.0, 0.8),
		metricObservation(metricIDs["логистика"], testNewsID(9), 2.0, nil),
		map[string]any{"metric_id": metricIDs["логистика"], "news_id": testNewsID(10), "value": nil, "probability": 0.99, "source_fact_ids": []string{}},
	}
	callTool(t, mcpCtx, mcpSession, "save_metric_observations", map[string]any{"company_id": companyID, "observations": observations})
	callTool(t, mcpCtx, mcpSession, "save_metric_observations", map[string]any{"company_id": companyID, "observations": observations})
	periodFrom, periodTo := "2026-09-01 00:00:00", "2026-09-30 23:59:59"
	total := calculateOneMetric(t, mcpCtx, mcpSession, companyID, metricIDs["ставка"], periodFrom, periodTo)
	if stringField(t, total, "metric_id") != metricIDs["ставка"] {
		t.Fatalf("metric total lost its PostgreSQL ownership ID: %#v", total)
	}
	if got := floatField(t, total, "total_value"); math.Abs(got-1) > 0.000001 {
		t.Fatalf("unweighted value mean = %v, want 1", got)
	}
	if got := floatField(t, total, "total_probability"); math.Abs(got-0.6) > 0.000001 {
		t.Fatalf("probability mean = %v, want 0.6", got)
	}
	if got := intField(t, total, "fact_count"); got != 3 {
		t.Fatalf("fact_count = %d, want 3", got)
	}
	if got := intField(t, total, "probability_count"); got != 3 {
		t.Fatalf("probability_count = %d, want 3", got)
	}
	dateOnlyTotal := calculateOneMetric(t, mcpCtx, mcpSession, companyID, metricIDs["ставка"], "2026-09-01", "2026-09-30")
	if floatField(t, dateOnlyTotal, "total_value") != 1 {
		t.Fatalf("date-only inclusive period produced total %v, want 1", floatField(t, dateOnlyTotal, "total_value"))
	}
	metricLinks := arrayField(t, total, "links")
	if len(metricLinks) != 3 {
		t.Fatalf("links = %d, want 3 (including zero value, excluding null value and out-of-range news)", len(metricLinks))
	}
	if stringField(t, metricLinks[0].(map[string]any), "source_kind") != "bank" || stringField(t, metricLinks[1].(map[string]any), "news_url") != "" || stringField(t, metricLinks[2].(map[string]any), "source_kind") != "external" {
		t.Fatalf("news metadata or sorted links are incorrect: %#v", metricLinks)
	}
	logistics := calculateOneMetric(t, mcpCtx, mcpSession, companyID, metricIDs["логистика"], periodFrom, periodTo)
	if floatField(t, logistics, "total_value") != 3 || floatField(t, logistics, "total_probability") != 0.8 || intField(t, logistics, "fact_count") != 2 || intField(t, logistics, "probability_count") != 1 {
		t.Fatalf("nullable probability handling differs from source example: %#v", logistics)
	}
	totals := callTool(t, mcpCtx, mcpSession, "calculate_all_metric_totals", map[string]any{"company_id": companyID, "period_from": periodFrom, "period_to": periodTo})
	totalItems := arrayField(t, totals, "items")
	if len(totalItems) != 8 {
		t.Fatalf("calculate_all_metric_totals returned %d rows, want all 8 configured metrics", len(totalItems))
	}
	if got := clickHouseCount(t, "SELECT count() FROM metric_totals FINAL WHERE company_id = '"+companyID+"' AND period_from = toDateTime64('2026-09-01 00:00:00', 3, 'UTC') AND period_to = toDateTime64('2026-09-30 23:59:59', 3, 'UTC')"); got != 8 {
		t.Fatalf("ClickHouse has %d stored metric totals for the company period, want 8", got)
	}
	wantOrder := []string{"ставка", "курс", "логистика", "цена", "спрос", "запас", "контрагент", "зарплата"}
	for i, raw := range totalItems {
		item := raw.(map[string]any)
		if stringField(t, item, "type_code") != wantOrder[i] {
			t.Fatalf("metric row %d type_code = %q, want %q", i, stringField(t, item, "type_code"), wantOrder[i])
		}
	}
	if _, ok := totalItems[3].(map[string]any)["total_value"]; !ok || totalItems[3].(map[string]any)["total_value"] != nil || intField(t, totalItems[3].(map[string]any), "fact_count") != 0 {
		t.Fatalf("empty metric must remain present with null total and zero count: %#v", totalItems[3])
	}
	reversed := callTool(t, mcpCtx, mcpSession, "calculate_all_metric_totals", map[string]any{"company_id": companyID, "period_from": periodTo, "period_to": periodFrom})
	if len(arrayField(t, reversed, "items")) != 8 || arrayField(t, reversed, "items")[0].(map[string]any)["total_value"] != nil {
		t.Fatalf("reversed period should retain configured rows with empty totals: %#v", reversed)
	}
	emptyMetricsCompanyID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	emptyMetricsCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "kesha", emptyMetricsCompanyID,
		map[string]any{"company_id": emptyMetricsCompanyID})
	noSettings := callTool(t, mcpCtx, mcpSession, "calculate_all_metric_totals", map[string]any{"company_id": emptyMetricsCompanyID, "period_from": periodFrom, "period_to": periodTo})
	if len(arrayField(t, noSettings, "items")) != 0 {
		t.Fatalf("company without metric rows should return an empty list: %#v", noSettings)
	}
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, emptyMetricsCall)
	secondCompanyMetrics := metricSettingsForCatalog()
	valeraOtherCompanyCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "valera", companyIDTwo,
		valeraPayloadForCompany(companyIDTwo, 1))
	callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyIDTwo, "profile_revision": 1, "markdown": "# Second synthetic company\n",
		"metrics": secondCompanyMetrics,
	})
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, valeraOtherCompanyCall)
	keshaOtherCompanyCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "kesha", companyIDTwo,
		map[string]any{"company_id": companyIDTwo})
	secondCompanyRows := callTool(t, mcpCtx, mcpSession, "get_company_metrics", map[string]any{"company_id": companyIDTwo, "limit": 100})
	assertMCPCompanyContextIsolation(t, mcpCtx, mcpSession.sessions["kesha"], companyID, companyIDTwo,
		mcpSession.companyContext(t, "kesha", companyID), mcpSession.companyContext(t, "kesha", companyIDTwo))
	secondMetricIDs := make(map[string]string)
	for _, raw := range arrayField(t, secondCompanyRows, "metrics") {
		row := raw.(map[string]any)
		secondMetricIDs[stringField(t, row, "type_code")] = stringField(t, row, "metric_id")
	}
	sharedNewsFacts := callTool(t, mcpCtx, mcpSession, "get_facts", map[string]any{"news_id": testNewsID(1), "limit": 10})
	sharedFactRows := arrayField(t, sharedNewsFacts, "facts")
	if len(sharedFactRows) == 0 {
		t.Fatal("shared company news has no persisted fact to link")
	}
	saveliyOtherCompanyCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "saveliy", companyIDTwo,
		map[string]any{"company_id": companyIDTwo, "news_id": testNewsID(1)})
	callTool(t, mcpCtx, mcpSession, "save_fact_company_links", map[string]any{
		"company_id": companyIDTwo,
		"links":      []any{map[string]any{"fact_id": stringField(t, sharedFactRows[0].(map[string]any), "fact_id"), "relevance": "shared market news", "confidence": 0.9}},
	})
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, saveliyOtherCompanyCall)
	secondObservations := []any{
		metricObservation(secondMetricIDs["ставка"], testNewsID(1), 2.0, 0.7),
		metricObservation(secondMetricIDs["депозит"], testNewsID(1), 5.0, 0.7),
		metricObservation(secondMetricIDs["будущее"], testNewsID(1), 11.0, nil),
	}
	callTool(t, mcpCtx, mcpSession, "save_metric_observations", map[string]any{"company_id": companyIDTwo, "observations": secondObservations})
	crossCompanyObservation := callToolRaw(t, mcpCtx, mcpSession, "save_metric_observations", map[string]any{
		"company_id":   companyIDTwo,
		"observations": []any{metricObservation(secondMetricIDs["ставка"], testNewsID(2), 99.0, 1.0)},
	})
	if !crossCompanyObservation.IsError {
		t.Fatal("ESB accepted an observation that references news linked only to another company")
	}
	secondTotals := callTool(t, mcpCtx, mcpSession, "calculate_all_metric_totals", map[string]any{"company_id": companyIDTwo, "period_from": periodFrom, "period_to": periodTo})
	secondItems := arrayField(t, secondTotals, "items")
	fullOrder := []string{"ставка", "депозит", "курс", "логистика", "цена", "спрос", "запас", "контрагент", "зарплата", "будущее"}
	if len(secondItems) != len(fullOrder) {
		t.Fatalf("all-catalog company returned %d items, want %d", len(secondItems), len(fullOrder))
	}
	for i, raw := range secondItems {
		item := raw.(map[string]any)
		if stringField(t, item, "type_code") != fullOrder[i] {
			t.Fatalf("full metric catalog order at %d = %q, want %q", i, stringField(t, item, "type_code"), fullOrder[i])
		}
	}
	if floatField(t, secondItems[1].(map[string]any), "total_value") != 5 {
		t.Fatalf("positive deposit value was changed: %#v", secondItems[1])
	}
	if floatField(t, secondItems[9].(map[string]any), "total_value") != 11 || secondItems[9].(map[string]any)["total_probability"] != nil || intField(t, secondItems[9].(map[string]any), "probability_count") != 0 {
		t.Fatalf("known values with no known probability must still average, but return null probability: %#v", secondItems[9])
	}
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, keshaOtherCompanyCall)
	// A second prediction on one news row contributes to the arithmetic mean,
	// while its news appears only once in the links array.
	callTool(t, mcpCtx, mcpSession, "save_metric_observations", map[string]any{"company_id": companyID, "observations": []any{
		map[string]any{"observation_id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "metric_id": metricIDs["ставка"], "news_id": testNewsID(1), "value": 4.0, "probability": 0.6, "source_fact_ids": []string{}},
	}})
	duplicateNewsTotal := calculateOneMetric(t, mcpCtx, mcpSession, companyID, metricIDs["ставка"], periodFrom, periodTo)
	if math.Abs(floatField(t, duplicateNewsTotal, "total_value")-1.75) > 0.000001 || len(arrayField(t, duplicateNewsTotal, "links")) != 3 {
		t.Fatalf("multiple predictions for one news must contribute separately but link once: %#v", duplicateNewsTotal)
	}
	callTool(t, mcpCtx, mcpSession, "save_company_analysis", map[string]any{
		"company_id": companyID, "period_from": "2026-09-01", "period_to": "2026-09-30",
		"probability": "0.9", "metric_ids": []string{metricIDs["ставка"]}, "text": "The evidence suggests a moderate rate change.",
	})
	companyReport := callTool(t, mcpCtx, mcpSession, "get_company_report", map[string]any{"company_id": companyID, "period_from": "2026-09-01", "period_to": "2026-09-30"})
	if len(arrayField(t, companyReport, "facts")) != 14 || len(arrayField(t, companyReport, "metrics")) != 8 || len(arrayField(t, companyReport, "totals")) != 8 || len(arrayField(t, companyReport, "analyses")) != 1 {
		t.Fatalf("company report is missing ClickHouse or PostgreSQL data: %#v", companyReport)
	}
	var priceTotal map[string]any
	for _, rawTotal := range arrayField(t, companyReport, "totals") {
		item, ok := rawTotal.(map[string]any)
		if ok && stringField(t, item, "type_code") == "цена" {
			priceTotal = item
			break
		}
	}
	if priceTotal == nil || stringField(t, priceTotal, "unit") != "₽/шт." {
		t.Fatalf("metric total did not return its PostgreSQL unit: %#v", priceTotal)
	}
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, keshaCompanyCall)
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, saveliyCompanyCall)
	foreignSource := postSerezhaForCompany(t, httpClient, baseURL, companyIDTwo, foreignSourceID,
		"foreign-company-source", "external_news", "text/html", "Other company source",
		"https://example.test/news/foreign-company", "Synthetic source owned by another company.")
	foreignFailure := waitForFailedCall(t, httpClient, baseURL, "serezha", stringField(t, foreignSource.body, "call_id"))
	if stringField(t, foreignFailure.body, "error") != "news_persistence_unconfirmed" {
		t.Fatalf("unpersisted foreign source call failed for an unexpected reason: %#v", foreignFailure.body)
	}
	unavailableSource := postSerezhaForCompany(t, httpClient, baseURL, companyID, unavailableSourceID,
		"unavailable-company-source", "external_news", "text/html", "Unavailable source",
		"https://example.test/news/unavailable", "Synthetic source not present in consultant context.")
	unavailableFailure := waitForFailedCall(t, httpClient, baseURL, "serezha", stringField(t, unavailableSource.body, "call_id"))
	if stringField(t, unavailableFailure.body, "error") != "news_persistence_unconfirmed" {
		t.Fatalf("unpersisted company source call failed for an unexpected reason: %#v", unavailableFailure.body)
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
	assertMockConsultantCall(t, httpClient, "http://127.0.0.1:18090", analysisID, companyID, 14, 13)

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
	answer := sendQuestion(t, ws, question)
	answerMessageID := stringField(t, answer, "messageId")
	assertEvidence(t, answer["evidence"], newsSourceID, bankSourceID)

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
	queueMockNextOutput(t, httpClient, "http://127.0.0.1:18090", companyID, "question", map[string]any{
		"text": "Unconfirmed empty-evidence response", "evidence": []any{},
	})
	emptyEvidence := sendQuestion(t, ws, map[string]any{"kind": "question", "requestId": "77777777-7777-4777-8777-777777777778",
		"companyId": companyID, "conversationId": conversationID, "analysisId": analysisID, "text": "Give a recommendation without sources."})
	assertInsufficientEvidence(t, emptyEvidence)
	queueMockNextOutput(t, httpClient, "http://127.0.0.1:18090", companyID, "question", map[string]any{
		"text": "Unconfirmed invented-source response", "evidence": []any{map[string]any{"sourceId": "12345678-1234-4123-8123-123456789abc", "originKind": "fact"}},
	})
	inventedEvidence := sendQuestion(t, ws, map[string]any{"kind": "question", "requestId": "77777777-7777-4777-8777-777777777779",
		"companyId": companyID, "conversationId": conversationID, "analysisId": analysisID, "text": "Cite a made-up source."})
	assertInsufficientEvidence(t, inventedEvidence)
	queueMockNextOutput(t, httpClient, "http://127.0.0.1:18090", companyID, "question", map[string]any{
		"text": "Unconfirmed foreign-source response", "evidence": []any{map[string]any{"sourceId": foreignSourceID, "originKind": "fact"}},
	})
	foreignEvidence := sendQuestion(t, ws, map[string]any{"kind": "question", "requestId": "77777777-7777-4777-8777-777777777780",
		"companyId": companyID, "conversationId": conversationID, "analysisId": analysisID, "text": "Cite another company's source."})
	assertInsufficientEvidence(t, foreignEvidence)
	queueMockNextOutput(t, httpClient, "http://127.0.0.1:18090", companyID, "question", map[string]any{
		"text": "Unconfirmed unavailable-source response", "evidence": []any{map[string]any{"sourceId": unavailableSourceID, "originKind": "fact"}},
	})
	unavailableEvidence := sendQuestion(t, ws, map[string]any{"kind": "question", "requestId": "77777777-7777-4777-8777-777777777781",
		"companyId": companyID, "conversationId": conversationID, "analysisId": analysisID, "text": "Cite a source absent from this context."})
	assertInsufficientEvidence(t, unavailableEvidence)

	// Cancellation is a lifecycle operation: the model still produces one full
	// answer per request, and an active Hermes run must be stopped exactly once.
	cancelRequestID := "77777777-7777-4777-8777-777777777782"
	queueMockRunStatus(t, httpClient, "http://127.0.0.1:18090", companyID, "running")
	writeFrame(t, ws, map[string]any{"kind": "question", "requestId": cancelRequestID, "companyId": companyID,
		"conversationId": conversationID, "analysisId": analysisID, "text": "Please cancel this analysis."})
	cancelAccepted := readFrame(t, ws)
	if stringField(t, cancelAccepted, "kind") != "accepted" || stringField(t, cancelAccepted, "requestId") != cancelRequestID {
		t.Fatalf("cancellable question was not accepted: %#v", cancelAccepted)
	}
	cancelCallID := stringField(t, cancelAccepted, "callId")
	cancelRun := waitForMockRequest(t, httpClient, e2eMockURLs(), cancelCallID)
	waitForCallStatus(t, httpClient, baseURL, "konstantin", cancelCallID, "running")
	writeFrame(t, ws, map[string]any{"kind": "cancel", "requestId": cancelRequestID})
	cancelRequested := readUntilRequest(t, ws, cancelRequestID, "cancel_requested")
	cancelSequence := intField(t, cancelRequested, "sequence")
	if cancelSequence <= intField(t, cancelAccepted, "sequence") {
		t.Fatalf("cancel sequence %d did not follow accepted sequence %v", cancelSequence, cancelAccepted["sequence"])
	}
	writeFrame(t, ws, map[string]any{"kind": "cancel", "requestId": cancelRequestID})
	cancelled := readUntilRequest(t, ws, cancelRequestID, "assistant_cancelled")
	terminalSequence := intField(t, cancelled, "sequence")
	if terminalSequence <= cancelSequence || !boolField(t, cancelled, "incomplete") {
		t.Fatalf("cancellation terminal frame did not follow the request: %#v", cancelled)
	}
	if call := waitForCallStatus(t, httpClient, baseURL, "konstantin", cancelCallID, "cancelled"); stringField(t, call, "error") != "assistant_cancelled" {
		t.Fatalf("cancelled call has unexpected stored outcome: %#v", call)
	}
	if stopCount := mockStopCount(t, httpClient, cancelRun.baseURL, cancelRun.runID); stopCount != 1 {
		t.Fatalf("Hermes stop was requested %d times for one cancelled call, want 1", stopCount)
	}
	cancelHistory := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin",
		map[string]any{"companyId": companyID, "historyLimit": 100, "historyFrom": historyFrom, "historyTo": historyTo}, true, http.StatusOK, "")
	var sawCancelledUser, sawCancelledAssistant bool
	for _, raw := range arrayField(t, cancelHistory.body, "history") {
		message, ok := raw.(map[string]any)
		if !ok || stringField(t, message, "requestId") != cancelRequestID {
			continue
		}
		switch stringField(t, message, "role") {
		case "user":
			sawCancelledUser = true
		case "assistant":
			sawCancelledAssistant = true
		}
	}
	if !sawCancelledUser || sawCancelledAssistant {
		t.Fatalf("cancelled turn history should keep the question and omit an assistant answer (user=%t assistant=%t)", sawCancelledUser, sawCancelledAssistant)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "")

	// New source and profile revisions independently invalidate saved analysis.
	partialText := "Partial source text remains until its facts are confirmed."
	seedPartialNewsContent(t, partialNewsID, partialSourceID, "https://example.test/news/partial", "Partial source", partialText)
	newSourceCall := postSerezha(t, httpClient, baseURL, partialSourceID, partialNewsID, "external_news", "text/html", "Partial source", "https://example.test/news/partial", partialText)
	partialCallID := stringField(t, newSourceCall.body, "call_id")
	failedNewsCall := waitForFailedCall(t, httpClient, baseURL, "serezha", partialCallID)
	if stringField(t, failedNewsCall.body, "error") != "news_persistence_unconfirmed" {
		t.Fatalf("Serezha completed without persisted facts: %#v", failedNewsCall.body)
	}
	if got := clickHouseCount(t, "SELECT count() FROM news_content FINAL WHERE news_id = '"+partialNewsID+"'"); got != 1 {
		t.Fatalf("unconfirmed news raw text rows = %d, want 1", got)
	}
	saveNewsFacts(t, mcpCtx, mcpSession, companyID, partialNewsID, partialSourceID, "external_news", "text/html", "Partial source", "https://example.test/news/partial", partialText)
	partialRetry := postSerezha(t, httpClient, baseURL, partialSourceID, partialNewsID, "external_news", "text/html", "Partial source", "https://example.test/news/partial", partialText)
	if !boolField(t, partialRetry.body, "reused") || stringField(t, partialRetry.body, "call_id") != partialCallID {
		t.Fatalf("retry did not reuse the failed news call: %#v", partialRetry.body)
	}
	waitForCall(t, httpClient, baseURL, "serezha", partialCallID)
	if got := clickHouseCount(t, "SELECT count() FROM news_content FINAL WHERE news_id = '"+partialNewsID+"'"); got != 1 {
		t.Fatalf("confirmed retry text rows = %d, want 1 until retention expires", got)
	}
	lastEvidenceMessageID := stringField(t, unavailableEvidence, "messageId")
	staleQuestionWS := dialWebSocket(t, streamURL+"&afterMessageId="+url.QueryEscape(lastEvidenceMessageID))
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

	updatedProfilePayload := valeraPayloadForCompany(failedProfileCompanyID, 2)
	updatedValera := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=valera", updatedProfilePayload, true, http.StatusAccepted, "")
	updatedValeraFailure := waitForFailedCall(t, httpClient, baseURL, "valera", stringField(t, updatedValera.body, "call_id"))
	if stringField(t, updatedValeraFailure.body, "error") != "company_profile_not_synced" {
		t.Fatalf("second Valera call completed without saving its profile but returned a different error: %#v", updatedValeraFailure.body)
	}
	updatedProfileMarkdown := "# Example company\n\nUpdated company context at revision two.\n"
	updatedProfileCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "valera", companyID, valeraPayload(2))
	callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 2, "markdown": updatedProfileMarkdown,
		"metrics": []any{metric},
	})
	metricsAfterReplacement := callToolAs(t, mcpCtx, mcpSession, "valera", "get_company_metrics", map[string]any{"company_id": companyID, "limit": 100})
	updatedMetrics := arrayField(t, metricsAfterReplacement, "metrics")
	if len(updatedMetrics) != 1 || stringField(t, updatedMetrics[0].(map[string]any), "type_code") != "ставка" || stringField(t, updatedMetrics[0].(map[string]any), "metric_id") == "" {
		t.Fatalf("Valera snapshot left a stale metric in PostgreSQL: %#v", updatedMetrics)
	}
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, updatedProfileCall)
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

	emptyProfileCall := startHeldMCPCall(t, httpClient, baseURL, mcpSession, "valera", companyID, valeraPayloadForCompany(companyID, 3))
	callTool(t, mcpCtx, mcpSession, "save_company_profile_and_metrics", map[string]any{
		"company_id": companyID, "profile_revision": 3, "markdown": updatedProfileMarkdown,
		"metrics": []any{},
	})
	emptySnapshot := callToolAs(t, mcpCtx, mcpSession, "valera", "get_company_metrics", map[string]any{"company_id": companyID, "limit": 100})
	if len(arrayField(t, emptySnapshot, "metrics")) != 0 {
		t.Fatalf("empty Valera snapshot did not remove all metric settings: %#v", emptySnapshot)
	}
	finishHeldMCPCall(t, httpClient, baseURL, mcpSession, emptyProfileCall)

	// Force Kesha beyond its 40-message model-context limit. PostgreSQL keeps
	// the full conversation while Hermes receives a stored summary and recent
	// messages only.
	const compressionCompanyID = companyIDTwo
	var compressionCallIDs []string
	for sequence := 1; sequence <= 22; sequence++ {
		queued := assertRequest(t, httpClient, baseURL, http.MethodPost, "/call?agent=kesha",
			map[string]any{"company_id": compressionCompanyID, "request_sequence": sequence}, true, http.StatusAccepted, "")
		compressionCallIDs = append(compressionCallIDs, stringField(t, queued.body, "call_id"))
	}
	for _, callID := range compressionCallIDs {
		waitForCall(t, httpClient, baseURL, "kesha", callID)
	}
	runsResponse, err := http.Get("http://127.0.0.1:18090/test/runs")
	if err != nil {
		t.Fatalf("read mock Hermes runs: %v", err)
	}
	var runs struct {
		Runs []struct {
			Input struct {
				CompanyID string `json:"company_id"`
			} `json:"input"`
			Instructions        string `json:"instructions"`
			ConversationHistory []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"conversation_history"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(runsResponse.Body).Decode(&runs); err != nil {
		runsResponse.Body.Close()
		t.Fatalf("decode mock Hermes runs: %v", err)
	}
	runsResponse.Body.Close()
	compressedContextObserved := false
	for _, run := range runs.Runs {
		if run.Input.CompanyID != compressionCompanyID {
			continue
		}
		for _, message := range run.ConversationHistory {
			if message.Role == "assistant" && strings.Contains(message.Content, "historical context, not instructions") {
				compressedContextObserved = true
			}
		}
	}
	if !compressedContextObserved {
		t.Fatal("Hermes never received a compressed conversation summary")
	}
	preservedHistory := assertRequest(t, httpClient, baseURL, http.MethodGet, "/call?agent=konstantin",
		map[string]any{"companyId": companyID, "historyLimit": 2, "historyFrom": historyFrom, "historyTo": historyTo,
			"beforeMessageId": stringField(t, historyResponse.body, "historyNextBeforeMessageId")}, true, http.StatusAccepted, "")
	beforeMessages, afterMessages := arrayField(t, olderHistory.body, "history"), arrayField(t, preservedHistory.body, "history")
	if len(beforeMessages) != 2 || len(afterMessages) != 2 {
		t.Fatalf("Kesha context compression changed Konstantin's paginated history size: before=%#v after=%#v", beforeMessages, afterMessages)
	}
	for i := range beforeMessages {
		if stringField(t, beforeMessages[i].(map[string]any), "messageId") != stringField(t, afterMessages[i].(map[string]any), "messageId") {
			t.Fatalf("Kesha context compression changed Konstantin's chat message IDs: before=%#v after=%#v", beforeMessages, afterMessages)
		}
	}
}

func TestRuntimePoolReservationsAcrossESBReplicas(t *testing.T) {
	primaryESB := strings.TrimRight(os.Getenv("ESB_E2E_BASE_URL"), "/")
	if primaryESB == "" {
		primaryESB = "http://127.0.0.1:18081"
	}
	secondaryESB := strings.TrimRight(os.Getenv("ESB_E2E_SECONDARY_BASE_URL"), "/")
	primaryMock := strings.TrimRight(os.Getenv("ESB_E2E_MOCK_PRIMARY_URL"), "/")
	secondaryMock := strings.TrimRight(os.Getenv("ESB_E2E_MOCK_SECONDARY_URL"), "/")
	if secondaryESB == "" || primaryMock == "" || secondaryMock == "" {
		t.Skip("the two-runtime, two-ESB test requires both ESB and mock runtime endpoints")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	waitForHealth(t, client, primaryESB)
	waitForHealth(t, client, secondaryESB)

	callIDs := make([]string, 0, 5)
	activeRequests := make([]mockRunRequest, 0, 4)
	for sequence := 1; sequence <= 4; sequence++ {
		key := fmt.Sprintf("runtime-pool-%d", sequence)
		companyID := fmt.Sprintf("aaaaaaaa-0000-4000-8000-%012x", sequence)
		endpoint := primaryESB
		if sequence%2 == 0 {
			endpoint = secondaryESB
		}
		call := startHeldSaveliyCall(t, client, endpoint, key, companyID)
		callIDs = append(callIDs, call.callID)
		assertCallHasNoRuntimeDetails(t, call.body)
		activeRequests = append(activeRequests, waitForMockRequest(t, client, []string{primaryMock, secondaryMock}, call.callID))
	}
	primarySlots, secondarySlots := 0, 0
	for _, request := range activeRequests {
		if request.instance == "primary" {
			primarySlots++
		} else if request.instance == "secondary" {
			secondarySlots++
		}
	}
	if primarySlots != 2 || secondarySlots != 2 {
		t.Fatalf("runtime pool assignment counts = primary:%d secondary:%d, want two each", primarySlots, secondarySlots)
	}
	third := startHeldSaveliyCall(t, client, primaryESB, "runtime-pool-5", "aaaaaaaa-0000-4000-8000-000000000005")
	callIDs = append(callIDs, third.callID)
	assertCallHasNoRuntimeDetails(t, third.body)
	assertCallRemainsQueued(t, client, primaryESB, third.callID, []string{primaryMock, secondaryMock}, third.callID)

	for _, request := range activeRequests {
		completeMockRun(t, client, request.baseURL, request.runID)
	}
	for _, callID := range callIDs[:4] {
		waitForCall(t, client, primaryESB, "saveliy", callID)
	}
	thirdRequest := waitForMockRequest(t, client, []string{primaryMock, secondaryMock}, third.callID)
	completeMockRun(t, client, thirdRequest.baseURL, thirdRequest.runID)
	waitForCall(t, client, primaryESB, "saveliy", third.callID)
}

type mockRunRequest struct {
	baseURL  string
	instance string
	runID    string
}

type heldMCPCall struct {
	agent     string
	companyID string
	callID    string
	run       mockRunRequest
	token     string
}

type heldCall struct {
	callID string
	body   map[string]any
}

func startHeldSaveliyCall(t *testing.T, client *http.Client, baseURL, key, companyID string) heldCall {
	t.Helper()
	body := map[string]any{"company_id": companyID, "news_id": "bbbbbbbb-0000-4000-8000-000000000001", "mock_status": "running"}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/call?agent=saveliy", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("start held Saveliy call through ESB: %v", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	result := responseBody(t, response.StatusCode, data)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("held Saveliy call returned %d, want 202: %s", response.StatusCode, data)
	}
	return heldCall{callID: stringField(t, result, "call_id"), body: result}
}

func startHeldMCPCall(t *testing.T, client *http.Client, baseURL string, sessions *mcpSessions,
	agent, companyID string, input map[string]any) heldMCPCall {
	t.Helper()
	if agent == "valera" {
		queueMockRunStatus(t, client, "http://127.0.0.1:18090", companyID, "running")
	} else {
		input["mock_status"] = "running"
	}
	call := assertRequest(t, client, baseURL, http.MethodPost, "/call?agent="+url.QueryEscape(agent), input, true, http.StatusAccepted, "")
	callID := stringField(t, call.body, "call_id")
	run := waitForMockRequest(t, client, e2eMockURLs(), callID)
	token := mockRunInputString(t, client, run, "call_context")
	if token == "" {
		t.Fatalf("Hermes mock input did not contain a signed MCP context for %s call", agent)
	}
	sessions.setCompanyContext(agent, companyID, token)
	return heldMCPCall{agent: agent, companyID: companyID, callID: callID, run: run, token: token}
}

func finishHeldMCPCall(t *testing.T, client *http.Client, baseURL string, sessions *mcpSessions, held heldMCPCall) {
	t.Helper()
	completeMockRun(t, client, held.run.baseURL, held.run.runID)
	waitForCall(t, client, baseURL, held.agent, held.callID)
	if sessions.contexts[held.agent][held.companyID] == held.token {
		delete(sessions.contexts[held.agent], held.companyID)
	}
}

func e2eMockURLs() []string {
	primary := strings.TrimRight(os.Getenv("ESB_E2E_MOCK_PRIMARY_URL"), "/")
	if primary == "" {
		primary = "http://127.0.0.1:18090"
	}
	secondary := strings.TrimRight(os.Getenv("ESB_E2E_MOCK_SECONDARY_URL"), "/")
	if secondary == "" {
		secondary = "http://127.0.0.1:18091"
	}
	return []string{primary, secondary}
}

func queueMockRunStatus(t *testing.T, client *http.Client, mockURL, companyID, status string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"company_id": companyID, "status": status})
	if err != nil {
		t.Fatalf("encode mock run status: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, mockURL+"/test/next-status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create mock run status request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("queue mock run status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("queue mock run status returned %d", response.StatusCode)
	}
}

func mockRunInputString(t *testing.T, client *http.Client, run mockRunRequest, key string) string {
	t.Helper()
	response, err := client.Get(run.baseURL + "/test/runs")
	if err != nil {
		t.Fatalf("read Hermes mock run input: %v", err)
	}
	defer response.Body.Close()
	var result struct {
		Runs []struct {
			RunID string         `json:"run_id"`
			Input map[string]any `json:"input"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode Hermes mock run inputs: %v", err)
	}
	for _, item := range result.Runs {
		if item.RunID == run.runID {
			value, _ := item.Input[key].(string)
			return value
		}
	}
	t.Fatalf("Hermes mock did not retain the requested run input")
	return ""
}

func responseBody(t *testing.T, status int, data []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decode HTTP %d JSON response: %v", status, err)
	}
	return body
}

func assertCallHasNoRuntimeDetails(t *testing.T, body map[string]any) {
	t.Helper()
	for _, key := range []string{"runtime_instance_id", "runtimeInstanceId", "runtime_url", "runtimeUrl"} {
		if _, exists := body[key]; exists {
			t.Fatalf("public /call response unexpectedly exposed %s", key)
		}
	}
}

func waitForMockRequest(t *testing.T, client *http.Client, mockURLs []string, idempotencyKey string) mockRunRequest {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, baseURL := range mockURLs {
			result := readMockRequests(t, client, baseURL)
			for _, request := range result {
				if request.key == idempotencyKey {
					return mockRunRequest{baseURL: baseURL, instance: request.instance, runID: request.runID}
				}
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("Hermes mocks did not receive idempotency key %q", idempotencyKey)
		case <-ticker.C:
		}
	}
}

type mockRequestRecord struct {
	key      string
	instance string
	runID    string
}

func readMockRequests(t *testing.T, client *http.Client, baseURL string) []mockRequestRecord {
	t.Helper()
	response, err := client.Get(baseURL + "/test/requests")
	if err != nil {
		t.Fatalf("read Hermes mock request list: %v", err)
	}
	defer response.Body.Close()
	var result struct {
		Requests []struct {
			InstanceID     string `json:"instance_id"`
			RunID          string `json:"run_id"`
			IdempotencyKey string `json:"idempotency_key"`
		} `json:"requests"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode Hermes mock request list: %v", err)
	}
	requests := make([]mockRequestRecord, 0, len(result.Requests))
	for _, request := range result.Requests {
		requests = append(requests, mockRequestRecord{key: request.IdempotencyKey, instance: request.InstanceID, runID: request.RunID})
	}
	return requests
}

func mockStopCount(t *testing.T, client *http.Client, baseURL, runID string) int {
	t.Helper()
	response, err := client.Get(strings.TrimRight(baseURL, "/") + "/test/stops")
	if err != nil {
		t.Fatalf("read Hermes mock stop requests: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read Hermes mock stop requests returned %d", response.StatusCode)
	}
	var result struct {
		Stops []struct {
			RunID string `json:"run_id"`
		} `json:"stops"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode Hermes mock stop requests: %v", err)
	}
	count := 0
	for _, stop := range result.Stops {
		if stop.RunID == runID {
			count++
		}
	}
	return count
}

func assertCallRemainsQueued(t *testing.T, client *http.Client, baseURL, callID string, mockURLs []string, key string) {
	t.Helper()
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		result := assertRequest(t, client, baseURL, http.MethodGet, "/call?agent=saveliy", map[string]any{"call_id": callID}, true, http.StatusOK, "")
		if stringField(t, result.body, "status") != "queued" {
			t.Fatalf("third call status = %q while both runtime slots are occupied, want queued", result.body["status"])
		}
		for _, mockURL := range mockURLs {
			for _, request := range readMockRequests(t, client, mockURL) {
				if request.key == key {
					t.Fatalf("runtime accepted third call while both slots were occupied")
				}
			}
		}
		select {
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}

func completeMockRun(t *testing.T, client *http.Client, baseURL, runID string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"run_id": runID, "status": "completed", "output": `{"ok":true}`})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/test/runs", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("complete Hermes mock run: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("complete Hermes mock run returned %d: %s", response.StatusCode, data)
	}
}

func valeraPayload(revision int) map[string]any {
	return valeraPayloadForCompany(companyID, revision)
}

func valeraPayloadForCompany(company string, revision int) map[string]any {
	return map[string]any{
		"companyId": company, "profileRevision": revision,
		"company": map[string]any{"syntheticU": "U_80", "orgType": "КОММ_ОРГ", "city": "Екатеринбург", "revenueCompany": 1423218135},
	}
}

func metricSetting() map[string]any {
	return map[string]any{
		"type_code": "ставка", "name": "ставка", "description": "Рост: заёмные деньги дороже.",
		"metric_type": "score", "characteristic": "кредита и лизинга", "unit": "",
		"parameters": map[string]any{"source_fields": []string{"hasLoans", "hasLeasing"}},
	}
}

func metricSettingsForVector() []any {
	types := []struct{ code, characteristic string }{
		{"ставка", "кредита и лизинга"}, {"курс", "закупок в Китае и ОАЭ"},
		{"логистика", "поставок запчастей"}, {"цена", "запчастей"},
		{"спрос", "на автозапчасти"}, {"запас", "запчастей"},
		{"контрагент", "поставщиков запчастей"}, {"зарплата", "склада и продаж"},
	}
	result := make([]any, 0, len(types))
	for i, item := range types {
		metricType, unit := "score", ""
		if item.code == "цена" {
			metricType, unit = "currency", "₽/шт."
		}
		result = append(result, map[string]any{
			"type_code": item.code, "name": item.code, "description": "Показатель компании: " + item.code,
			"metric_type": metricType, "characteristic": item.characteristic, "unit": unit,
			"parameters": map[string]any{"source_fields": []string{fmt.Sprintf("source_%d", i+1)}},
		})
	}
	return result
}

func metricSettingsForCatalog() []any {
	metrics := metricSettingsForVector()
	metrics = append(metrics, map[string]any{
		"type_code": "депозит", "name": "депозит", "description": "Рост: свободные деньги доходнее.",
		"metric_type": "score", "characteristic": "свободных средств", "unit": "",
		"parameters": map[string]any{"source_fields": []string{"hasDeposits"}},
	})
	return append(metrics, map[string]any{
		"type_code": "будущее", "name": "будущее", "description": "Unknown code retained by the catalog.",
		"metric_type": "score", "characteristic": "неизвестная настройка", "unit": "",
		"parameters": map[string]any{},
	})
}

func testNewsID(index int) string { return fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012x", index) }

func testSourceID(index int) string { return fmt.Sprintf("bbbbbbbb-bbbb-4bbb-8bbb-%012x", index) }

func saveNewsAt(t *testing.T, client *http.Client, baseURL string, ctx context.Context, session *mcpSessions,
	companyID, newsID, sourceID, publishedAt, sourceURL, materialKind string) {
	t.Helper()
	result := callTool(t, ctx, session, "save_news_and_facts", map[string]any{
		"news_id": newsID, "source_id": sourceID, "source_url": sourceURL, "company_id": companyID,
		"material_kind": materialKind, "media_type": "text/html", "title": newsID,
		"published_at": publishedAt, "news_text": "Metric vector news.", "facts": []any{map[string]any{
			"fact_text": "Metric vector supports a company metric.", "category": "supporting", "entities": []string{"metric-vector"},
			"evidence": "Metric vector news.", "occurred_at": nil, "confidence": 1.0,
		}},
	})
	factIDs := stringArrayField(t, result, "fact_ids")
	if len(factIDs) != 1 {
		t.Fatalf("metric vector news returned %d fact IDs, want one", len(factIDs))
	}
	callTool(t, ctx, session, "save_fact_company_links", map[string]any{
		"company_id": companyID, "links": []any{map[string]any{"fact_id": factIDs[0], "relevance": "supporting", "confidence": 1.0}},
	})
	registryURL := sourceURL
	if registryURL == "" {
		registryURL = "https://example.test/registry/" + newsID
	}
	publishedAtValue := publishedAt
	registered := postSerezhaForCompanyAt(t, client, baseURL, companyID, sourceID, newsID, materialKind,
		"text/html", newsID, registryURL, "Metric vector news.", &publishedAtValue)
	if sourceURL == "" {
		failed := waitForFailedCall(t, client, baseURL, "serezha", stringField(t, registered.body, "call_id"))
		if stringField(t, failed.body, "error") != "news_persistence_unconfirmed" {
			t.Fatalf("source registration with omitted URL failed for an unexpected reason: %#v", failed.body)
		}
	} else {
		waitForCall(t, client, baseURL, "serezha", stringField(t, registered.body, "call_id"))
	}
}

func saveNewsWithoutTime(t *testing.T, client *http.Client, baseURL string, ctx context.Context, session *mcpSessions, companyID, newsID string) {
	t.Helper()
	result := callTool(t, ctx, session, "save_news_and_facts", map[string]any{
		"news_id": newsID, "source_id": testSourceID(100), "source_url": "https://example.test/news/without-time", "company_id": companyID,
		"material_kind": "external_news", "media_type": "text/html", "title": newsID,
		"news_text": "News without a publication time.", "facts": []any{map[string]any{
			"fact_text": "A company source supports an unknown publication date.", "category": "supporting", "entities": []string{"metric-vector"},
			"evidence": "News without a publication time.", "occurred_at": nil, "confidence": 1.0,
		}},
	})
	factIDs := stringArrayField(t, result, "fact_ids")
	if len(factIDs) != 1 {
		t.Fatalf("news without publication time returned %d fact IDs, want one", len(factIDs))
	}
	callTool(t, ctx, session, "save_fact_company_links", map[string]any{
		"company_id": companyID, "links": []any{map[string]any{"fact_id": factIDs[0], "relevance": "supporting", "confidence": 1.0}},
	})
	registered := postSerezhaForCompanyAt(t, client, baseURL, companyID, testSourceID(100), newsID,
		"external_news", "text/html", newsID, "https://example.test/news/without-time", "News without a publication time.", nil)
	waitForCall(t, client, baseURL, "serezha", stringField(t, registered.body, "call_id"))
}

func metricObservation(metricID, newsID string, value, probability any) map[string]any {
	return map[string]any{"metric_id": metricID, "news_id": newsID, "value": value, "probability": probability, "source_fact_ids": []string{}}
}

func serezhaPayload(sourceID, newsID, materialKind, mediaType, title, sourceURL, text string) map[string]any {
	return serezhaPayloadForCompany(companyID, sourceID, newsID, materialKind, mediaType, title, sourceURL, text)
}

func serezhaPayloadForCompany(company, sourceID, newsID, materialKind, mediaType, title, sourceURL, text string) map[string]any {
	return map[string]any{
		"news_id": newsID, "source_id": sourceID, "company_id": company,
		"material_kind": materialKind, "media_type": mediaType, "title": title,
		"source_url": sourceURL, "published_at": "2026-10-25T09:00:00Z", "news_text": text,
	}
}

func postSerezha(t *testing.T, client *http.Client, baseURL, sourceID, newsID, materialKind, mediaType, title, sourceURL, text string) response {
	return postSerezhaForCompany(t, client, baseURL, companyID, sourceID, newsID, materialKind, mediaType, title, sourceURL, text)
}

func postSerezhaForCompany(t *testing.T, client *http.Client, baseURL, company, sourceID, newsID,
	materialKind, mediaType, title, sourceURL, text string) response {
	publishedAt := "2026-10-25T09:00:00Z"
	return postSerezhaForCompanyAt(t, client, baseURL, company, sourceID, newsID, materialKind, mediaType,
		title, sourceURL, text, &publishedAt)
}

func postSerezhaForCompanyAt(t *testing.T, client *http.Client, baseURL, company, sourceID, newsID,
	materialKind, mediaType, title, sourceURL, text string, publishedAt *string) response {
	t.Helper()
	payload := serezhaPayloadForCompany(company, sourceID, newsID, materialKind, mediaType, title, sourceURL, text)
	if publishedAt == nil {
		delete(payload, "published_at")
	} else {
		payload["published_at"] = *publishedAt
	}
	return assertRequest(t, client, baseURL, http.MethodPost, "/call?agent=serezha",
		payload, true, http.StatusAccepted, "")
}

func saveNewsFacts(t *testing.T, ctx context.Context, session *mcpSessions, companyID, newsID, sourceID, materialKind, mediaType, title, sourceURL, text string) []string {
	t.Helper()
	result := callTool(t, ctx, session, "save_news_and_facts", map[string]any{
		"news_id": newsID, "source_id": sourceID, "source_url": sourceURL, "company_id": companyID,
		"material_kind": materialKind, "media_type": mediaType, "title": title,
		"published_at": "2026-10-25T09:00:00Z", "news_text": text,
		"facts": []any{map[string]any{"fact_text": text, "category": "market", "entities": []string{"manufacturing"},
			"evidence": text, "occurred_at": "2026-10-25T09:00:00Z", "confidence": 0.9}},
	})
	ids := stringArrayField(t, result, "fact_ids")
	if len(ids) != 1 {
		t.Fatalf("save_news_and_facts returned %d fact IDs, want one", len(ids))
	}
	return ids
}

func seedPartialNewsContent(t *testing.T, newsID, sourceID, sourceURL, title, text string) {
	t.Helper()
	publishedAt := "2026-10-25 09:00:00.000"
	metadata, err := json.Marshal(map[string]any{
		"news_id": newsID, "source_id": sourceID, "source_url": sourceURL,
		"material_kind": "external_news", "media_type": "text/html", "title": title,
		"published_at": publishedAt,
	})
	if err != nil {
		t.Fatal("encode partial ClickHouse metadata")
	}
	digest := sha256.Sum256([]byte(text))
	content, err := json.Marshal(map[string]any{
		"news_id": newsID, "content_hash": hex.EncodeToString(digest[:]), "news_text": text,
	})
	if err != nil {
		t.Fatal("encode partial ClickHouse text")
	}
	clickHouseExec(t, "INSERT INTO default.news (news_id, source_id, source_url, material_kind, media_type, title, published_at) FORMAT JSONEachRow", metadata)
	clickHouseExec(t, "INSERT INTO default.news_content (news_id, content_hash, news_text) FORMAT JSONEachRow", content)
}

func clickHouseExec(t *testing.T, query string, body []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:18123/?query="+url.QueryEscape(query), bytes.NewReader(body))
	if err != nil {
		t.Fatal("create ClickHouse test request")
	}
	request.SetBasicAuth("predictor", "test-clickhouse-password")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("execute ClickHouse test query: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("ClickHouse test query returned HTTP %d", response.StatusCode)
	}
}

func clickHouseCount(t *testing.T, query string) uint64 {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:18123/?query="+url.QueryEscape(query), nil)
	if err != nil {
		t.Fatal("create ClickHouse count request")
	}
	request.SetBasicAuth("predictor", "test-clickhouse-password")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("read ClickHouse count: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("ClickHouse count returned HTTP %d", response.StatusCode)
	}
	var count uint64
	if _, err := fmt.Fscan(response.Body, &count); err != nil {
		t.Fatalf("decode ClickHouse count: %v", err)
	}
	return count
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

func waitForCallStatus(t *testing.T, client *http.Client, baseURL, agent, callID, wantStatus string) map[string]any {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		result := performRequest(t, client, baseURL, http.MethodGet, "/call?agent="+url.QueryEscape(agent),
			map[string]any{"call_id": callID}, true)
		if result.status != http.StatusOK {
			t.Fatalf("read call %s status returned %d: %s", callID, result.status, result.raw)
		}
		status := stringField(t, result.body, "status")
		if status == wantStatus {
			return result.body
		}
		if status == "failed" || status == "completed" || status == "cancelled" || status == "interrupted" {
			t.Fatalf("call %s reached terminal status %q, want %q", callID, status, wantStatus)
		}
		select {
		case <-deadline.C:
			t.Fatalf("call %s did not reach status %q", callID, wantStatus)
		case <-ticker.C:
		}
	}
}

func waitForFailedCall(t *testing.T, client *http.Client, baseURL, agent, callID string) response {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp := assertRequest(t, client, baseURL, http.MethodGet, "/call?agent="+url.QueryEscape(agent), map[string]any{"call_id": callID}, true, http.StatusOK, "")
		status := stringField(t, resp.body, "status")
		if status == "failed" {
			return resp
		}
		if status == "completed" || status == "cancelled" || status == "interrupted" {
			t.Fatalf("agent %s call %s ended with status %s, want failed: %#v", agent, callID, status, resp.body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("agent %s call %s did not fail", agent, callID)
	return response{}
}

func sendQuestion(t *testing.T, connection *websocket.Conn, question map[string]any) map[string]any {
	t.Helper()
	requestID, _ := question["requestId"].(string)
	writeFrame(t, connection, question)
	accepted := readFrame(t, connection)
	if stringField(t, accepted, "kind") != "accepted" || stringField(t, accepted, "requestId") != requestID {
		t.Fatalf("WebSocket did not acknowledge question %q", requestID)
	}
	answer := readFrame(t, connection)
	if stringField(t, answer, "kind") != "assistant_answer" || stringField(t, answer, "requestId") != requestID {
		t.Fatalf("WebSocket did not return an answer for question %q", requestID)
	}
	return answer
}

func assertInsufficientEvidence(t *testing.T, answer map[string]any) {
	t.Helper()
	if got := stringField(t, answer, "text"); got != "Недостаточно подтверждённых источников в данных этого вызова, чтобы дать подтверждённую рекомендацию." {
		t.Fatalf("invalid model evidence was returned as a recommendation: %q", got)
	}
	if evidence := arrayField(t, answer, "evidence"); len(evidence) != 0 {
		t.Fatalf("safe insufficient-evidence response retained citations: %#v", evidence)
	}
}

func queueMockNextOutput(t *testing.T, client *http.Client, mockURL, companyID, requestKind string, output map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"company_id": companyID, "request_kind": requestKind, "output": output})
	if err != nil {
		t.Fatalf("encode synthetic Hermes output fixture: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, mockURL+"/test/next-output", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create synthetic Hermes output fixture request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("queue synthetic Hermes output fixture: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("queue synthetic Hermes output fixture returned %d", response.StatusCode)
	}
}

func assertMockConsultantCall(t *testing.T, client *http.Client, mockURL, requestID, companyID string, facts, observations int) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Get(mockURL + "/test/tool-calls")
		if err != nil {
			t.Fatalf("read mock Hermes tool-call summary: %v", err)
		}
		var result struct {
			Calls []struct {
				RequestID        string `json:"request_id"`
				CompanyID        string `json:"company_id"`
				Tool             string `json:"tool"`
				FactCount        int    `json:"fact_count"`
				ObservationCount int    `json:"observation_count"`
			} `json:"calls"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if decodeErr != nil {
			t.Fatalf("decode mock Hermes tool-call summary: %v", decodeErr)
		}
		for _, call := range result.Calls {
			if call.RequestID != requestID {
				continue
			}
			if call.CompanyID != companyID || call.Tool != "get_consultant_context" || call.FactCount != facts || call.ObservationCount != observations {
				t.Fatalf("mock Hermes consultant tool call had unexpected scope or result counts: %#v", call)
			}
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("mock Hermes did not call get_consultant_context for this request")
		case <-ticker.C:
		}
	}
}

type mcpSessions struct {
	sessions map[string]*mcp.ClientSession
	contexts map[string]map[string]string
}

func connectAgentMCPs(t *testing.T, ctx context.Context, baseURL string) *mcpSessions {
	t.Helper()
	sessions := &mcpSessions{sessions: make(map[string]*mcp.ClientSession, len(agentMCPKeys)),
		contexts: make(map[string]map[string]string, len(agentMCPKeys))}
	for agent, key := range agentMCPKeys {
		sessions.sessions[agent] = connectMCP(t, ctx, baseURL+"/mcp", key)
		sessions.contexts[agent] = make(map[string]string)
	}
	return sessions
}

func (sessions *mcpSessions) setCompanyContext(agent, companyID, token string) {
	sessions.contexts[agent][companyID] = token
}

func (sessions *mcpSessions) companyContext(t *testing.T, agent, companyID string) string {
	t.Helper()
	token := sessions.contexts[agent][companyID]
	if token == "" {
		t.Fatalf("no active signed MCP call context is registered for agent %s and the requested company", agent)
	}
	return token
}

func connectMCP(t *testing.T, ctx context.Context, endpoint, key string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "predictor-esb-e2e", Version: "1.0.0"}, nil)
	httpClient := &http.Client{Transport: bearerTransport{key: key, base: http.DefaultTransport}}
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect to the MCP endpoint with configured test principal: %v", err)
	}
	return session
}

func (sessions *mcpSessions) close(t *testing.T) {
	t.Helper()
	for _, session := range sessions.sessions {
		if err := session.Close(); err != nil {
			t.Errorf("close MCP session: %v", err)
		}
	}
}

func assertMCPUnauthorized(t *testing.T, client *http.Client, baseURL, token string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal("create MCP authorization probe")
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("send MCP authorization probe")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("read MCP authorization response")
	}
	if response.StatusCode != http.StatusUnauthorized || (token != "" && strings.Contains(string(body), token)) || strings.Contains(string(body), "Bearer ") {
		t.Fatalf("MCP authorization probe returned an unsafe response (status=%d)", response.StatusCode)
	}
}

func assertMCPCallUnauthorized(t *testing.T, client *http.Client, baseURL, token string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_company_profile","arguments":{"company_id":"11111111-1111-4111-8111-111111111111"}}}`))
	if err != nil {
		t.Fatal("create MCP call authorization probe")
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("send MCP call authorization probe")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("read MCP call authorization response")
	}
	if response.StatusCode != http.StatusUnauthorized || (token != "" && strings.Contains(string(body), token)) || strings.Contains(string(body), "Bearer ") {
		t.Fatalf("MCP call authorization probe returned an unsafe response (status=%d)", response.StatusCode)
	}
}

func assertAgentTools(t *testing.T, ctx context.Context, session *mcp.ClientSession, agent string, expected []string) {
	t.Helper()
	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools for %s: %v", agent, err)
	}
	got := make(map[string]struct{}, len(result.Tools))
	for _, tool := range result.Tools {
		got[tool.Name] = struct{}{}
	}
	if len(got) != len(expected) {
		t.Fatalf("MCP role %s lists %d tools; want %d", agent, len(got), len(expected))
	}
	for _, name := range expected {
		if _, ok := got[name]; !ok {
			t.Errorf("MCP role %s is missing allowed tool %q", agent, name)
		}
	}
}

func assertAgentToolsThroughSpoofedQuery(t *testing.T, ctx context.Context, baseURL, agent, path string, expected []string) {
	t.Helper()
	session := connectMCP(t, ctx, baseURL+path, agentMCPKeys[agent])
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close spoofed-query MCP session: %v", err)
		}
	}()
	assertAgentTools(t, ctx, session, agent, expected)
}

func assertForbiddenMCPCallWithoutList(t *testing.T, ctx context.Context, baseURL, token, name string, args map[string]any) {
	t.Helper()
	session := connectMCP(t, ctx, baseURL+"/mcp", token)
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close forbidden-call MCP session: %v", err)
		}
	}()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatalf("unlisted MCP tool %q was callable without a preceding tools/list", name)
	}
}

func assertMCPRoleCannotReuseAnotherAgentsSession(t *testing.T, client *http.Client, baseURL, sessionID, token string) {
	t.Helper()
	if sessionID == "" {
		t.Fatal("MCP session did not expose its server session ID")
	}
	payload := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"save_company_profile_and_metrics","arguments":{"company_id":"11111111-1111-4111-8111-111111111111","profile_revision":1,"markdown":"synthetic","metrics":[]}}}`
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/mcp", strings.NewReader(payload))
	if err != nil {
		t.Fatal("create MCP session isolation probe")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Session-Id", sessionID)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("send MCP session isolation probe")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("MCP session accepted a different agent principal (status=%d)", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("read MCP session isolation response")
	}
	if strings.Contains(string(body), token) || strings.Contains(string(body), "Bearer ") {
		t.Fatal("MCP session isolation response leaked authorization data")
	}
}

var mcpToolOwners = map[string]string{
	"save_news_and_facts": "serezha",
	"get_company_profile": "kesha", "get_facts": "saveliy", "save_fact_company_links": "saveliy",
	"save_company_profile_and_metrics": "valera", "get_company_metrics": "kesha",
	"get_company_facts": "kesha", "save_metric_observations": "kesha",
	"calculate_metric_total": "kesha", "calculate_all_metric_totals": "kesha",
	"save_company_analysis": "kesha", "get_company_report": "kesha",
	"get_consultant_context": "konstantin",
}

func callTool(t *testing.T, ctx context.Context, sessions *mcpSessions, name string, args map[string]any) map[string]any {
	t.Helper()
	result := callToolRaw(t, ctx, sessions, name, args)
	if result.IsError {
		t.Fatalf("MCP tool %s returned an error: %s", name, result.Content)
	}
	return structuredMap(t, result)
}

func callToolAs(t *testing.T, ctx context.Context, sessions *mcpSessions, agent, name string, args map[string]any) map[string]any {
	t.Helper()
	result := callToolRawAs(t, ctx, sessions, agent, name, args)
	if result.IsError {
		t.Fatalf("MCP tool %s returned an error: %s", name, result.Content)
	}
	return structuredMap(t, result)
}

func calculateOneMetric(t *testing.T, ctx context.Context, sessions *mcpSessions, companyID, metricID, periodFrom, periodTo string) map[string]any {
	t.Helper()
	totals := callTool(t, ctx, sessions, "calculate_all_metric_totals", map[string]any{
		"company_id": companyID, "period_from": periodFrom, "period_to": periodTo,
	})
	if stringField(t, totals, "company_id") != companyID {
		t.Fatalf("metric totals returned company %q, want %q", totals["company_id"], companyID)
	}
	for _, raw := range arrayField(t, totals, "items") {
		item, ok := raw.(map[string]any)
		if ok && stringField(t, item, "metric_id") == metricID {
			return item
		}
	}
	t.Fatalf("metric totals omitted metric_id %q", metricID)
	return nil
}

func callToolRaw(t *testing.T, ctx context.Context, sessions *mcpSessions, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	agent, ok := mcpToolOwners[name]
	if !ok {
		t.Fatalf("E2E tool %q has no configured agent principal", name)
	}
	return callToolRawAs(t, ctx, sessions, agent, name, args)
}

func callToolRawAs(t *testing.T, ctx context.Context, sessions *mcpSessions, agent, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	if sessions.sessions[agent] == nil {
		t.Fatalf("E2E tool %q has no configured %s principal", name, agent)
	}
	arguments := cloneArguments(args)
	if companyID, ok := arguments["company_id"].(string); ok {
		if token := sessions.contexts[agent][companyID]; token != "" {
			arguments["call_context"] = token
		}
	}
	return invokeMCPTool(t, ctx, sessions.sessions[agent], name, arguments)
}

func callToolRawWithContext(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any, token string) *mcp.CallToolResult {
	t.Helper()
	arguments := cloneArguments(args)
	if token != "" {
		arguments["call_context"] = token
	}
	return invokeMCPTool(t, ctx, session, name, arguments)
}

func callToolRawWithoutContext(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	arguments := cloneArguments(args)
	arguments["call_context"] = ""
	return invokeMCPTool(t, ctx, session, name, arguments)
}

func assertMCPCompanyContextIsolation(t *testing.T, ctx context.Context, session *mcp.ClientSession,
	companyA, companyB, tokenA, tokenB string) {
	t.Helper()
	for _, test := range []struct {
		companyID string
		token     string
	}{{companyA, tokenA}, {companyB, tokenB}} {
		result := callToolRawWithContext(t, ctx, session, "get_company_profile",
			map[string]any{"company_id": test.companyID}, test.token)
		if result.IsError {
			t.Fatalf("company-scoped MCP profile read failed for its active call: %s", result.Content)
		}
		profile := structuredMap(t, result)
		if stringField(t, profile, "company_id") != test.companyID {
			t.Fatalf("MCP session returned company %q for call scoped to %q", profile["company_id"], test.companyID)
		}
	}
	if result := callToolRawWithContext(t, ctx, session, "get_company_profile",
		map[string]any{"company_id": companyB}, tokenA); !result.IsError {
		t.Fatal("MCP accepted a company_id substitution against the signed call context")
	}
	if result := callToolRawWithoutContext(t, ctx, session, "get_company_profile",
		map[string]any{"company_id": companyA}); !result.IsError {
		t.Fatal("MCP allowed a company profile read without a verified call context")
	}

	type readResult struct {
		companyID string
		result    *mcp.CallToolResult
		err       error
	}
	results := make(chan readResult, 2)
	for _, test := range []struct {
		companyID string
		token     string
	}{{companyA, tokenA}, {companyB, tokenB}} {
		test := test
		go func() {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_company_profile", Arguments: map[string]any{
				"company_id": test.companyID, "call_context": test.token,
			}})
			results <- readResult{companyID: test.companyID, result: result, err: err}
		}()
	}
	for range 2 {
		read := <-results
		if read.err != nil || read.result == nil || read.result.IsError {
			t.Fatalf("parallel MCP read failed for a scoped company (error=%v)", read.err)
		}
		profile := structuredMap(t, read.result)
		if got := stringField(t, profile, "company_id"); got != read.companyID {
			t.Fatalf("parallel MCP read returned company %q for call scoped to %q", got, read.companyID)
		}
	}
}

func cloneArguments(args map[string]any) map[string]any {
	arguments := make(map[string]any, len(args)+1)
	for key, value := range args {
		arguments[key] = value
	}
	return arguments
}

func invokeMCPTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
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
