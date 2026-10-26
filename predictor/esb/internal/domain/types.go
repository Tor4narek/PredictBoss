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
	Type             AgentType
	ScopeField       string
	RequiredFields   []string
	MaxHistory       int
	ContextPolicy    ContextPolicy
	RuntimeURL       string
	RuntimeInstances []RuntimeInstance
	Instructions     string
}

// RuntimeState controls whether an instance accepts newly queued work.
type RuntimeState string

const (
	RuntimeAvailable   RuntimeState = "available"
	RuntimeDraining    RuntimeState = "draining"
	RuntimeUnavailable RuntimeState = "unavailable"
)

// RuntimeInstance is one stable Hermes runtime target in an agent's pool.
type RuntimeInstance struct {
	ID                string
	Agent             AgentType
	BaseURL           string
	State             RuntimeState
	MaxConcurrentRuns int
}

// ForRuntime resolves a call's pinned runtime into an agent-specific client view.
func (agent Agent) ForRuntime(instanceID string) (Agent, bool) {
	for _, instance := range agent.RuntimeInstances {
		if instance.ID == instanceID {
			agent.RuntimeURL = instance.BaseURL
			return agent, true
		}
	}
	if instanceID == "" && (agent.RuntimeURL != "" || len(agent.RuntimeInstances) == 0) {
		return agent, true
	}
	return Agent{}, false
}

// RetentionPolicy describes when an eligible data category may be removed.
// Immediate removal still respects dependency and retry-safety checks.
type RetentionPolicy struct {
	Never     bool
	Immediate bool
	Age       time.Duration
}

type AnalyticsRetention struct {
	ProcessedNewsText   RetentionPolicy
	UnprocessedNewsText RetentionPolicy
	NewsMetadata        RetentionPolicy
	NewsMarker          RetentionPolicy
	Facts               RetentionPolicy
	FactCompany         RetentionPolicy
	MetricObservations  RetentionPolicy
	MetricTotals        RetentionPolicy
	CompanyAnalyses     RetentionPolicy
}

