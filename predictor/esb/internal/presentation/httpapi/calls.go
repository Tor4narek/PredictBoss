package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/application/news"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func (h *callHandler) postCall(w http.ResponseWriter, r *http.Request) {
	agent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent")))
	if agent == "" {
		writeError(w, http.StatusBadRequest, "query parameter agent is required")
		return
	}
	input, err := decodeJSONBody(w, r, h.maxJSONBodyBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json_body")
		return
	}
	if agent == string(domain.AgentKonstantin) {
		var incoming konstantinCallBody
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
		writeJSON(w, http.StatusAccepted, acceptedCallResponse{
			Status: "accepted", CallID: call.ID, Agent: call.Agent,
			ScopeID: call.ScopeID, CallStatus: call.Status, Reused: reused,
		})
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
		writeJSON(w, http.StatusAccepted, acceptedCallResponse{
			Status: "accepted", CallID: call.ID, Agent: call.Agent,
			ScopeID: call.ScopeID, CallStatus: call.Status, Reused: reused,
		})
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
		writeJSON(w, http.StatusAccepted, acceptedCallResponse{
			Status: "accepted", CompanyID: companyID, ProfileRevision: profileRevision,
			CallID: call.ID, Reused: reused,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, acceptedCallResponse{
		CallID: call.ID, Agent: call.Agent, ScopeID: call.ScopeID,
		Status: string(call.Status), Reused: reused,
	})
}

func (h *callHandler) getCall(w http.ResponseWriter, r *http.Request) {
	data := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("data")))
	if data != "" {
		if data != "facts" || r.URL.Query().Get("agent") != "" {
			writeError(w, http.StatusBadRequest, "invalid_data_query")
			return
		}
		h.facts.getFacts(w, r)
		return
	}
	agent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent")))
	if agent == "" {
		writeError(w, http.StatusBadRequest, "query parameter agent is required")
		return
	}
	var query getCallRequest
	if err := decodeJSONInto(w, r, &query, h.maxJSONBodyBytes); err != nil {
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
		historyFrom := history.From.Format(time.RFC3339Nano)
		historyTo := history.To.Format(time.RFC3339Nano)
		conversationID := event.ConversationID
		requestID := event.RequestID
		if !ready {
			conversationID = call.ConversationID
			requestID = callRequestID(call.Input)
		}
		streamValues := url.Values{"companyId": {query.CompanyID}, "conversationId": {conversationID}}
		if ready {
			streamURL := "/stream?" + streamValues.Encode()
			writeJSON(w, http.StatusOK, consultantReadyResponse{
				Status: "ready", CompanyID: event.CompanyID, ConversationID: event.ConversationID,
				AnalysisID: event.AnalysisID, MessageID: event.MessageID, Text: event.Text,
				Evidence: event.Evidence, CreatedAt: event.CreatedAt, StreamURL: streamURL,
				History: history.Messages, HistoryHasMore: history.HasMore, HistoryFrom: historyFrom,
				HistoryTo: historyTo, HistoryNextBeforeMessageID: history.NextBeforeMessageID,
			})
			return
		}
		streamValues.Set("requestId", requestID)
		streamURL := "/stream?" + streamValues.Encode()
		writeJSON(w, http.StatusAccepted, consultantProcessingResponse{
			Status: "processing", CompanyID: query.CompanyID, ConversationID: conversationID,
			RequestID: requestID, StreamURL: streamURL, History: history.Messages,
			HistoryHasMore: history.HasMore, HistoryFrom: historyFrom, HistoryTo: historyTo,
			HistoryNextBeforeMessageID: history.NextBeforeMessageID,
		})
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
