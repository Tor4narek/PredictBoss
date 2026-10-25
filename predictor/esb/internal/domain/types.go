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
	AgentSerezha AgentType = "serezha"
	AgentSaveliy AgentType = "saveliy"
	AgentValera  AgentType = "valera"
	AgentKesha   AgentType = "kesha"
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
	ID          string          `json:"call_id"`
	Agent       AgentType       `json:"agent"`
	ScopeID     string          `json:"scope_id"`
	Input       json.RawMessage `json:"-"`
	Status      CallStatus      `json:"status"`
	HermesRunID *string         `json:"hermes_run_id,omitempty"`
	Output      *string         `json:"output,omitempty"`
	Error       *string         `json:"error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type News struct {
	ID        string
	SourceID  string
	SourceURL string
	Text      string
}

type Fact struct {
	ID         string   `json:"fact_id"`
	NewsID     string   `json:"news_id"`
	SourceID   string   `json:"source_id"`
	SourceURL  string   `json:"source_url"`
	Text       string   `json:"fact_text"`
	Category   string   `json:"category"`
	Entities   []string `json:"entities"`
	Evidence   string   `json:"evidence"`
	OccurredAt *string  `json:"occurred_at"`
	Confidence float64  `json:"confidence"`
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
}

type CompanyMetricSettingsRepository interface {
	UpsertCompanyMetrics(context.Context, string, []CompanyMetric) error
	CompanyMetrics(context.Context, string, int) ([]CompanyMetric, error)
}

type ProfileRepository interface {
	Get(context.Context, string) (string, string, error)
	Save(context.Context, string, string) (string, error)
}