// ContextPolicy controls model context compaction. MaxHistory remains the
// maximum number of recent PostgreSQL messages sent to Hermes.
type ContextPolicy struct {
	CompressAfterMessages int
	BatchMessages         int
	SummaryMaxBytes       int
	SummaryStorage        string
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

// Terminal reports whether a call has reached a state that cannot resume.
func (status CallStatus) Terminal() bool {
	switch status {
	case CallCompleted, CallFailed, CallCancelled, CallInterrupted:
		return true
	default:
		return false
	}
}

type Call struct {
	ID                string          `json:"call_id"`
	Agent             AgentType       `json:"agent"`
	ScopeID           string          `json:"scope_id"`
	ConversationID    string          `json:"conversation_id"`
	Input             json.RawMessage `json:"-"`
	Status            CallStatus      `json:"status"`
	HermesRunID       *string         `json:"hermes_run_id,omitempty"`
	RuntimeInstanceID string          `json:"-"`
	Output            *string         `json:"output,omitempty"`
	Error             *string         `json:"error,omitempty"`
	RetryCount        int             `json:"-"`
	DeadlineAt        time.Time       `json:"-"`
	LaunchStartedAt   *time.Time      `json:"-"`
	CancelRequestedAt *time.Time      `json:"-"`
	StopAttemptedAt   *time.Time      `json:"-"`
	StopFailedAt      *time.Time      `json:"-"`
	StopReason        string          `json:"-"`
	EventSequence     int64           `json:"-"`
	CancelSequence    int64           `json:"-"`
	StopFailureSeq    int64           `json:"-"`
	TerminalSequence  int64           `json:"-"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// CancelDisposition describes the result of an idempotent cancellation request.
type CancelDisposition string

const (
	CancelQueued       CancelDisposition = "queued_cancelled"
	CancelRequested    CancelDisposition = "cancel_requested"
	CancelAlreadyAsked CancelDisposition = "already_requested"
	CancelFailed       CancelDisposition = "cancel_failed"
	CancelCompleted    CancelDisposition = "completed"
	CancelTerminal     CancelDisposition = "terminal"
)

// CancelResult contains authoritative call state for the /stream adapter.
type CancelResult struct {
	Call        Call
	Disposition CancelDisposition
}

// MCPCallContext is a short-lived capability binding an agent to one persisted call.
// The company scope is derived from the call record, never from MCP arguments.
type MCPCallContext struct {
	Agent     AgentType
	CallID    string
	CompanyID string
	ExpiresAt time.Time
}

// MCPCallContextSigner signs and verifies per-call MCP capabilities.
type MCPCallContextSigner interface {
	IssueMCPCallContext(MCPCallContext) (string, error)
	VerifyMCPCallContext(string, AgentType) (MCPCallContext, error)
}

// MCPCompanyCallAuthorizer validates an active call capability and requested
// company against the authoritative call record.
type MCPCompanyCallAuthorizer interface {
	AuthorizeMCPCompany(context.Context, AgentType, string, string) (MCPCallContext, error)
	RecordMCPCallSources(context.Context, MCPCallContext, []ConsultantSource) error
}

type CompanyProfileSyncState struct {
	Revision int64
	Synced   bool
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ContextMessage struct {
	Message
	CallID    string    `json:"call_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ContextWindow struct {
	ConversationID          string
	Summary                 string
	SummaryThroughCallID    string
	SummaryThroughCreatedAt time.Time
	Recent                  []Message
	Pending                 []ContextMessage
	PendingThroughCallID    string
	PendingThroughCreatedAt time.Time
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
	TextHash     string
}

type Fact struct {
	ID           string   `json:"fact_id"`
	NewsID       string   `json:"news_id"`
	SourceID     string   `json:"source_id"`
	SourceURL    string   `json:"source_url"`
	MaterialKind string   `json:"material_kind,omitempty"`
	Title        string   `json:"title,omitempty"`
	PublishedAt  *string  `json:"published_at,omitempty"`
	Text         string   `json:"fact_text"`
	Category     string   `json:"category"`
	Entities     []string `json:"entities"`
	Evidence     string   `json:"evidence"`
	OccurredAt   *string  `json:"occurred_at"`
	Confidence   float64  `json:"confidence"`
}

// NewsReference is the persisted source metadata used to validate analytical
// writes against an existing news record.
type NewsReference struct {
	ID           string `json:"id"`
	SourceID     string `json:"source_id"`
	SourceURL    string `json:"source_url"`
	MaterialKind string `json:"material_kind"`
}

// FactReference binds an existing universal fact to its originating news.
type FactReference struct {
	ID        string `json:"id"`
	NewsID    string `json:"news_id"`
	SourceID  string `json:"source_id"`
	SourceURL string `json:"source_url"`
}

type FactQuery struct {
	CompanyID    string
	NewsID       string
	SourceID     string
	MaterialKind string
	Category     string
	Entity       string
	PeriodFrom   *time.Time
	PeriodTo     *time.Time
	BeforeFactID string
	Limit        int
}

type FactRecord struct {
	Fact
	Relevance       string  `json:"relevance,omitempty"`
	MatchConfidence float64 `json:"match_confidence,omitempty"`
}

type FactPage struct {
	Facts        []FactRecord `json:"facts"`
	HasMore      bool         `json:"hasMore"`
	NextBeforeID string       `json:"nextBeforeFactId,omitempty"`
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
	ID             string         `json:"metric_id,omitempty"`
	TypeCode       string         `json:"type_code"`
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
	Title         string   `json:"title,omitempty"`
	PublishedAt   *string  `json:"published_at,omitempty"`
	Value         *float64 `json:"value"`
	Probability   *float64 `json:"probability"`
	SourceFactIDs []string `json:"source_fact_ids"`
}

type MetricTotal struct {
	CompanyID        string       `json:"company_id"`
	MetricID         string       `json:"metric_id"`
	Unit             string       `json:"unit"`
	PeriodFrom       string       `json:"period_from"`
	PeriodTo         string       `json:"period_to"`
	TotalValue       *float64     `json:"total_value"`
	TotalProbability *float64     `json:"total_probability"`
	FactCount        uint32       `json:"fact_count"`
	ProbabilityCount uint32       `json:"probability_count"`
	Links            []MetricLink `json:"links"`
}

type MetricLink struct {
	NewsID     string `json:"news_id"`
	NewsURL    string `json:"news_url"`
	NewsTime   string `json:"news_time"`
	SourceKind string `json:"source_kind"`
}

type CompanyMetricTotal struct {
	MetricID         string       `json:"metric_id"`
	TypeCode         string       `json:"type_code"`
	Characteristic   string       `json:"characteristic"`
	Unit             string       `json:"unit"`
	TotalValue       *float64     `json:"total_value"`
	TotalProbability *float64     `json:"total_probability"`
	FactCount        uint32       `json:"fact_count"`
	ProbabilityCount uint32       `json:"probability_count"`
	Links            []MetricLink `json:"links"`
}

type CompanyMetricTotals struct {
	CompanyID  string               `json:"company_id"`
	PeriodFrom string               `json:"period_from"`
	PeriodTo   string               `json:"period_to"`
	Items      []CompanyMetricTotal `json:"items"`
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
	CompanyID string               `json:"company_id"`
	Facts     []CompanyFact        `json:"facts"`
	Metrics   []CompanyMetric      `json:"metrics"`
	Totals    []CompanyMetricTotal `json:"totals"`
	Analyses  []CompanyAnalysis    `json:"analyses"`
}

type ConsultantEvidence struct {
	SourceID     string  `json:"sourceId"`
	NewsID       string  `json:"newsId"`
	SourceURL    string  `json:"sourceUrl,omitempty"`
	MaterialKind string  `json:"materialKind"`
	Title        string  `json:"title,omitempty"`
	PublishedAt  *string `json:"publishedAt,omitempty"`
	OriginKind   string  `json:"originKind"`
}

// ConsultantSource captures one publication returned by a company-scoped MCP
// analytics read. The call-scoped registry uses it to enrich the final answer.
type ConsultantSource struct {
	SourceID     string  `json:"source_id"`
	NewsID       string  `json:"news_id"`
	SourceURL    string  `json:"source_url,omitempty"`
	MaterialKind string  `json:"material_kind"`
	Title        string  `json:"title,omitempty"`
	PublishedAt  *string `json:"published_at,omitempty"`
}

type ConsultantEvent struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Kind           string               `json:"kind"`
	RequestID      string               `json:"requestId"`
	CallID         string               `json:"callId,omitempty"`
	ResponseID     string               `json:"responseId,omitempty"`
	Sequence       int64                `json:"sequence,omitempty"`
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
	CompanyID    string               `json:"company_id"`
	Facts        []CompanyFact        `json:"facts"`
	Observations []MetricObservation  `json:"observations"`
	Totals       []CompanyMetricTotal `json:"totals"`
}

