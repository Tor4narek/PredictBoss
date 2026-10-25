package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/domain"
)

const maxRequestBytes = 2 << 20

type ReadyCheck func(context.Context) error

type Handler struct {
	apiKey    string
	calls     *calls.Service
	analytics *analytics.Service
	ready     ReadyCheck
}

func NewHandler(apiKey string, callsService *calls.Service, analyticsService *analytics.Service, ready ReadyCheck) http.Handler {
	handler := &Handler{apiKey: apiKey, calls: callsService, analytics: analyticsService, ready: ready}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /readyz", handler.authorize(http.HandlerFunc(handler.readiness)))
	mux.Handle("POST /call", handler.authorize(http.HandlerFunc(handler.postCall)))
	mux.Handle("GET /call", handler.authorize(http.HandlerFunc(handler.getCall)))
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
	call, reused, err := h.calls.Create(r.Context(), agent, input, r.Header.Get("Idempotency-Key"))
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
	writeJSON(w, http.StatusAccepted, map[string]any{"call_id": call.ID, "agent": call.Agent, "scope_id": call.ScopeID, "status": call.Status, "reused": reused})
}

type getCallRequest struct {
	CallID     string `json:"call_id,omitempty"`
	CompanyID  string `json:"company_id,omitempty"`
	PeriodFrom string `json:"period_from,omitempty"`
	PeriodTo   string `json:"period_to,omitempty"`
}

func (h *Handler) getCall(w http.ResponseWriter, r *http.Request) {
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
