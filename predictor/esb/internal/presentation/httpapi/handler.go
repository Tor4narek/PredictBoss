package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/application/news"
	"github.com/predictboss/predictor/esb/internal/domain"
)

const maxRequestBytes = 2 << 20

type ReadyCheck func(context.Context) error

type Handler struct {
	apiKey         string
	calls          *calls.Service
	analytics      *analytics.Service
	companyContext *companycontext.Service
	news           *news.Service
	ready          ReadyCheck
}

func NewHandler(apiKey string, callsService *calls.Service, analyticsService *analytics.Service, companyContextService *companycontext.Service, newsService *news.Service, ready ReadyCheck) http.Handler {
	handler := &Handler{apiKey: apiKey, calls: callsService, analytics: analyticsService, companyContext: companyContextService, news: newsService, ready: ready}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /readyz", handler.authorize(http.HandlerFunc(handler.readiness)))
	mux.Handle("POST /call", handler.authorize(http.HandlerFunc(handler.postCall)))
	mux.Handle("GET /call", handler.authorize(http.HandlerFunc(handler.getCall)))
	mux.Handle("GET /stream", handler.authorize(http.HandlerFunc(handler.stream)))
	return mux
}

func (h *Handler) postCall(w http.ResponseWriter, r *http.Request) {
	agent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent")))
	if agent == "" {
		writeError(w, http.StatusBadRequest, "query parameter agent is required")
		return
	}
	input, err := decodeJSONBody(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json_body")
		return
	}
	if agent == string(domain.AgentKonstantin) {
		var incoming struct {
			CompanyID      string `json:"companyId"`
			CompanyIDSnake string `json:"company_id"`
		}
		if err := json.Unmarshal(input, &incoming); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json_body")
			return
		}
		if incoming.CompanyID == "" {
			incoming.CompanyID = incoming.CompanyIDSnake
		}
		call, reused, err := h.calls.QueueKonstantinAnalysis(r.Context(), incoming.CompanyID)
		if errors.Is(err, calls.ErrInvalidCall) {
			writeError(w, http.StatusBadRequest, "invalid_company_id")
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusConflict, "company_profile_not_found")
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "profile_update_pending")
			return
		}
		if err != nil {
			log.Printf("enqueue Konstantin analysis: %v", err)
			writeError(w, http.StatusInternalServerError, "analysis_request_failed")
			return
		}
		w.Header().Set("Location", "/call?agent=konstantin")
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "call_id": call.ID, "agent": call.Agent,
			"scope_id": call.ScopeID, "call_status": call.Status, "reused": reused})
		return
	}
	if agent == string(domain.AgentSerezha) {
		if h.news == nil {
			writeError(w, http.StatusServiceUnavailable, "news_ingestion_unavailable")
			return
		}
		call, reused, err := h.news.Submit(r.Context(), input)
		if errors.Is(err, news.ErrInvalidInput) || errors.Is(err, calls.ErrInvalidCall) {
			writeError(w, http.StatusBadRequest, "invalid_news_request")
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "source_id_conflict")
			return
		}
		if err != nil {
			log.Printf("enqueue Serezha news call: %v", err)
			writeError(w, http.StatusInternalServerError, "news_ingestion_failed")
			return
		}
		w.Header().Set("Location", "/call?agent=serezha")
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "call_id": call.ID,
			"agent": call.Agent, "scope_id": call.ScopeID, "call_status": call.Status, "reused": reused})
		return
	}
	if agent == string(domain.AgentValera) {
		input, err = normalizeValeraInput(input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_valera_payload")
			return
		}
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	var profileRevision int64
	var companyID string
	if agent == string(domain.AgentValera) {
		var valeraInput struct {
			CompanyID       string `json:"company_id"`
			ProfileRevision int64  `json:"profile_revision"`
		}
		_ = json.Unmarshal(input, &valeraInput)
		companyID, profileRevision = valeraInput.CompanyID, valeraInput.ProfileRevision
		if profileRevision > 0 {
			if h.companyContext == nil {
				writeError(w, http.StatusServiceUnavailable, "company_profile_storage_unavailable")
				return
			}
			if err := h.companyContext.ReserveProfileRevision(r.Context(), companyID, profileRevision, input); err != nil {
				if errors.Is(err, domain.ErrConflict) {
					writeError(w, http.StatusConflict, "profile_revision_conflict")
					return
				}
				if errors.Is(err, companycontext.ErrInvalidInput) {
					writeError(w, http.StatusBadRequest, "invalid_valera_payload")
					return
				}
				log.Printf("reserve company profile revision: %v", err)
				writeError(w, http.StatusInternalServerError, "profile_revision_reservation_failed")
				return
			}
			idempotencyKey = "valera:" + companyID + ":" + strconv.FormatInt(profileRevision, 10)
		}
	}
	call, reused, err := h.calls.Create(r.Context(), agent, input, idempotencyKey)
	if err != nil {
		if errors.Is(err, calls.ErrInvalidCall) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "idempotency_key_conflict")
			return
		}
		log.Printf("enqueue agent call: %v", err)
		writeError(w, http.StatusInternalServerError, "call_persistence_failed")
		return
	}
	w.Header().Set("Location", "/call?agent="+string(call.Agent))
	if agent == string(domain.AgentValera) && profileRevision > 0 {
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "companyId": companyID, "profileRevision": profileRevision, "callId": call.ID, "reused": reused})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"call_id": call.ID, "agent": call.Agent, "scope_id": call.ScopeID, "status": call.Status, "reused": reused})
}