type CallRepository interface {
	Enqueue(context.Context, AgentType, string, json.RawMessage, string, time.Time) (Call, bool, error)
	Get(context.Context, string) (Call, error)
	GetForAgent(context.Context, string, AgentType) (Call, error)
	LoadHistory(context.Context, string, int) ([]Message, error)
	ClaimQueued(context.Context, int) ([]Call, error)
	SetRun(context.Context, string, string) error
	ListActive(context.Context) ([]Call, error)
	SetStatus(context.Context, string, CallStatus, string) error
	Complete(context.Context, string, string) error
	CompleteValera(context.Context, string, string, string, int64) (CompanyProfileSyncState, bool, error)
	RetryProfileSave(context.Context, string, string, int64, int) (bool, error)
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
	RecordKonstantinCallSources(context.Context, string, string, []ConsultantSource) error
	SourceRevision(context.Context, string) (int64, error)
	ProfileRevision(context.Context, string) (int64, error)
}

// CallExecutionRepository arbitrates execution deadlines and cancellation
// against queue workers and other ESB replicas using PostgreSQL state.
type CallExecutionRepository interface {
	BeginHermesLaunch(context.Context, string, time.Time) (bool, error)
	CancelKonstantin(context.Context, string, string, string, time.Time) (CancelResult, error)
	ClaimHermesStop(context.Context, string, time.Time) (Call, bool, error)
	MarkHermesStopFailed(context.Context, string, string, time.Time) (Call, error)
	RequestExpiredCalls(context.Context, time.Time, int) ([]Call, error)
}

