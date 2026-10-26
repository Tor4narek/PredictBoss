package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

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
	defer func() {
		if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
			log.Printf("consultant WebSocket close failed (company_id=%s error_type=%T)", companyID, err)
		}
	}()
	conn.SetReadLimit(h.maxFrameBytes)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	outgoing := make(chan []byte, h.outgoingQueue)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case payload := <-outgoing:
				writeCtx, stop := context.WithTimeout(ctx, h.writeTimeout)
				err := conn.Write(writeCtx, websocket.MessageText, payload)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-writerDone }()
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
	sender := orderedStreamSender{last: make(map[string]int64), sent: make(map[string]bool), terminal: make(map[string]bool)}
	sendOrdered := func(requestID string, sequence int64, terminal bool, value any) error {
		return sender.send(ctx, send, requestID, sequence, terminal, value)
	}
	sendEvent := func(event domain.ConsultantEvent) error {
		if event.CompanyID != companyID || event.ConversationID != conversationID {
			return nil
		}
		terminal := event.Kind == "analysis_result" || event.Kind == "assistant_answer"
		return sendOrdered(event.RequestID, event.Sequence, terminal, event)
	}

	if afterMessageID != "" {
		events, err := h.calls.KonstantinEventsAfter(r.Context(), companyID, conversationID, afterMessageID, h.replayLimit)
		if err != nil {
			if sendErr := send(errorFrame{Kind: "error", Error: "invalid_replay_cursor"}); sendErr != nil {
				return
			}
		} else {
			for _, event := range events {
				if err := sendEvent(event); err != nil {
					return
				}
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

	watching := make(map[string]struct{})
	watchMu := sync.Mutex{}
	watchSlots := make(chan struct{}, max(1, h.outgoingQueue))
	var watchers sync.WaitGroup
	startWatcher := func(requestID string) {
		requestID = strings.ToLower(strings.TrimSpace(requestID))
		if requestID == "" {
			return
		}
		watchMu.Lock()
		if _, exists := watching[requestID]; exists {
			watchMu.Unlock()
			return
		}
		select {
		case watchSlots <- struct{}{}:
			watching[requestID] = struct{}{}
			watchers.Add(1)
			watchMu.Unlock()
		default:
			watchMu.Unlock()
			sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: requestID,
				Error: "stream_watch_limit", Incomplete: true, Terminal: true}, companyID)
			return
		}
		go func() {
			defer watchers.Done()
			defer func() {
				<-watchSlots
				watchMu.Lock()
				delete(watching, requestID)
				watchMu.Unlock()
			}()
			h.watchKonstantin(ctx, companyID, conversationID, requestID, sendOrdered, sendEvent)
		}()
	}
	defer func() { cancel(); watchers.Wait() }()
	if initialRequestID != "" {
		startWatcher(initialRequestID)
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
		var envelope struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(payload, &envelope) != nil {
			if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", Error: "invalid_frame"}, companyID) {
				return
			}
			continue
		}
		switch envelope.Kind {
		case "question":
			var frame questionFrame
			if json.Unmarshal(payload, &frame) != nil {
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", Error: "invalid_question"}, companyID) {
					return
				}
				continue
			}
			frame.CompanyID = strings.ToLower(strings.TrimSpace(frame.CompanyID))
			frame.ConversationID = strings.ToLower(strings.TrimSpace(frame.ConversationID))
			frame.RequestID = strings.ToLower(strings.TrimSpace(frame.RequestID))
			frame.AnalysisID = strings.ToLower(strings.TrimSpace(frame.AnalysisID))
			if (frame.CompanyID != "" && frame.CompanyID != companyID) || (frame.ConversationID != "" && frame.ConversationID != conversationID) {
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "subscription_scope_mismatch"}, companyID) {
					return
				}
				continue
			}
			call, reused, err := h.calls.AskKonstantin(ctx, domain.ConsultantRequest{Kind: frame.Kind, RequestID: frame.RequestID,
				CompanyID: companyID, ConversationID: conversationID, AnalysisID: frame.AnalysisID, Text: frame.Text})
			if errors.Is(err, calls.ErrAnalysisNotReady) {
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "analysis_not_ready"}, companyID) {
					return
				}
				continue
			}
			if errors.Is(err, calls.ErrInvalidCall) {
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "invalid_question"}, companyID) {
					return
				}
				continue
			}
			if err != nil {
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "question_persistence_failed"}, companyID) {
					return
				}
				continue
			}
			if err := sendOrdered(frame.RequestID, 0, false, acceptedFrame{Kind: "accepted", RequestID: frame.RequestID,
				CallID: call.ID, ResponseID: call.ID, Sequence: 0, Reused: reused}); err != nil {
				return
			}
			startWatcher(frame.RequestID)
		case "cancel":
			var frame cancelFrame
			if json.Unmarshal(payload, &frame) != nil || strings.TrimSpace(frame.RequestID) == "" {
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", Error: "invalid_cancel"}, companyID) {
					return
				}
				continue
			}
			frame.RequestID = strings.ToLower(strings.TrimSpace(frame.RequestID))
			result, err := h.calls.CancelKonstantin(ctx, companyID, conversationID, frame.RequestID)
			switch {
			case errors.Is(err, domain.ErrNotFound):
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "request_not_found"}, companyID) {
					return
				}
			case errors.Is(err, calls.ErrInvalidCall):
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "invalid_cancel"}, companyID) {
					return
				}
			case err != nil:
				if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", RequestID: frame.RequestID, Error: "cancel_request_failed"}, companyID) {
					return
				}
			default:
				if result.Disposition == domain.CancelCompleted {
					if event, eventErr := h.calls.KonstantinEvent(ctx, frame.RequestID); eventErr == nil {
						if sendErr := sendEvent(event); sendErr != nil && !errors.Is(sendErr, context.Canceled) {
							log.Printf("consultant WebSocket event queue failed (company_id=%s error_type=%T)", companyID, sendErr)
						}
					}
					continue
				}
				if result.Disposition == domain.CancelTerminal {
					if err := sendTerminalCall(sendOrdered, result.Call, frame.RequestID); err != nil {
						return
					}
					continue
				}
				if result.Call.CancelSequence > 0 {
					if err := sendOrdered(frame.RequestID, result.Call.CancelSequence, false, cancelRequestedFrame{
						Kind: "cancel_requested", RequestID: frame.RequestID, CallID: result.Call.ID,
						ResponseID: result.Call.ID, Sequence: result.Call.CancelSequence,
					}); err != nil {
						return
					}
				}
				if result.Disposition == domain.CancelQueued {
					if err := sendTerminalCall(sendOrdered, result.Call, frame.RequestID); err != nil {
						return
					}
					continue
				}
				if result.Disposition == domain.CancelFailed && result.Call.StopFailureSeq > 0 {
					if err := sendOrdered(frame.RequestID, result.Call.StopFailureSeq, false, cancelFailedFrame{
						Kind: "cancel_failed", RequestID: frame.RequestID, CallID: result.Call.ID,
						ResponseID: result.Call.ID, Sequence: result.Call.StopFailureSeq, Error: "cancel_failed",
					}); err != nil {
						return
					}
				}
				startWatcher(frame.RequestID)
			}
		default:
			if !sendOrCancel(ctx, cancel, send, errorFrame{Kind: "error", Error: "expected_question_or_cancel_frame"}, companyID) {
				return
			}
		}
	}
}

