package httpapi

import (
	"encoding/json"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

// These request and response models are used by the HTTP boundary and by the
// OpenAPI generator. Keep their JSON names aligned with the wire contract.
type getCallRequest struct {
	CallID         string `json:"call_id,omitempty" jsonschema:"Read a queued call by its ID."`
	CompanyID      string `json:"company_id,omitempty" jsonschema:"Company UUID; snake_case compatibility alias."`
	CompanyIDCamel string `json:"companyId,omitempty" jsonschema:"Company UUID."`
	PeriodFrom     string `json:"period_from,omitempty" jsonschema:"Inclusive start date for a Kesha report."`
	PeriodTo       string `json:"period_to,omitempty" jsonschema:"Inclusive end date for a Kesha report."`
	HistoryLimit   int    `json:"historyLimit,omitempty" jsonschema:"Number of Konstantin history messages; defaults to KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE and cannot exceed KONSTANTIN_HISTORY_MAX_PAGE_SIZE."`
	HistoryFrom    string `json:"historyFrom,omitempty" jsonschema:"RFC 3339 lower bound for Konstantin chat history."`
	HistoryTo      string `json:"historyTo,omitempty" jsonschema:"RFC 3339 upper bound for Konstantin chat history."`
	BeforeMessage  string `json:"beforeMessageId,omitempty" jsonschema:"Message cursor for loading an older history batch."`
}

type getFactsRequest struct {
	CompanyID       string `json:"companyId,omitempty" jsonschema:"Company UUID; snake_case compatibility alias is also accepted."`
	CompanyIDSnake  string `json:"company_id,omitempty" jsonschema:"Company UUID."`
	NewsID          string `json:"newsId,omitempty" jsonschema:"Stable news identifier."`
	NewsIDSnake     string `json:"news_id,omitempty" jsonschema:"Stable news identifier."`
	SourceID        string `json:"sourceId,omitempty" jsonschema:"Source UUID."`
	SourceIDSnake   string `json:"source_id,omitempty" jsonschema:"Source UUID."`
	MaterialKind    string `json:"materialKind,omitempty" jsonschema:"Material kind: external_news or alpha_analysis."`
	MaterialKindAlt string `json:"material_kind,omitempty" jsonschema:"Material kind: external_news or alpha_analysis."`
	Category        string `json:"category,omitempty" jsonschema:"Fact category."`
	Entity          string `json:"entity,omitempty" jsonschema:"Entity assigned to the fact."`
	PeriodFrom      string `json:"periodFrom,omitempty" jsonschema:"Inclusive RFC 3339 lower bound."`
	PeriodFromSnake string `json:"period_from,omitempty" jsonschema:"Inclusive RFC 3339 lower bound."`
	PeriodTo        string `json:"periodTo,omitempty" jsonschema:"Exclusive RFC 3339 upper bound."`
	PeriodToSnake   string `json:"period_to,omitempty" jsonschema:"Exclusive RFC 3339 upper bound."`
	Limit           int    `json:"limit,omitempty" jsonschema:"Page size; defaults to FACTS_DEFAULT_PAGE_SIZE and cannot exceed FACTS_MAX_PAGE_SIZE."`
	BeforeFactID    string `json:"beforeFactId,omitempty" jsonschema:"Fact ID cursor for the next page."`
	BeforeFactSnake string `json:"before_fact_id,omitempty" jsonschema:"Fact ID cursor for the next page."`
}

type valeraCallBody struct {
	CompanyID       string          `json:"companyId" jsonschema:"Company UUID."`
	ProfileRevision int64           `json:"profileRevision" jsonschema:"Monotonically increasing profile revision."`
	Company         json.RawMessage `json:"company" jsonschema:"Company JSON object used to build the Markdown profile and metric settings."`
}

type valeraCallEnvelope struct {
	Agent   string         `json:"agent,omitempty" jsonschema:"Compatibility field; agent query parameter remains authoritative."`
	Payload valeraCallBody `json:"payload" jsonschema:"Valera payload."`
}

type saveliyFact struct {
	FactID     string   `json:"fact_id" jsonschema:"Existing universal fact UUID."`
	FactText   string   `json:"fact_text" jsonschema:"Text of the existing fact."`
	Category   string   `json:"category" jsonschema:"Fact category."`
	Entities   []string `json:"entities" jsonschema:"Entities named in the fact."`
	Evidence   string   `json:"evidence" jsonschema:"Evidence excerpt from the source."`
	OccurredAt *string  `json:"occurred_at" jsonschema:"Event timestamp, or null if unknown."`
	Confidence float64  `json:"confidence" jsonschema:"Confidence from 0 to 1."`
}

type saveliyCallBody struct {
	CompanyID string        `json:"company_id" jsonschema:"Company UUID."`
	NewsID    string        `json:"news_id" jsonschema:"News identifier whose facts should be linked."`
	Facts     []saveliyFact `json:"facts,omitempty" jsonschema:"Optional facts already returned by Serezha."`
}

type keshaCallBody struct {
	CompanyID  string `json:"company_id" jsonschema:"Company UUID."`
	PeriodFrom string `json:"period_from,omitempty" jsonschema:"Optional inclusive start date for analysis."`
	PeriodTo   string `json:"period_to,omitempty" jsonschema:"Optional inclusive end date for analysis."`
}

type konstantinCallBody struct {
	CompanyID      string `json:"companyId,omitempty" jsonschema:"Company UUID."`
	CompanyIDSnake string `json:"company_id,omitempty" jsonschema:"Company UUID; snake_case compatibility alias."`
}

type acceptedCallResponse struct {
	CallID          string            `json:"call_id,omitempty" jsonschema:"Identifier of the queued asynchronous call."`
	Agent           domain.AgentType  `json:"agent,omitempty" jsonschema:"Selected agent type."`
	ScopeID         string            `json:"scope_id,omitempty" jsonschema:"Agent-specific conversation scope."`
	CallStatus      domain.CallStatus `json:"call_status,omitempty" jsonschema:"Current queue status for news and consultant calls."`
	Status          string            `json:"status,omitempty" jsonschema:"Acceptance state or queue status."`
	CompanyID       string            `json:"companyId,omitempty" jsonschema:"Company UUID for Valera calls."`
	ProfileRevision int64             `json:"profileRevision,omitempty" jsonschema:"Reserved profile revision for Valera calls."`
	Reused          bool              `json:"reused" jsonschema:"True when an idempotent earlier call was reused."`
}

type factsResponse struct {
	Data             string              `json:"data" jsonschema:"Response kind; always facts."`
	Facts            []domain.FactRecord `json:"facts" jsonschema:"Page of matching universal facts."`
	HasMore          bool                `json:"hasMore" jsonschema:"Whether another page is available."`
	NextBeforeFactID string              `json:"nextBeforeFactId,omitempty" jsonschema:"Cursor for the next page."`
}

type consultantReadyResponse struct {
	Status                     string                            `json:"status" jsonschema:"Always ready."`
	CompanyID                  string                            `json:"companyId" jsonschema:"Company UUID."`
	ConversationID             string                            `json:"conversationId" jsonschema:"Company-scoped chat ID."`
	AnalysisID                 string                            `json:"analysisId" jsonschema:"Current analysis ID."`
	MessageID                  string                            `json:"messageId" jsonschema:"Persisted analysis message ID."`
	Text                       string                            `json:"text" jsonschema:"Analysis text."`
	Evidence                   []domain.ConsultantEvidence       `json:"evidence" jsonschema:"Publication metadata verified by ESB from MCP results for this call."`
	CreatedAt                  time.Time                         `json:"createdAt" jsonschema:"Analysis creation time."`
	StreamURL                  string                            `json:"streamUrl" jsonschema:"WebSocket subscription URL."`
	History                    []domain.ConsultantHistoryMessage `json:"history" jsonschema:"Bounded page of this company's chat history."`
	HistoryHasMore             bool                              `json:"historyHasMore" jsonschema:"Whether an older history page is available."`
	HistoryFrom                string                            `json:"historyFrom" jsonschema:"Inclusive RFC 3339 history window start."`
	HistoryTo                  string                            `json:"historyTo" jsonschema:"Exclusive RFC 3339 history window end."`
	HistoryNextBeforeMessageID string                            `json:"historyNextBeforeMessageId,omitempty" jsonschema:"Cursor for the next older history page."`
}

type consultantProcessingResponse struct {
	Status                     string                            `json:"status" jsonschema:"Always processing."`
	CompanyID                  string                            `json:"companyId" jsonschema:"Company UUID."`
	ConversationID             string                            `json:"conversationId" jsonschema:"Company-scoped chat ID."`
	RequestID                  string                            `json:"requestId" jsonschema:"Pending analysis request ID."`
	StreamURL                  string                            `json:"streamUrl" jsonschema:"WebSocket subscription URL."`
	History                    []domain.ConsultantHistoryMessage `json:"history" jsonschema:"Bounded page of this company's chat history."`
	HistoryHasMore             bool                              `json:"historyHasMore" jsonschema:"Whether an older history page is available."`
	HistoryFrom                string                            `json:"historyFrom" jsonschema:"Inclusive RFC 3339 history window start."`
	HistoryTo                  string                            `json:"historyTo" jsonschema:"Exclusive RFC 3339 history window end."`
	HistoryNextBeforeMessageID string                            `json:"historyNextBeforeMessageId,omitempty" jsonschema:"Cursor for the next older history page."`
}

type errorResponse struct {
	Error string `json:"error" jsonschema:"Stable error code."`
}

type healthResponse struct {
	Status string `json:"status" jsonschema:"Current service state."`
}

type questionFrame struct {
	Kind           string `json:"kind" jsonschema:"Message kind; must be question."`
	RequestID      string `json:"requestId" jsonschema:"Idempotency ID for this user question."`
	CompanyID      string `json:"companyId,omitempty" jsonschema:"Optional company UUID; if supplied it must match the subscription."`
	ConversationID string `json:"conversationId,omitempty" jsonschema:"Optional chat ID; if supplied it must match the subscription."`
	AnalysisID     string `json:"analysisId" jsonschema:"Current ready analysis ID."`
	Text           string `json:"text" jsonschema:"User question."`
}

type acceptedFrame struct {
	Kind       string `json:"kind" jsonschema:"Message kind; always accepted."`
	RequestID  string `json:"requestId" jsonschema:"Question request ID."`
	CallID     string `json:"callId" jsonschema:"Queued Hermes call ID."`
	ResponseID string `json:"responseId" jsonschema:"Stable identifier for this one-shot answer; equal to callId."`
	Sequence   int64  `json:"sequence" jsonschema:"First lifecycle sequence number; always zero."`
	Reused     bool   `json:"reused" jsonschema:"True when an idempotent earlier call was reused."`
}

type cancelFrame struct {
	Kind      string `json:"kind" jsonschema:"Message kind; must be cancel."`
	RequestID string `json:"requestId" jsonschema:"Request ID of the Konstantin question to cancel."`
}

type cancelRequestedFrame struct {
	Kind       string `json:"kind" jsonschema:"Message kind; always cancel_requested."`
	RequestID  string `json:"requestId" jsonschema:"Cancelled question request ID."`
	CallID     string `json:"callId" jsonschema:"ESB call ID."`
	ResponseID string `json:"responseId" jsonschema:"Stable answer identifier; equal to callId."`
	Sequence   int64  `json:"sequence" jsonschema:"Monotonic sequence number for this call."`
}

type cancelFailedFrame struct {
	Kind       string `json:"kind" jsonschema:"Message kind; always cancel_failed."`
	RequestID  string `json:"requestId" jsonschema:"Question request ID."`
	CallID     string `json:"callId" jsonschema:"ESB call ID."`
	ResponseID string `json:"responseId" jsonschema:"Stable answer identifier; equal to callId."`
	Sequence   int64  `json:"sequence" jsonschema:"Monotonic sequence number for this call."`
	Error      string `json:"error" jsonschema:"Stable error code; never includes Hermes details."`
}

type assistantCancelledFrame struct {
	Kind       string `json:"kind" jsonschema:"Message kind; always assistant_cancelled."`
	RequestID  string `json:"requestId" jsonschema:"Question request ID."`
	CallID     string `json:"callId" jsonschema:"ESB call ID."`
	ResponseID string `json:"responseId" jsonschema:"Stable answer identifier; equal to callId."`
	Sequence   int64  `json:"sequence" jsonschema:"Terminal sequence number for this call."`
	Incomplete bool   `json:"incomplete" jsonschema:"True because no final assistant answer was saved."`
}

type errorFrame struct {
	Kind       string `json:"kind" jsonschema:"Message kind; always error."`
	RequestID  string `json:"requestId,omitempty" jsonschema:"Related request ID when available."`
	CallID     string `json:"callId,omitempty" jsonschema:"ESB call ID when a call exists."`
	ResponseID string `json:"responseId,omitempty" jsonschema:"Stable answer identifier when a call exists."`
	Sequence   int64  `json:"sequence,omitempty" jsonschema:"Monotonic sequence number when a call exists."`
	Error      string `json:"error" jsonschema:"Stable machine-readable error code."`
	Incomplete bool   `json:"incomplete,omitempty" jsonschema:"True when no complete answer was saved."`
	Terminal   bool   `json:"terminal,omitempty" jsonschema:"True when no later answer event will follow on this connection."`
}
