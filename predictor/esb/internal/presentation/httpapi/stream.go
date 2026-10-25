package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func (h *consultantHandler) stream(w http.ResponseWriter, r *http.Request) {
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
	conn.SetReadLimit(h.maxFrameBytes)
	ctx, cancel := context.WithCancel(context.Background())
	outgoing := make(chan []byte, h.outgoingQueue)
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
			writeCtx, stop := context.WithTimeout(ctx, h.writeTimeout)
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
		events, err := h.calls.KonstantinEventsAfter(r.Context(), companyID, conversationID, afterMessageID, h.replayLimit)
		if err != nil {
			payload, _ := json.Marshal(errorFrame{Kind: "error", Error: "invalid_replay_cursor"})
			writeCtx, stop := context.WithTimeout(context.Background(), h.writeTimeout)
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
		events, err := h.calls.KonstantinEventsAfter(r.Context(), companyID, conversationID, "", h.replayLimit)
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
				_ = send(errorFrame{Kind: "error", RequestID: initialRequestID, Error: "analysis_failed"})
			}
		}()
	}
	for {
		readCtx := ctx
		cancelRead := func() {}
		if h.readTimeout > 0 {
			readCtx, cancelRead = context.WithTimeout(ctx, h.readTimeout)
		}
		_, payload, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			return
		}
		var frame questionFrame
		if json.Unmarshal(payload, &frame) != nil || frame.Kind != "question" {
			_ = send(errorFrame{Kind: "error", Error: "expected_question_frame"})
			continue
		}
		frame.CompanyID = strings.ToLower(strings.TrimSpace(frame.CompanyID))
		frame.ConversationID = strings.ToLower(strings.TrimSpace(frame.ConversationID))
		frame.RequestID = strings.ToLower(strings.TrimSpace(frame.RequestID))
		frame.AnalysisID = strings.ToLower(strings.TrimSpace(frame.AnalysisID))
		if (frame.CompanyID != "" && frame.CompanyID != companyID) || (frame.ConversationID != "" && frame.ConversationID != conversationID) {
			_ = send(errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "subscription_scope_mismatch"})
			continue
		}
		call, reused, err := h.calls.AskKonstantin(ctx, domain.ConsultantRequest{Kind: frame.Kind, RequestID: frame.RequestID,
			CompanyID: companyID, ConversationID: conversationID, AnalysisID: frame.AnalysisID, Text: frame.Text})
		if errors.Is(err, calls.ErrAnalysisNotReady) {
			_ = send(errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "analysis_not_ready"})
			continue
		}
		if errors.Is(err, calls.ErrInvalidCall) {
			_ = send(errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "invalid_question"})
			continue
		}
		if err != nil {
			_ = send(errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "question_persistence_failed"})
			continue
		}
		if err := send(acceptedFrame{Kind: "accepted", RequestID: frame.RequestID, CallID: call.ID, Reused: reused}); err != nil {
			return
		}
		requestID := strings.ToLower(frame.RequestID)
		go func() {
			event, err := h.calls.WaitKonstantinEvent(ctx, requestID)
			if err == nil {
				_ = sendEvent(event)
			} else if ctx.Err() == nil {
				_ = send(errorFrame{Kind: "error", RequestID: requestID, Error: "assistant_response_failed"})
			}
		}()
	}
}