type orderedStreamSender struct {
	mu       sync.Mutex
	last     map[string]int64
	sent     map[string]bool
	terminal map[string]bool
}

func (s *orderedStreamSender) send(ctx context.Context, send func(any) error, requestID string, sequence int64, terminal bool, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal[requestID] || (s.sent[requestID] && sequence <= s.last[requestID]) {
		return nil
	}
	if err := send(value); err != nil {
		return err
	}
	s.last[requestID], s.sent[requestID] = sequence, true
	if terminal {
		s.terminal[requestID] = true
	}
	return nil
}

func (h *consultantHandler) watchKonstantin(ctx context.Context, companyID, conversationID, requestID string,
	sendOrdered func(string, int64, bool, any) error, sendEvent func(domain.ConsultantEvent) error) {
	interval := h.calls.KonstantinEventPollInterval()
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var reportedStopFailure int64
	for {
		call, err := h.calls.KonstantinRequestStatus(ctx, requestID)
		if err != nil {
			if !errors.Is(err, domain.ErrNotFound) && ctx.Err() == nil {
				log.Printf("consultant call status lookup failed (company_id=%s error_type=%T)", companyID, err)
			}
			return
		}
		if call.ScopeID != companyID || call.ConversationID != conversationID {
			return
		}
		if call.StopFailedAt != nil && call.StopFailureSeq > reportedStopFailure {
			code := "cancel_failed"
			value := any(cancelFailedFrame{Kind: "cancel_failed", RequestID: requestID, CallID: call.ID,
				ResponseID: call.ID, Sequence: call.StopFailureSeq, Error: code})
			if call.StopReason == "timeout" {
				code = "execution_deadline_stop_failed"
				value = errorFrame{Kind: "error", RequestID: requestID, CallID: call.ID,
					ResponseID: call.ID, Sequence: call.StopFailureSeq, Error: code, Incomplete: true}
			}
			if err := sendOrdered(requestID, call.StopFailureSeq, false, value); err != nil {
				return
			}
			reportedStopFailure = call.StopFailureSeq
		}
		if call.Status.Terminal() {
			if err := sendTerminalCall(sendOrdered, call, requestID); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("consultant WebSocket terminal queue failed (company_id=%s error_type=%T)", companyID, err)
			}
			if call.Status == domain.CallCompleted {
				event, eventErr := h.calls.KonstantinEvent(ctx, requestID)
				if eventErr == nil {
					if err := sendEvent(event); err != nil && !errors.Is(err, context.Canceled) {
						log.Printf("consultant WebSocket event queue failed (company_id=%s error_type=%T)", companyID, err)
					}
				} else if ctx.Err() == nil {
					if err := sendOrdered(requestID, call.TerminalSequence, true, errorFrame{Kind: "error", RequestID: requestID,
						CallID: call.ID, ResponseID: call.ID, Sequence: call.TerminalSequence,
						Error: "assistant_result_unavailable", Terminal: true}); err != nil && !errors.Is(err, context.Canceled) {
						log.Printf("consultant WebSocket error queue failed (company_id=%s error_type=%T)", companyID, err)
					}
				}
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func sendTerminalCall(sendOrdered func(string, int64, bool, any) error, call domain.Call, requestID string) error {
	sequence := call.TerminalSequence
	switch call.Status {
	case domain.CallCancelled:
		if call.StopReason == "timeout" {
			return sendOrdered(requestID, sequence, true, errorFrame{Kind: "error", RequestID: requestID,
				CallID: call.ID, ResponseID: call.ID, Sequence: sequence,
				Error: "execution_deadline_exceeded", Incomplete: true, Terminal: true})
		}
		return sendOrdered(requestID, sequence, true, assistantCancelledFrame{Kind: "assistant_cancelled",
			RequestID: requestID, CallID: call.ID, ResponseID: call.ID, Sequence: sequence, Incomplete: true})
	case domain.CallFailed, domain.CallInterrupted:
		code := "assistant_response_failed"
		if call.Error != nil && stableStreamError(*call.Error) {
			code = *call.Error
		}
		return sendOrdered(requestID, sequence, true, errorFrame{Kind: "error", RequestID: requestID,
			CallID: call.ID, ResponseID: call.ID, Sequence: sequence, Error: code, Incomplete: true, Terminal: true})
	}
	return nil
}

func stableStreamError(code string) bool {
	switch code {
	case "assistant_response_failed", "assistant_cancelled", "execution_deadline_exceeded",
		"hermes_submission_failed", "cancel_failed", "stop_failed", "stop_unsupported",
		"company_profile_not_synced", "profile_revision_superseded":
		return true
	default:
		return false
	}
}

func sendOrCancel(ctx context.Context, cancel context.CancelFunc, send func(any) error, value any, companyID string) bool {
	if err := send(value); err != nil {
		if !errors.Is(err, context.Canceled) && ctx.Err() == nil {
			log.Printf("consultant WebSocket response queue failed (company_id=%s error_type=%T)", companyID, err)
		}
		cancel()
		return false
	}
	return true
}
