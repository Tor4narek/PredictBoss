package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type AgentType string

const (
	AgentSerezha    AgentType = "serezha"
	AgentSaveliy    AgentType = "saveliy"
	AgentValera     AgentType = "valera"
	AgentKesha      AgentType = "kesha"
	AgentKonstantin AgentType = "konstantin"
)

type Agent struct {
	Type           AgentType
	ScopeField     string
	RequiredFields []string
	MaxHistory     int
	RuntimeURL     string
	Instructions   string
}

type CallStatus string

const (
	CallQueued      CallStatus = "queued"
	CallSubmitting  CallStatus = "submitting"
	CallStarted     CallStatus = "started"
	CallRunning     CallStatus = "running"
	CallProcessing  CallStatus = "processing"
	CallCompleted   CallStatus = "completed"
	CallFailed      CallStatus = "failed"
	CallCancelled   CallStatus = "cancelled"
	CallInterrupted CallStatus = "interrupted"
)

type Call struct {
	ID             string          `json:"call_id"`
	Agent          AgentType       `json:"agent"`
	ScopeID        string          `json:"scope_id"`
	ConversationID string          `json:"conversation_id"`
	Input          json.RawMessage `json:"-"`
	Status         CallStatus      `json:"status"`
	HermesRunID    *string         `json:"hermes_run_id,omitempty"`
	Output         *string         `json:"output,omitempty"`
	Error          *string         `json:"error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type News struct {
	ID           string
	SourceID     string
	SourceURL    string
	CompanyID    string
	MaterialKind string
	MediaType    string
	Title        string
	PublishedAt  *string
	Text         string
}

type Fact struct {
	ID           string   `json:"fact_id"`
	NewsID       string   `json:"news_id"`
	SourceID     string   `json:"source_id"`
	SourceURL    string   `json:"source_url"`
	MaterialKind string   `json:"material_kind,omitempty"`
	Text         string   `json:"fact_text"`
	Category     string   `json:"category"`
	Entities     []string `json:"entities"`
	Evidence     string   `json:"evidence"`
	OccurredAt   *string  `json:"occurred_at"`
	Confidence   float64  `json:"confidence"`
}

type FactCompanyLink struct {
	FactID     string  `json:"fact_id"`
	Relevance  string  `json:"relevance"`
	Confidence float64 `json:"confidence"`
}

type CompanyFact struct {
	Fact
	Relevance       string  `json:"relevance"`
	MatchConfidence float64 `json:"match_confidence"`
}

type MetricType string

const (
	MetricNumeric    MetricType = "numeric"
	MetricCurrency   MetricType = "currency"
	MetricPercentage MetricType = "percentage"
	MetricCount      MetricType = "count"
	MetricRatio      MetricType = "ratio"
	MetricScore      MetricType = "score"
	MetricText       MetricType = "text"
)

type CompanyMetric struct {
	ID             string         `json:"metric_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Unit           string         `json:"unit"`
	Type           MetricType     `json:"metric_type"`
	Characteristic string         `json:"characteristic"`
	Parameters     map[string]any `json:"parameters,omitempty"`
}

func (t MetricType) Valid() bool {
	switch t {
	case MetricNumeric, MetricCurrency, MetricPercentage, MetricCount, MetricRatio, MetricScore, MetricText:
		return true
	default:
		return false
	}
}

type MetricObservation struct {
	ID            string   `json:"observation_id,omitempty"`
	CompanyID     string   `json:"company_id"`
	MetricID      string   `json:"metric_id"`
	NewsID        string   `json:"news_id"`
	SourceID      string   `json:"source_id,omitempty"`
	SourceURL     string   `json:"source_url,omitempty"`
	MaterialKind  string   `json:"material_kind,omitempty"`
	Value         *float64 `json:"value"`
	Probability   float64  `json:"probability"`
	PeriodFrom    string   `json:"period_from"`
	PeriodTo      string   `json:"period_to"`
	SourceFactIDs []string `json:"source_fact_ids"`
}

type MetricTotal struct {
	CompanyID        string  `json:"company_id"`
	MetricID         string  `json:"metric_id"`
	PeriodFrom       string  `json:"period_from"`
	PeriodTo         string  `json:"period_to"`
	Total            float64 `json:"total"`
	ObservationCount uint32  `json:"observation_count"`
	CalculatedAt     string  `json:"calculated_at,omitempty"`
}

type CompanyAnalysis struct {
	CompanyID   string   `json:"company_id"`
	PeriodFrom  string   `json:"period_from"`
	PeriodTo    string   `json:"period_to"`
	Probability float64  `json:"probability"`
	MetricIDs   []string `json:"metric_ids"`
	Text        string   `json:"text"`
}

type CompanyReport struct {
	CompanyID string            `json:"company_id"`
	Facts     []CompanyFact     `json:"facts"`
	Metrics   []CompanyMetric   `json:"metrics"`
	Totals    []MetricTotal     `json:"totals"`
	Analyses  []CompanyAnalysis `json:"analyses"`
}

type ConsultantEvidence struct {
	SourceID   string `json:"sourceId"`
	OriginKind string `json:"originKind"`
}