type getCallRequest struct {
	CallID         string `json:"call_id,omitempty"`
	CompanyID      string `json:"company_id,omitempty"`
	CompanyIDCamel string `json:"companyId,omitempty"`
	PeriodFrom     string `json:"period_from,omitempty"`
	PeriodTo       string `json:"period_to,omitempty"`
	HistoryLimit   int    `json:"historyLimit,omitempty"`
	HistoryFrom    string `json:"historyFrom,omitempty"`
	HistoryTo      string `json:"historyTo,omitempty"`
	BeforeMessage  string `json:"beforeMessageId,omitempty"`
}

type getFactsRequest struct {
	CompanyID       string `json:"companyId,omitempty"`
	CompanyIDSnake  string `json:"company_id,omitempty"`
	NewsID          string `json:"newsId,omitempty"`
	NewsIDSnake     string `json:"news_id,omitempty"`
	SourceID        string `json:"sourceId,omitempty"`
	SourceIDSnake   string `json:"source_id,omitempty"`
	MaterialKind    string `json:"materialKind,omitempty"`
	MaterialKindAlt string `json:"material_kind,omitempty"`
	Category        string `json:"category,omitempty"`
	Entity          string `json:"entity,omitempty"`
	PeriodFrom      string `json:"periodFrom,omitempty"`
	PeriodFromSnake string `json:"period_from,omitempty"`
	PeriodTo        string `json:"periodTo,omitempty"`
	PeriodToSnake   string `json:"period_to,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	BeforeFactID    string `json:"beforeFactId,omitempty"`
	BeforeFactSnake string `json:"before_fact_id,omitempty"`
}

func (h *Handler) getCall(w http.ResponseWriter, r *http.Request) {
	data := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("data")))
	if data != "" {
		if data != "facts" || r.URL.Query().Get("agent") != "" {
			writeError(w, http.StatusBadRequest, "invalid_data_query")
			return
		}
		h.getFacts(w, r)
		return
	}
	agent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent")))
	if agent == "" {
		writeError(w, http.StatusBadRequest, "query parameter agent is required")
		return
	}
	var query getCallRequest
	if err := decodeJSONInto(w, r, &query); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json_body")
		return
	}
	if query.CompanyID == "" {
		query.CompanyID = query.CompanyIDCamel
	}
	if query.CallID != "" && query.CompanyID != "" {
		writeError(w, http.StatusBadRequest, "provide call_id or company_id")
		return
	}
	if query.CallID != "" {
		call, err := h.calls.GetForAgent(r.Context(), query.CallID, agent)
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "call_not_found")
			return
		}
		if err != nil {
			if errors.Is(err, calls.ErrInvalidCall) {
				writeError(w, http.StatusBadRequest, "unknown_agent")
				return
			}
			log.Printf("read call: %v", err)
			writeError(w, http.StatusInternalServerError, "call_lookup_failed")
			return
		}
		writeJSON(w, http.StatusOK, call)
		return
	}
	if query.CompanyID == "" {
		writeError(w, http.StatusBadRequest, "body must contain call_id or company_id")
		return
	}
	if agent == string(domain.AgentKonstantin) {
		event, call, ready, err := h.calls.StartKonstantinAnalysis(r.Context(), query.CompanyID)
		if err != nil {
			if errors.Is(err, calls.ErrInvalidCall) {
				writeError(w, http.StatusBadRequest, "invalid_company_id")
				return
			}
			if errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusConflict, "company_profile_not_found")
				return
			}
			if errors.Is(err, domain.ErrConflict) {
				writeError(w, http.StatusConflict, "profile_update_pending")
				return
			}
			log.Printf("start Konstantin analysis: %v", err)
			writeError(w, http.StatusInternalServerError, "analysis_request_failed")
			return
		}
		history, err := h.calls.KonstantinHistory(r.Context(), query.CompanyID,
			query.HistoryFrom, query.HistoryTo, query.BeforeMessage, query.HistoryLimit)
		if errors.Is(err, calls.ErrInvalidHistoryRequest) {
			writeError(w, http.StatusBadRequest, "invalid_history_request")
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "history_cursor_not_found")
			return
		}
		if err != nil {
			log.Printf("read Konstantin chat history: %v", err)
			writeError(w, http.StatusInternalServerError, "chat_history_lookup_failed")
			return
		}
		historyFields := map[string]any{
			"history": history.Messages, "historyHasMore": history.HasMore,
			"historyFrom": history.From.Format(time.RFC3339Nano), "historyTo": history.To.Format(time.RFC3339Nano),
		}
		if history.NextBeforeMessageID != "" {
			historyFields["historyNextBeforeMessageId"] = history.NextBeforeMessageID
		}
		conversationID := event.ConversationID
		requestID := event.RequestID
		if !ready {
			conversationID = call.ConversationID
			requestID = callRequestID(call.Input)
		}
		streamValues := url.Values{"companyId": {query.CompanyID}, "conversationId": {conversationID}}
		if ready {
			streamURL := "/stream?" + streamValues.Encode()
			response := map[string]any{"status": "ready", "companyId": event.CompanyID, "conversationId": event.ConversationID,
				"analysisId": event.AnalysisID, "messageId": event.MessageID, "text": event.Text, "evidence": event.Evidence,
				"createdAt": event.CreatedAt, "streamUrl": streamURL}
			for key, value := range historyFields {
				response[key] = value
			}
			writeJSON(w, http.StatusOK, response)
			return
		}
		streamValues.Set("requestId", requestID)
		streamURL := "/stream?" + streamValues.Encode()
		response := map[string]any{"status": "processing", "companyId": query.CompanyID,
			"conversationId": conversationID, "requestId": requestID, "streamUrl": streamURL}
		for key, value := range historyFields {
			response[key] = value
		}
		writeJSON(w, http.StatusAccepted, response)
		return
	}
	if agent != string(domain.AgentKesha) {
		writeError(w, http.StatusBadRequest, "company analytics are served by agent kesha")
		return
	}
	report, err := h.analytics.CompanyReport(r.Context(), query.CompanyID, query.PeriodFrom, query.PeriodTo)
	if err != nil {
		if errors.Is(err, analytics.ErrInvalidAnalyticsInput) {
			writeError(w, http.StatusBadRequest, "invalid_company_or_period")
			return
		}
		log.Printf("read company analytics: %v", err)
		writeError(w, http.StatusInternalServerError, "company_analytics_lookup_failed")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) getFacts(w http.ResponseWriter, r *http.Request) {
	var request getFactsRequest
	if err := decodeJSONInto(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json_body")
		return
	}
	if request.CompanyID == "" {
		request.CompanyID = request.CompanyIDSnake
	}
	if request.NewsID == "" {
		request.NewsID = request.NewsIDSnake
	}
	if request.SourceID == "" {
		request.SourceID = request.SourceIDSnake
	}
	if request.MaterialKind == "" {
		request.MaterialKind = request.MaterialKindAlt
	}
	if request.PeriodFrom == "" {
		request.PeriodFrom = request.PeriodFromSnake
	}
	if request.PeriodTo == "" {
		request.PeriodTo = request.PeriodToSnake
	}
	if request.BeforeFactID == "" {
		request.BeforeFactID = request.BeforeFactSnake
	}
	page, err := h.analytics.QueryFacts(r.Context(), analytics.FactsRequest{
		CompanyID: request.CompanyID, NewsID: request.NewsID, SourceID: request.SourceID,
		MaterialKind: request.MaterialKind, Category: request.Category, Entity: request.Entity,
		PeriodFrom: request.PeriodFrom, PeriodTo: request.PeriodTo,
		BeforeFactID: request.BeforeFactID, Limit: request.Limit,
	})
	if errors.Is(err, analytics.ErrInvalidAnalyticsInput) {
		writeError(w, http.StatusBadRequest, "invalid_facts_query")
		return
	}
	if err != nil {
		log.Printf("read facts from ClickHouse: %v", err)
		writeError(w, http.StatusInternalServerError, "facts_lookup_failed")
		return
	}
	response := map[string]any{"data": "facts", "facts": page.Facts, "hasMore": page.HasMore}
	if page.NextBeforeID != "" {
		response["nextBeforeFactId"] = page.NextBeforeID
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("agent") {
		writeError(w, http.StatusBadRequest, "stream_does_not_accept_agent")
		return
	}
	companyID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("companyId")))
	conversationID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("conversationId")))
	afterMessageID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("afterMessageId")))
	initialRequestID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("requestId")))
	if companyID == "" || conversationID == "" {
		writeError(w, http.StatusBadRequest, "companyId_and_conversationId_are_required")
		return
	}
	knownConversationID, err := h.calls.KonstantinConversation(r.Context(), companyID)
	if err != nil || knownConversationID != conversationID {
		writeError(w, http.StatusNotFound, "conversation_not_found")
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(context.Background())
	outgoing := make(chan []byte, 64)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			var payload []byte
			select {
			case <-ctx.Done():
				return
			case payload = <-outgoing:
			}
			writeCtx, stop := context.WithTimeout(ctx, 15*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, payload)
			stop()
			if err != nil {
				cancel()
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-writerDone
	}()
	send := func(value any) error {
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		select {
		case outgoing <- payload:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	var sentMu sync.Mutex
	sentMessages := make(map[string]struct{})
	sendEvent := func(event domain.ConsultantEvent) error {
		sentMu.Lock()
		if _, exists := sentMessages[event.MessageID]; exists {
			sentMu.Unlock()
			return nil
		}
		sentMessages[event.MessageID] = struct{}{}
		sentMu.Unlock()
		return send(event)
	}
	if afterMessageID != "" {
		events, err := h.calls.KonstantinEventsAfter(r.Context(), companyID, conversationID, afterMessageID, 500)
		if err != nil {
			payload, _ := json.Marshal(map[string]any{"kind": "error", "error": "invalid_replay_cursor"})
			writeCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
			_ = conn.Write(writeCtx, websocket.MessageText, payload)
			stop()
			return
		}
		for _, event := range events {
			if err := sendEvent(event); err != nil {
				return
			}
		}
	} else {
		events, err := h.calls.KonstantinEventsAfter(r.Context(), companyID, conversationID, "", 500)
		if err == nil {
			for _, event := range events {
				if err := sendEvent(event); err != nil {
					return
				}
			}
		}
	}
	if initialRequestID != "" {
		go func() {
			event, err := h.calls.WaitKonstantinEvent(ctx, initialRequestID)
			if err == nil && event.CompanyID == companyID && event.ConversationID == conversationID {
				_ = sendEvent(event)
			} else if err != nil && ctx.Err() == nil {
				_ = send(map[string]any{"kind": "error", "requestId": initialRequestID, "error": "analysis_failed"})
			}
		}()
	}
	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var frame struct {
			Kind           string `json:"kind"`
			RequestID      string `json:"requestId"`
			CompanyID      string `json:"companyId"`
			ConversationID string `json:"conversationId"`
			AnalysisID     string `json:"analysisId"`
			Text           string `json:"text"`
		}
		if json.Unmarshal(payload, &frame) != nil || frame.Kind != "question" {
			_ = send(map[string]any{"kind": "error", "error": "expected_question_frame"})
			continue
		}
		frame.CompanyID = strings.ToLower(strings.TrimSpace(frame.CompanyID))
		frame.ConversationID = strings.ToLower(strings.TrimSpace(frame.ConversationID))
		frame.RequestID = strings.ToLower(strings.TrimSpace(frame.RequestID))
		frame.AnalysisID = strings.ToLower(strings.TrimSpace(frame.AnalysisID))
		if (frame.CompanyID != "" && frame.CompanyID != companyID) || (frame.ConversationID != "" && frame.ConversationID != conversationID) {
			_ = send(map[string]any{"kind": "error", "requestId": frame.RequestID, "error": "subscription_scope_mismatch"})
			continue
		}
		call, reused, err := h.calls.AskKonstantin(ctx, domain.ConsultantRequest{Kind: frame.Kind, RequestID: frame.RequestID,
			CompanyID: companyID, ConversationID: conversationID, AnalysisID: frame.AnalysisID, Text: frame.Text})
		if errors.Is(err, calls.ErrAnalysisNotReady) {
			_ = send(map[string]any{"kind": "error", "requestId": frame.RequestID, "error": "analysis_not_ready"})
			continue
		}
		if errors.Is(err, calls.ErrInvalidCall) {
			_ = send(map[string]any{"kind": "error", "requestId": frame.RequestID, "error": "invalid_question"})
			continue
		}
		if err != nil {
			_ = send(map[string]any{"kind": "error", "requestId": frame.RequestID, "error": "question_persistence_failed"})
			continue
		}
		if err := send(map[string]any{"kind": "accepted", "requestId": frame.RequestID, "callId": call.ID, "reused": reused}); err != nil {
			return
		}
		requestID := strings.ToLower(frame.RequestID)
		go func() {
			event, err := h.calls.WaitKonstantinEvent(ctx, requestID)
			if err == nil {
				_ = sendEvent(event)
			} else if ctx.Err() == nil {
				_ = send(map[string]any{"kind": "error", "requestId": requestID, "error": "assistant_response_failed"})
			}
		}()
	}
}

func normalizeValeraInput(input json.RawMessage) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(input, &body); err != nil {
		return nil, err
	}
	if nested, ok := body["payload"]; ok {
		if err := json.Unmarshal(nested, &body); err != nil {
			return nil, err
		}
	}
	companyID := body["companyId"]
	if len(companyID) == 0 {
		return input, nil
	}
	profileRevision := body["profileRevision"]
	company := body["company"]
	if len(profileRevision) == 0 || len(company) == 0 {
		return nil, errors.New("companyId, profileRevision and company are required")
	}
	var revision int64
	if err := json.Unmarshal(profileRevision, &revision); err != nil || revision < 1 {
		return nil, errors.New("profileRevision must be positive")
	}
	var id string
	if err := json.Unmarshal(companyID, &id); err != nil {
		return nil, err
	}
	id = strings.ToLower(strings.TrimSpace(id))
	return json.Marshal(struct {
		CompanyID       string          `json:"company_id"`
		ProfileRevision int64           `json:"profile_revision"`
		CompanyData     json.RawMessage `json:"company_data"`
	}{id, revision, company})
}

func callRequestID(raw json.RawMessage) string {
	var value struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(raw, &value)
	return value.RequestID
}

func (h *Handler) readiness(w http.ResponseWriter, r *http.Request) {
	if h.ready != nil {
		if err := h.ready(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+h.apiKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := decodeJSONInto(w, r, &raw); err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, errors.New("JSON object required")
	}
	return raw, nil
}

func decodeJSONInto(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
