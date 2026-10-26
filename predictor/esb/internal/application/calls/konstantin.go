package calls

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

var uuidInputPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var ErrAnalysisNotReady = errors.New("analysis_not_ready")
var ErrInvalidHistoryRequest = errors.New("invalid_history_request")

type konstantinAnalysisInput struct {
	CompanyID       string `json:"company_id"`
	RequestID       string `json:"request_id"`
	AnalysisID      string `json:"analysis_id"`
	RequestKind     string `json:"request_kind"`
	ProfileRevision int64  `json:"profile_revision"`
	SourceRevision  int64  `json:"source_revision"`
}

type konstantinQuestionInput struct {
	CompanyID       string `json:"company_id"`
	ConversationID  string `json:"conversation_id"`
	RequestID       string `json:"request_id"`
	AnalysisID      string `json:"analysis_id"`
	Text            string `json:"text"`
	RequestKind     string `json:"request_kind"`
	ProfileRevision int64  `json:"profile_revision"`
	SourceRevision  int64  `json:"source_revision"`
}

func (s *Service) StartKonstantinAnalysis(ctx context.Context, companyID string) (domain.ConsultantEvent, domain.Call, bool, error) {
	companyID = strings.TrimSpace(companyID)
	if !uuidInputPattern.MatchString(companyID) {
		return domain.ConsultantEvent{}, domain.Call{}, false, ErrInvalidCall
	}
	companyID = strings.ToLower(companyID)
	profileRevision, err := s.repository.ProfileRevision(ctx, companyID)
	if err != nil {
		return domain.ConsultantEvent{}, domain.Call{}, false, err
	}
	sourceRevision, err := s.repository.SourceRevision(ctx, companyID)
	if err != nil {
		return domain.ConsultantEvent{}, domain.Call{}, false, err
	}
	if event, ready, err := s.repository.FreshAnalysis(ctx, companyID); err != nil || ready {
		return event, domain.Call{}, ready, err
	}
	call, _, err := s.queueKonstantinAnalysis(ctx, companyID, profileRevision, sourceRevision)
	return domain.ConsultantEvent{}, call, false, err
}

// QueueKonstantinAnalysis enqueues a company analysis, reusing the same call for
// the same profile and source revisions. Questions are submitted over /stream.
func (s *Service) QueueKonstantinAnalysis(ctx context.Context, companyID string) (domain.Call, bool, error) {
	companyID = strings.ToLower(strings.TrimSpace(companyID))
	if !uuidInputPattern.MatchString(companyID) {
		return domain.Call{}, false, ErrInvalidCall
	}
	profileRevision, err := s.repository.ProfileRevision(ctx, companyID)
	if err != nil {
		return domain.Call{}, false, err
	}
	sourceRevision, err := s.repository.SourceRevision(ctx, companyID)
	if err != nil {
		return domain.Call{}, false, err
	}
	return s.queueKonstantinAnalysis(ctx, companyID, profileRevision, sourceRevision)
}

func (s *Service) queueKonstantinAnalysis(ctx context.Context, companyID string, profileRevision, sourceRevision int64) (domain.Call, bool, error) {
	key := fmt.Sprintf("konstantin:analysis:%s:p%d:s%d", companyID, profileRevision, sourceRevision)
	requestID := deterministicUUID(key)
	input, err := json.Marshal(konstantinAnalysisInput{CompanyID: companyID, RequestID: requestID, AnalysisID: requestID,
		RequestKind: "analysis", ProfileRevision: profileRevision, SourceRevision: sourceRevision})
	if err != nil {
		return domain.Call{}, false, err
	}
	return s.Create(ctx, string(domain.AgentKonstantin), input, key)
}

func (s *Service) AskKonstantin(ctx context.Context, request domain.ConsultantRequest) (domain.Call, bool, error) {
	request.CompanyID = strings.TrimSpace(request.CompanyID)
	request.ConversationID = strings.TrimSpace(request.ConversationID)
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.AnalysisID = strings.TrimSpace(request.AnalysisID)
	request.Text = strings.TrimSpace(request.Text)
	if !uuidInputPattern.MatchString(request.CompanyID) || !uuidInputPattern.MatchString(request.RequestID) ||
		!uuidInputPattern.MatchString(request.AnalysisID) || !uuidInputPattern.MatchString(request.ConversationID) || request.Text == "" || int64(len(request.Text)) > s.settings.MaxQuestionBytes {
		return domain.Call{}, false, ErrInvalidCall
	}
	request.CompanyID = strings.ToLower(request.CompanyID)
	request.RequestID = strings.ToLower(request.RequestID)
	request.AnalysisID = strings.ToLower(request.AnalysisID)
	request.ConversationID = strings.ToLower(request.ConversationID)
	if call, err := s.repository.RequestStatus(ctx, request.RequestID); err == nil {
		var previous konstantinQuestionInput
		if json.Unmarshal(call.Input, &previous) != nil || previous.CompanyID != request.CompanyID ||
			previous.ConversationID != request.ConversationID || previous.AnalysisID != request.AnalysisID || previous.Text != request.Text {
			return domain.Call{}, false, domain.ErrConflict
		}
		return call, true, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Call{}, false, err
	}
	ready, err := s.repository.AnalysisReady(ctx, request.CompanyID, request.AnalysisID)
	if err != nil {
		return domain.Call{}, false, err
	}
	if !ready {
		return domain.Call{}, false, ErrAnalysisNotReady
	}
	conversationID, err := s.repository.ConversationID(ctx, domain.AgentKonstantin, request.CompanyID)
	if err != nil {
		return domain.Call{}, false, err
	}
	if conversationID != request.ConversationID {
		return domain.Call{}, false, ErrAnalysisNotReady
	}
	profileRevision, err := s.repository.ProfileRevision(ctx, request.CompanyID)
	if err != nil {
		return domain.Call{}, false, err
	}
	sourceRevision, err := s.repository.SourceRevision(ctx, request.CompanyID)
	if err != nil {
		return domain.Call{}, false, err
	}
	input, err := json.Marshal(konstantinQuestionInput{CompanyID: request.CompanyID, ConversationID: conversationID,
		RequestID: request.RequestID, AnalysisID: request.AnalysisID, Text: request.Text, RequestKind: "question",
		ProfileRevision: profileRevision, SourceRevision: sourceRevision})
	if err != nil {
		return domain.Call{}, false, err
	}
	return s.Create(ctx, string(domain.AgentKonstantin), input, "konstantin:question:"+request.RequestID)
}