type ConsultantEvent struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Kind           string               `json:"kind"`
	RequestID      string               `json:"requestId"`
	CompanyID      string               `json:"companyId"`
	ConversationID string               `json:"conversationId"`
	AnalysisID     string               `json:"analysisId"`
	MessageID      string               `json:"messageId"`
	Text           string               `json:"text"`
	Evidence       []ConsultantEvidence `json:"evidence"`
	CreatedAt      time.Time            `json:"createdAt"`
}

type ConsultantHistoryMessage struct {
	MessageID      string               `json:"messageId"`
	Role           string               `json:"role"`
	Kind           string               `json:"kind"`
	RequestID      string               `json:"requestId"`
	CompanyID      string               `json:"companyId"`
	ConversationID string               `json:"conversationId"`
	AnalysisID     string               `json:"analysisId"`
	Text           string               `json:"text"`
	Evidence       []ConsultantEvidence `json:"evidence"`
	CreatedAt      time.Time            `json:"createdAt"`
}

type ConsultantHistoryPage struct {
	Messages            []ConsultantHistoryMessage `json:"messages"`
	HasMore             bool                       `json:"hasMore"`
	NextBeforeMessageID string                     `json:"nextBeforeMessageId,omitempty"`
	From                time.Time                  `json:"historyFrom"`
	To                  time.Time                  `json:"historyTo"`
}

type ConsultantRequest struct {
	Kind           string `json:"kind"`
	RequestID      string `json:"requestId"`
	CompanyID      string `json:"companyId"`
	ConversationID string `json:"conversationId,omitempty"`
	AnalysisID     string `json:"analysisId,omitempty"`
	Text           string `json:"text,omitempty"`
}

type CompanySource struct {
	SourceID     string
	CompanyID    string
	NewsID       string
	MaterialKind string
	MediaType    string
	SourceURL    string
	Title        string
	PublishedAt  *string
}

type ConsultantContext struct {
	CompanyID    string              `json:"company_id"`
	Facts        []CompanyFact       `json:"facts"`
	Observations []MetricObservation `json:"observations"`
	Totals       []MetricTotal       `json:"totals"`
}

type CallRepository interface {
	Enqueue(context.Context, AgentType, string, json.RawMessage, string) (Call, bool, error)
	Get(context.Context, string) (Call, error)
	GetForAgent(context.Context, string, AgentType) (Call, error)
	LoadHistory(context.Context, string, int) ([]Message, error)
	ClaimQueued(context.Context, int) ([]Call, error)
	SetRun(context.Context, string, string) error
	ListActive(context.Context) ([]Call, error)
	SetStatus(context.Context, string, CallStatus, string) error
	Complete(context.Context, string, string) error
	Fail(context.Context, string, string) error
	Recover(context.Context) error
	ConversationID(context.Context, AgentType, string) (string, error)
	FreshAnalysis(context.Context, string) (ConsultantEvent, bool, error)
	Event(context.Context, string) (ConsultantEvent, error)
	RequestStatus(context.Context, string) (Call, error)
	EventsAfter(context.Context, string, string, string, int) ([]ConsultantEvent, error)
	KonstantinHistory(context.Context, string, time.Time, time.Time, string, int) (ConsultantHistoryPage, error)
	AnalysisReady(context.Context, string, string) (bool, error)
	RecordSource(context.Context, CompanySource) (bool, error)
	SourceRevision(context.Context, string) (int64, error)
	ProfileRevision(context.Context, string) (int64, error)
}

type AgentRuntime interface {
	Start(context.Context, Agent, string, []Message, string) (string, error)
	Status(context.Context, Agent, string) (RunState, error)
}

type RunState struct {
	Status string
	Output string
	Error  string
}

type AnalyticsRepository interface {
	SaveNewsAndFacts(context.Context, News, []Fact) ([]string, error)
	FactsByNews(context.Context, string, int) ([]Fact, error)
	SaveFactCompanyLinks(context.Context, string, []FactCompanyLink) error
	CompanyFacts(context.Context, string, int) ([]CompanyFact, error)
	SaveMetricObservations(context.Context, string, []MetricObservation) error
	SaveCompanyAnalysis(context.Context, CompanyAnalysis) error
	CalculateMetricTotal(context.Context, string, string, string, string) (MetricTotal, error)
	CompanyReport(context.Context, string, string, string) (CompanyReport, error)
	ConsultantContext(context.Context, string, int) (ConsultantContext, error)
}

type CompanyMetricSettingsRepository interface {
	ReserveCompanyProfileRevision(context.Context, string, int64, string) error
	UpsertCompanyMetrics(context.Context, string, []CompanyMetric) error
	UpsertCompanyMetricsAtRevision(context.Context, string, int64, string, []CompanyMetric) error
	MarkCompanyProfileSynced(context.Context, string, int64, string) error
	CompanyMetrics(context.Context, string, int) ([]CompanyMetric, error)
}

type ProfileRepository interface {
	Get(context.Context, string) (string, string, error)
	Save(context.Context, string, string) (string, error)
}