// AgentRuntimeStop exposes Hermes' Run API stop operation to the application layer.
type AgentRuntimeStop interface {
	Stop(context.Context, Agent, string) error
}

// RuntimeCallRepository claims queued calls while atomically reserving a slot
// on one configured Hermes instance. The assignment is persisted before I/O.
type RuntimeCallRepository interface {
	ClaimQueuedForRuntimes(context.Context, int, []RuntimeInstance) ([]Call, error)
	ActiveRuntimeAssignments(context.Context) ([]Call, error)
}

// ContextRepository stores compacted summaries separately from the complete
// message history. Updates use a compare-and-swap cursor to avoid lost writes.
type ContextRepository interface {
	LoadContextWindow(context.Context, string, int, int, int) (ContextWindow, error)
	SaveContextSummary(context.Context, string, string, time.Time, string, time.Time, string) (bool, error)
}

type NewsFinalizer interface {
	FinalizeNews(context.Context, News) (string, error)
}

type NewsCallCompleter interface {
	CompleteNews(context.Context, string, string, string) error
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

// CleanupRepository exposes bounded, idempotent maintenance operations.
// WithCleanupLock serializes each pass across ESB replicas using PostgreSQL.
type CleanupRepository interface {
	WithCleanupLock(context.Context, func(context.Context) error) (bool, error)
	RecoverStaleSubmissions(context.Context, time.Time, int) (int, error)
	ListStaleActive(context.Context, time.Time, int) ([]Call, error)
	RetryStaleActive(context.Context, Call, int) (bool, error)
	PruneCallPayloads(context.Context, time.Time, int) (int, error)
	PruneCallRecords(context.Context, time.Time, int) (int, error)
	PruneMessages(context.Context, AgentType, time.Time, int) (int, error)
	PruneContextSummaries(context.Context, time.Time, int) (int, error)
	TerminalNewsIDs(context.Context, time.Time, int) ([]string, error)
}

type AnalyticsCleanupRepository interface {
	Prune(context.Context, AnalyticsRetention, time.Time, []string, int) (map[string]int, error)
}

type StaleCallReconciler interface {
	ReconcileStale(context.Context, time.Time, time.Time, int, int) (int, error)
}

type AnalyticsRepository interface {
	SaveNewsAndFacts(context.Context, News, []Fact) ([]string, error)
	NewsReferences(context.Context, []string) ([]NewsReference, error)
	FactReferences(context.Context, []string) ([]FactReference, error)
	CompanyFactIDs(context.Context, string, []string) ([]string, error)
	CompanyNewsIDs(context.Context, string, []string) ([]string, error)
	FinalizeNews(context.Context, News, string) error
	FactsByNews(context.Context, string, int) ([]Fact, error)
	QueryFacts(context.Context, FactQuery) ([]FactRecord, error)
	SaveFactCompanyLinks(context.Context, string, []FactCompanyLink) error
	CompanyFacts(context.Context, string, int) ([]CompanyFact, error)
	SaveMetricObservations(context.Context, string, []MetricObservation) error
	SaveMetricTotal(context.Context, MetricTotal) error
	SaveCompanyAnalysis(context.Context, CompanyAnalysis) error
	CalculateMetricTotal(context.Context, string, string, string) (MetricTotal, error)
	CompanyReport(context.Context, string, string, string) (CompanyReport, error)
	ConsultantContext(context.Context, string, int) (ConsultantContext, error)
}

type CompanyMetricSettingsRepository interface {
	WithCompanyProfileLock(context.Context, string, func(context.Context) error) error
	ReserveCompanyProfileRevision(context.Context, string, int64, string) error
	UpsertCompanyMetrics(context.Context, string, []CompanyMetric) error
	ReplaceCompanyMetricsAtRevision(context.Context, string, int64, string, []CompanyMetric) error
	MarkCompanyProfileSynced(context.Context, string, int64, string) error
	CompanyMetrics(context.Context, string, int) ([]CompanyMetric, error)
	MetricForID(context.Context, string) (CompanyMetric, string, error)
}

type ProfileRepository interface {
	Get(context.Context, string) (string, string, error)
	Save(context.Context, string, string) (string, error)
}