func (s *Service) KonstantinEvent(ctx context.Context, requestID string) (domain.ConsultantEvent, error) {
	return s.repository.Event(ctx, requestID)
}

func (s *Service) KonstantinConversation(ctx context.Context, companyID string) (string, error) {
	if !uuidInputPattern.MatchString(strings.TrimSpace(companyID)) {
		return "", ErrInvalidCall
	}
	companyID = strings.ToLower(companyID)
	return s.repository.ConversationID(ctx, domain.AgentKonstantin, companyID)
}

func (s *Service) KonstantinEventsAfter(ctx context.Context, companyID, conversationID, afterMessageID string, limit int) ([]domain.ConsultantEvent, error) {
	if !uuidInputPattern.MatchString(companyID) || !uuidInputPattern.MatchString(conversationID) {
		return nil, ErrInvalidCall
	}
	companyID, conversationID, afterMessageID = strings.ToLower(companyID), strings.ToLower(conversationID), strings.ToLower(afterMessageID)
	return s.repository.EventsAfter(ctx, companyID, conversationID, afterMessageID, limit)
}

func (s *Service) KonstantinHistory(ctx context.Context, companyID, fromText, toText, beforeMessageID string, limit int) (domain.ConsultantHistoryPage, error) {
	companyID = strings.ToLower(strings.TrimSpace(companyID))
	fromText = strings.TrimSpace(fromText)
	toText = strings.TrimSpace(toText)
	beforeMessageID = strings.ToLower(strings.TrimSpace(beforeMessageID))
	if !uuidInputPattern.MatchString(companyID) || (beforeMessageID != "" && !uuidInputPattern.MatchString(beforeMessageID)) || limit < 0 || limit > s.settings.KonstantinHistoryMaxPageSize {
		return domain.ConsultantHistoryPage{}, ErrInvalidHistoryRequest
	}
	if limit == 0 {
		limit = s.settings.KonstantinHistoryDefaultPageSize
	}
	to := time.Now().UTC()
	if toText != "" {
		parsed, err := time.Parse(time.RFC3339Nano, toText)
		if err != nil {
			return domain.ConsultantHistoryPage{}, ErrInvalidHistoryRequest
		}
		to = parsed.UTC()
	}
	from := to.Add(-s.settings.KonstantinDefaultWindow)
	if fromText != "" {
		parsed, err := time.Parse(time.RFC3339Nano, fromText)
		if err != nil {
			return domain.ConsultantHistoryPage{}, ErrInvalidHistoryRequest
		}
		from = parsed.UTC()
	}
	if !from.Before(to) || to.Sub(from) > s.settings.KonstantinMaxWindow {
		return domain.ConsultantHistoryPage{}, ErrInvalidHistoryRequest
	}
	page, err := s.repository.KonstantinHistory(ctx, companyID, from, to, beforeMessageID, limit)
	if err != nil {
		return domain.ConsultantHistoryPage{}, err
	}
	page.From, page.To = from, to
	return page, nil
}

func (s *Service) WaitKonstantinEvent(ctx context.Context, requestID string) (domain.ConsultantEvent, error) {
	ticker := time.NewTicker(s.settings.KonstantinEventPollInterval)
	defer ticker.Stop()
	for {
		event, err := s.repository.Event(ctx, requestID)
		if err == nil {
			return event, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return domain.ConsultantEvent{}, err
		}
		call, statusErr := s.repository.RequestStatus(ctx, requestID)
		if statusErr == nil && (call.Status == domain.CallFailed || call.Status == domain.CallCancelled || call.Status == domain.CallInterrupted) {
			message := "agent_call_" + string(call.Status)
			if call.Error != nil && *call.Error != "" {
				message = *call.Error
			}
			return domain.ConsultantEvent{}, errors.New(message)
		}
		if statusErr != nil && !errors.Is(statusErr, domain.ErrNotFound) {
			return domain.ConsultantEvent{}, statusErr
		}
		select {
		case <-ctx.Done():
			return domain.ConsultantEvent{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func deterministicUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	data := digest[:16]
	data[6] = data[6]&0x0f | 0x50
	data[8] = data[8]&0x3f | 0x80
	value := hex.EncodeToString(data)
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}
