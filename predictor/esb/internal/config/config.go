// Package config loads and validates the ESB process configuration from the
// environment. Agent roles and Hermes runtime configuration remain in agents/.
package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type LookupEnv func(string) (string, bool)

type Definition struct {
	Name        string
	Type        string
	Default     string
	Range       string
	Unit        string
	Purpose     string
	Category    string
	Secret      bool
	defaultRaw  string
	minInt      int64
	maxInt      int64
	minDuration time.Duration
	maxDuration time.Duration
	required    bool
	allowNever  bool
}

type Connections struct {
	HTTPAddr       string
	DatabaseURL    string
	ESBAPIKey      string
	AgentMCPTokens map[domain.AgentType]string
	HermesAPIKey   string
	ClickHouseURL  string
	CHDatabase     string
	CHUser         string
	CHPassword     string
	S3Bucket       string
	S3Region       string
	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3PathStyle    bool
	S3CreateBucket bool
	AgentsDir      string
}

type HTTP struct {
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxJSONBodyBytes  int64
}

type Queue struct {
	HermesPollInterval       time.Duration
	DispatchBatchSize        int
	ContextCompactionTimeout time.Duration
	ValeraProfileSyncRetries int
}

type Cleanup struct {
	Enabled                 bool
	StatusCheckInterval     time.Duration
	RetentionCheckInterval  time.Duration
	BatchSize               int
	QueuedStaleAfter        time.Duration
	RunningStaleAfter       time.Duration
	ActiveMaxRetries        int
	CallPayloadRetention    domain.RetentionPolicy
	CallRecordRetention     domain.RetentionPolicy
	ContextSummaryRetention domain.RetentionPolicy
	MessagesRetention       map[domain.AgentType]domain.RetentionPolicy
	Analytics               domain.AnalyticsRetention
}

type Hermes struct {
	RequestTimeout time.Duration
	HealthTimeout  time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	MaxResponse    int64
}

type ClickHouse struct {
	RequestTimeout time.Duration
	MaxResponse    int64
}

type WebSocket struct {
	MaxFrameBytes     int64
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	OutgoingQueue     int
	ReplayLimit       int
	EventPollInterval time.Duration
}

type Pagination struct {
	FactsDefaultPageSize             int
	FactsMaxPageSize                 int
	FactsMaxPeriodDays               int
	CompanyItemsDefaultPageSize      int
	CompanyItemsMaxPageSize          int
	ConsultantDefaultPageSize        int
	ConsultantMaxPageSize            int
	KonstantinHistoryDefaultPageSize int
	KonstantinHistoryMaxPageSize     int
	KonstantinDefaultWindowDays      int
	KonstantinMaxWindowDays          int
	CompanyReportDefaultWindowDays   int
	CompanyReportFactsLimit          int
	CompanyReportAnalysisLimit       int
}

type PayloadLimits struct {
	MaxNewsTextBytes            int64
	MaxQuestionBytes            int64
	MaxProfileBytes             int64
	MaxFactsPerNews             int
	MaxFactCompanyLinks         int
	MaxMetricObservations       int
	MaxCompanyMetricsPerRequest int
}

type Config struct {
	Connections   Connections
	HTTP          HTTP
	Queue         Queue
	Hermes        Hermes
	ClickHouse    ClickHouse
	WebSocket     WebSocket
	Pagination    Pagination
	Payload       PayloadLimits
	Cleanup       Cleanup
	Agents        []domain.Agent
	AgentMCPTools map[domain.AgentType][]string
}

const (
	defaultFactsPageSize           = 100
	defaultFactsMaxPageSize        = 100
	defaultCompanyPageSize         = 100
	defaultCompanyMaxPageSize      = 500
	defaultConsultantPageSize      = 100
	defaultConsultantMaxPageSize   = 500
	defaultKonstantinHistoryPage   = 100
	defaultKonstantinHistoryMax    = 100
	defaultKonstantinWindowDays    = 30
	defaultKonstantinMaxWindowDays = 90
	maxNewsRequestMetadataBytes    = int64(4096)
	maxAgentMCPTokenBytes          = 256
)

var definitions = buildDefinitions()

func buildDefinitions() []Definition {
	settings := []Definition{
		stringDef("HTTP_ADDR", ":8081", "ESB listen address inside its network namespace.", "runtime", false),
		stringDef("DATABASE_URL", "", "PostgreSQL connection URL used only by ESB.", "secret", true),
		stringDef("ESB_API_KEY", "", "Bearer key for Domain HTTP and WebSocket integration; it does not authorize MCP tools.", "secret", true),
		stringDef("HERMES_API_KEY", "", "Bearer key ESB uses for Hermes runtime calls.", "secret", true),
		stringDef("HERMES_MODEL_PROVIDER", "custom", "Shared Hermes model-provider identifier for all agents.", "hermes", false),
		stringDef("HERMES_SEREZHA_URL", "", "Legacy single Serezha Hermes URL; use HERMES_SEREZHA_INSTANCES for a pool.", "runtime", false),
		stringDef("HERMES_SAVELIY_URL", "", "Legacy single Saveliy Hermes URL; use HERMES_SAVELIY_INSTANCES for a pool.", "runtime", false),
		stringDef("HERMES_VALERA_URL", "", "Legacy single Valera Hermes URL; use HERMES_VALERA_INSTANCES for a pool.", "runtime", false),
		stringDef("HERMES_KESHA_URL", "", "Legacy single Kesha Hermes URL; use HERMES_KESHA_INSTANCES for a pool.", "runtime", false),
		stringDef("HERMES_KONSTANTIN_URL", "", "Legacy single Konstantin Hermes URL; use HERMES_KONSTANTIN_INSTANCES for a pool.", "runtime", false),
		stringDef("HERMES_SEREZHA_INSTANCES", "", "Named Serezha runtime pool as id=state@url entries separated by semicolons.", "runtime", false),
		stringDef("HERMES_SAVELIY_INSTANCES", "", "Named Saveliy runtime pool as id=state@url entries separated by semicolons.", "runtime", false),
		stringDef("HERMES_VALERA_INSTANCES", "", "Named Valera runtime pool as id=state@url entries separated by semicolons.", "runtime", false),
		stringDef("HERMES_KESHA_INSTANCES", "", "Named Kesha runtime pool as id=state@url entries separated by semicolons.", "runtime", false),
		stringDef("HERMES_KONSTANTIN_INSTANCES", "", "Named Konstantin runtime pool as id=state@url entries separated by semicolons.", "runtime", false),
		durationDef("HERMES_POLL_INTERVAL", 2*time.Second, time.Millisecond, 10*time.Minute, "interval", "Queue dispatch and Hermes status polling interval.", "runtime"),
		intDef("QUEUE_DISPATCH_BATCH_SIZE", 100, 1, 10000, "calls", "Maximum queued calls claimed in one dispatch cycle; each claim can start one worker.", "runtime"),
		durationDef("CONTEXT_COMPACTION_TIMEOUT", 30*time.Second, time.Millisecond, time.Hour, "duration", "Maximum time spent requesting a compressed agent-context summary from Hermes.", "runtime"),
		intDef("VALERA_PROFILE_SYNC_MAX_RETRIES", 1, 0, 1, "retries", "Maximum automatic retries after Valera output is completed but its profile revision is not confirmed; PostgreSQL retry_count allows 0..1.", "runtime"),
		boolDef("CLEANUP_ENABLED", true, "Enable ESB stale-call recovery and retention cleanup loops, including immediate cleanup after terminal calls.", "cleanup"),
		durationDef("CLEANUP_STATUS_CHECK_INTERVAL", time.Hour, time.Millisecond, 365*24*time.Hour, "interval", "Interval between stale-call reconciliation passes; kept separate from data-retention cleanup.", "cleanup"),
		durationDef("CLEANUP_RETENTION_CHECK_INTERVAL", 24*time.Hour, time.Millisecond, 365*24*time.Hour, "interval", "Interval between bounded PostgreSQL and ClickHouse retention passes.", "cleanup"),
		intDef("CLEANUP_BATCH_SIZE", 500, 1, 10000, "rows", "Maximum candidate rows handled per cleanup category in one pass.", "cleanup"),
		durationDef("CLEANUP_QUEUED_STALE_AFTER", 10*time.Minute, time.Millisecond, 365*24*time.Hour, "duration", "Age after which a submitting call without a saved Hermes run ID can be safely returned to the queue.", "cleanup"),
		durationDef("CLEANUP_RUNNING_STALE_AFTER", 30*time.Minute, time.Millisecond, 365*24*time.Hour, "duration", "Minimum age before cleanup independently checks the Hermes status of an active call.", "cleanup"),
		intDef("CLEANUP_ACTIVE_MAX_RETRIES", 1, 0, 32767, "retries", "Maximum cleanup retries for calls Hermes confirms failed or interrupted; active or uncertain runs are never retried.", "cleanup"),
		retentionDef("CLEANUP_CALL_PAYLOAD_RETENTION", "2160h", "Age before large request and output fields of terminal calls are cleared; Serezha retry payloads stay available until processing succeeds."),
		retentionDef("CLEANUP_CALL_RECORD_RETENTION", "2160h", "Age before terminal call rows are deleted when safe or reduced to idempotency and history tombstones."),
		retentionDef("CLEANUP_CONTEXT_SUMMARY_RETENTION", "never", "Independent retention for compacted agent context summaries."),
		retentionDef("CLEANUP_MESSAGES_RETENTION_SEREZHA", "never", "Retention for Serezha conversation messages."),
		retentionDef("CLEANUP_MESSAGES_RETENTION_SAVELIY", "never", "Retention for Saveliy conversation messages."),
		retentionDef("CLEANUP_MESSAGES_RETENTION_VALERA", "never", "Retention for Valera conversation messages."),
		retentionDef("CLEANUP_MESSAGES_RETENTION_KESHA", "never", "Retention for Kesha conversation messages."),
		retentionDef("CLEANUP_MESSAGES_RETENTION_KONSTANTIN", "never", "Retention for Konstantin chat history exposed to Domain."),
		retentionDef("CLEANUP_PROCESSED_NEWS_TEXT_RETENTION", "168h", "Retention after ClickHouse confirms news processing and persisted facts; this age starts at the processing marker."),
		retentionDef("CLEANUP_UNPROCESSED_NEWS_TEXT_RETENTION", "never", "Retention for raw text only when its news call is terminal and the text is not marked processed."),
		retentionDef("CLEANUP_NEWS_METADATA_RETENTION", "never", "Retention for unreferenced ClickHouse news metadata; metadata required by retained facts or observations is preserved."),
		retentionDef("CLEANUP_NEWS_MARKER_RETENTION", "never", "Retention for processing markers only when no raw text or analytic rows depend on them."),
		retentionDef("CLEANUP_FACTS_RETENTION", "never", "Retention for ClickHouse universal facts."),
		retentionDef("CLEANUP_FACT_COMPANY_RETENTION", "never", "Retention for ClickHouse fact-to-company links."),
		retentionDef("CLEANUP_METRIC_OBSERVATIONS_RETENTION", "never", "Retention for metric observations."),
		retentionDef("CLEANUP_METRIC_TOTALS_RETENTION", "never", "Retention for calculated metric totals."),
		retentionDef("CLEANUP_COMPANY_ANALYSES_RETENTION", "never", "Retention for generated company analysis text."),
		durationDef("HERMES_REQUEST_TIMEOUT", 45*time.Second, time.Millisecond, 10*time.Minute, "duration", "HTTP client timeout for Hermes run, status, and health requests.", "runtime"),
		durationDef("HERMES_HEALTH_TIMEOUT", 2*time.Second, time.Millisecond, time.Minute, "duration", "Per-agent readiness probe timeout.", "runtime"),
		intDef("HERMES_RETRY_MAX_ATTEMPTS", 5, 1, 20, "attempts", "Total attempts to submit a run to Hermes, including the first attempt.", "runtime"),
		durationDef("HERMES_RETRY_BASE_DELAY", time.Second, 0, 5*time.Minute, "duration", "Base delay between Hermes submission retries; retry delay grows linearly by attempt.", "runtime"),
		int64Def("HERMES_MAX_RESPONSE_BYTES", 1<<20, 1024, 16<<20, "bytes", "Maximum response body size accepted from Hermes.", "runtime"),
		durationDef("CLICKHOUSE_REQUEST_TIMEOUT", 45*time.Second, time.Millisecond, time.Hour, "duration", "HTTP client timeout for ClickHouse requests.", "runtime"),
		int64Def("CLICKHOUSE_MAX_RESPONSE_BYTES", 4<<20, 1024, 64<<20, "bytes", "Maximum ClickHouse response body size accepted by ESB.", "runtime"),
		stringDef("CLICKHOUSE_URL", "http://localhost:8123", "ClickHouse HTTP interface URL.", "storage", false),
		stringDef("CLICKHOUSE_DATABASE", "default", "ClickHouse database name.", "storage", false),
		stringDef("CLICKHOUSE_USER", "predictor", "ClickHouse account used by ESB.", "storage", false),
		stringDef("CLICKHOUSE_PASSWORD", "", "ClickHouse account password.", "secret", true),
		stringDef("S3_BUCKET", "", "S3 bucket for company Markdown profiles.", "storage", true),
		stringDef("S3_REGION", "us-east-1", "S3 signing region.", "storage", false),
		stringDef("S3_ENDPOINT", "", "Optional S3-compatible endpoint; empty selects AWS endpoint discovery.", "storage", false),
		stringDef("S3_ACCESS_KEY_ID", "", "Optional static S3 access key; pair with S3_SECRET_ACCESS_KEY.", "secret", false),
		stringDef("S3_SECRET_ACCESS_KEY", "", "Optional static S3 secret key; pair with S3_ACCESS_KEY_ID.", "secret", false),
		boolDef("S3_FORCE_PATH_STYLE", false, "Use path-style S3 URLs; typically enabled for MinIO.", "storage"),
		boolDef("S3_CREATE_BUCKET", false, "Create the configured S3 bucket during ESB startup when missing.", "storage"),
		stringDef("AGENTS_DIR", "../agents", "Directory containing agent.json, skills, and Hermes configuration.", "runtime", false),
		intDef("FACTS_DEFAULT_PAGE_SIZE", defaultFactsPageSize, 1, 100000, "facts", "Number of facts returned when a facts query omits limit.", "runtime"),
		intDef("FACTS_MAX_PAGE_SIZE", defaultFactsMaxPageSize, 1, 100000, "facts", "Maximum facts returned by one paged facts query.", "runtime"),
		intDef("FACTS_MAX_PERIOD_DAYS", 90, 1, 3650, "days", "Maximum allowed time span for a filtered facts query.", "runtime"),
		intDef("COMPANY_ITEMS_DEFAULT_PAGE_SIZE", defaultCompanyPageSize, 1, 100000, "items", "Default page size for company fact and metric lists.", "runtime"),
		intDef("COMPANY_ITEMS_MAX_PAGE_SIZE", defaultCompanyMaxPageSize, 1, 100000, "items", "Maximum page size for company fact and metric lists.", "runtime"),
		intDef("CONSULTANT_CONTEXT_DEFAULT_PAGE_SIZE", defaultConsultantPageSize, 1, 100000, "items", "Default fact/observation page size in real-time consultant context.", "runtime"),
		intDef("CONSULTANT_CONTEXT_MAX_PAGE_SIZE", defaultConsultantMaxPageSize, 1, 100000, "items", "Maximum fact/observation page size in real-time consultant context.", "runtime"),
		intDef("KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE", defaultKonstantinHistoryPage, 1, 100000, "messages", "Default number of chat-history messages returned by one request.", "runtime"),
		intDef("KONSTANTIN_HISTORY_MAX_PAGE_SIZE", defaultKonstantinHistoryMax, 1, 100000, "messages", "Maximum number of chat-history messages returned by one request.", "runtime"),
		intDef("KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS", defaultKonstantinWindowDays, 1, 3650, "days", "Default lookback period for Konstantin chat history.", "runtime"),
		intDef("KONSTANTIN_HISTORY_MAX_WINDOW_DAYS", defaultKonstantinMaxWindowDays, 1, 3650, "days", "Maximum time span for Konstantin chat-history requests.", "runtime"),
		intDef("COMPANY_REPORT_DEFAULT_WINDOW_DAYS", 30, 1, 3650, "days", "Default lookback period for company reports when dates are omitted.", "runtime"),
		intDef("COMPANY_REPORT_FACTS_LIMIT", 200, 1, 100000, "facts", "Maximum company-linked facts included in one report.", "runtime"),
		intDef("COMPANY_REPORT_ANALYSIS_LIMIT", 100, 1, 100000, "analyses", "Maximum stored company analyses included in one report.", "runtime"),
		intDef("STREAM_REPLAY_LIMIT", 500, 1, 10000, "events", "Maximum prior Konstantin events replayed after WebSocket subscription.", "runtime"),
		durationDef("KONSTANTIN_EVENT_POLL_INTERVAL", 300*time.Millisecond, time.Millisecond, time.Minute, "interval", "Interval for checking PostgreSQL for a completed real-time consultant answer.", "runtime"),
		int64Def("HTTP_MAX_JSON_BODY_BYTES", 2<<20, 1024, 64<<20, "bytes", "Maximum JSON request body accepted by HTTP endpoints.", "runtime"),
		int64Def("MAX_NEWS_TEXT_BYTES", 1<<20, 1, 64<<20, "bytes", "Maximum raw news text accepted before Hermes processing.", "runtime"),
		int64Def("MAX_QUESTION_BYTES", 1<<20, 1, 16<<20, "bytes", "Maximum UTF-8 question text accepted over Konstantin WebSocket.", "runtime"),
		int64Def("WS_MAX_FRAME_BYTES", 1<<20, 1024, 16<<20, "bytes", "Maximum inbound WebSocket frame size.", "runtime"),
		int64Def("S3_PROFILE_MAX_BYTES", 2<<20, 1, 64<<20, "bytes", "Maximum company Markdown profile read from or written to S3.", "runtime"),
		intDef("MAX_FACTS_PER_NEWS", 500, 1, 100000, "facts", "Maximum facts stored for a single news item in one MCP call.", "runtime"),
		intDef("MAX_FACT_COMPANY_LINKS_PER_REQUEST", 1000, 1, 100000, "links", "Maximum fact-to-company links stored in one MCP call.", "runtime"),
		intDef("MAX_METRIC_OBSERVATIONS_PER_REQUEST", 1000, 1, 100000, "observations", "Maximum metric observations stored in one MCP call.", "runtime"),
		intDef("MAX_COMPANY_METRICS_PER_REQUEST", 500, 1, 100000, "metrics", "Maximum company metric settings accepted in one update.", "runtime"),
		durationDef("HTTP_READ_HEADER_TIMEOUT", 5*time.Second, time.Millisecond, 10*time.Minute, "duration", "HTTP server timeout for receiving request headers.", "runtime"),
		durationDef("HTTP_IDLE_TIMEOUT", 60*time.Second, time.Millisecond, 24*time.Hour, "duration", "HTTP server timeout for idle keep-alive connections.", "runtime"),
		durationDef("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second, time.Millisecond, 10*time.Minute, "duration", "Grace period for HTTP server shutdown after a termination signal.", "runtime"),
		durationDef("WS_WRITE_TIMEOUT", 15*time.Second, time.Millisecond, 10*time.Minute, "duration", "Maximum time to write one WebSocket frame.", "runtime"),
		durationDef("WS_READ_TIMEOUT", 0, 0, time.Hour, "duration", "Maximum idle time waiting for one WebSocket frame; zero keeps subscriptions open indefinitely.", "runtime"),
		intDef("WS_OUTGOING_QUEUE_SIZE", 64, 1, 65536, "frames", "Bounded queue of outbound WebSocket frames per connection.", "runtime"),
		stringDef("POSTGRES_DB", "predictor", "Database name for the PostgreSQL Compose service.", "compose", false),
		stringDef("POSTGRES_USER", "predictor", "Account name for the PostgreSQL Compose service.", "compose", false),
		stringDef("POSTGRES_PASSWORD", "local-predictor-password", "Local Compose PostgreSQL password; replace for private deployments.", "secret", false),
		durationDef("POSTGRES_HEALTHCHECK_INTERVAL", 5*time.Second, time.Millisecond, time.Hour, "duration", "PostgreSQL container healthcheck interval in Compose.", "compose"),
		durationDef("POSTGRES_HEALTHCHECK_TIMEOUT", 3*time.Second, time.Millisecond, time.Hour, "duration", "PostgreSQL container healthcheck command timeout in Compose.", "compose"),
		intDef("POSTGRES_HEALTHCHECK_RETRIES", 20, 1, 100, "retries", "Consecutive PostgreSQL healthcheck failures before the container is unhealthy.", "compose"),
		durationDef("CLICKHOUSE_HEALTHCHECK_INTERVAL", 5*time.Second, time.Millisecond, time.Hour, "duration", "ClickHouse container healthcheck interval in Compose.", "compose"),
		durationDef("CLICKHOUSE_HEALTHCHECK_TIMEOUT", 3*time.Second, time.Millisecond, time.Hour, "duration", "ClickHouse container healthcheck command timeout in Compose.", "compose"),
		intDef("CLICKHOUSE_HEALTHCHECK_RETRIES", 20, 1, 100, "retries", "Consecutive ClickHouse healthcheck failures before the container is unhealthy.", "compose"),
		durationDef("MINIO_HEALTHCHECK_INTERVAL", 3*time.Second, time.Millisecond, time.Hour, "duration", "MinIO container healthcheck interval in Compose.", "compose"),
		durationDef("MINIO_HEALTHCHECK_TIMEOUT", 2*time.Second, time.Millisecond, time.Hour, "duration", "MinIO container healthcheck command timeout in Compose.", "compose"),
		intDef("MINIO_HEALTHCHECK_RETRIES", 30, 1, 100, "retries", "Consecutive MinIO healthcheck failures before the container is unhealthy.", "compose"),
		durationDef("ESB_HEALTHCHECK_INTERVAL", 10*time.Second, time.Millisecond, time.Hour, "duration", "ESB container healthcheck interval in Compose.", "compose"),
		durationDef("ESB_HEALTHCHECK_TIMEOUT", 3*time.Second, time.Millisecond, time.Hour, "duration", "ESB container healthcheck command timeout in Compose.", "compose"),
		intDef("ESB_HEALTHCHECK_RETRIES", 20, 1, 100, "retries", "Consecutive ESB healthcheck failures before the container is unhealthy.", "compose"),
		stringDef("MINIO_ROOT_USER", "predictor-local", "Local MinIO root account.", "secret", false),
		stringDef("MINIO_ROOT_PASSWORD", "local-minio-password-change-me", "Local MinIO root password; replace for private deployments.", "secret", false),
		stringDef("LLM_API_KEY", "", "LLM provider key consumed by Hermes, never by ESB.", "secret", true),
		stringDef("ATLAS_TOKEN", "", "Optional Atlas token used by the ClickHouse Atlas driver.", "secret", false),
		intDef("ESB_HOST_PORT", 8081, 1, 65535, "port", "Host port published for the ESB HTTP service.", "compose"),
		intDef("MINIO_API_HOST_PORT", 9000, 1, 65535, "port", "Host port published for the MinIO S3 API.", "compose"),
		intDef("MINIO_CONSOLE_HOST_PORT", 9001, 1, 65535, "port", "Host port published for the MinIO console.", "compose"),
		stringDef("ESB_E2E_BASE_URL", "http://127.0.0.1:18081", "Test-only base URL for the ESB E2E suite.", "test", false),
		stringDef("MOCK_PORT", "8090", "Test-only listening port for the mock Hermes runtime.", "test", false),
	}
	for _, agent := range []domain.AgentType{domain.AgentSerezha, domain.AgentSaveliy, domain.AgentValera, domain.AgentKesha, domain.AgentKonstantin} {
		name := "MCP_TOKEN_" + strings.ToUpper(string(agent))
		token := stringDef(name, "", "Agent-only Bearer token for the ESB MCP endpoint; unique and separate from integration keys.", "secret", true)
		token.Range = "32…256"
		token.Unit = "bytes"
		settings = append(settings, token)
	}
	return append(settings, hermesAgentDefinitions()...)
}

func hermesAgentDefinitions() []Definition {
	type agentDefaults struct {
		agent      string
		model      string
		maxTurns   int64
		reasoning  string
		concurrent int64
	}

	defaults := []agentDefaults{
		{agent: "SEREZHA", model: "google/gemma-4-26b-a4b-it", maxTurns: 8, reasoning: "low", concurrent: 1},
		{agent: "SAVELIY", model: "google/gemma-4-26b-a4b-it", maxTurns: 8, reasoning: "low", concurrent: 1},
		{agent: "VALERA", model: "google/gemma-4-26b-a4b-it", maxTurns: 10, reasoning: "low", concurrent: 1},
		{agent: "KESHA", model: "google/gemma-4-26b-a4b-it", maxTurns: 12, reasoning: "low", concurrent: 1},
		{agent: "KONSTANTIN", model: "google/gemma-4-26b-a4b-it", maxTurns: 12, reasoning: "low", concurrent: 1},
	}

	settings := make([]Definition, 0, len(defaults)*6)
	for _, agent := range defaults {
		prefix := "AGENT_" + agent.agent + "_"
		settings = append(settings,
			stringDef(prefix+"MODEL_ID", agent.model, agent.agent+" model identifier sent to the configured provider.", "hermes-agent", false),
			intDef(prefix+"MAX_TURNS", agent.maxTurns, 1, 1000, "turns", agent.agent+" Hermes maximum turns per run.", "hermes-agent"),
			stringDef(prefix+"REASONING_EFFORT", agent.reasoning, agent.agent+" Hermes reasoning-effort setting supported by its model/provider.", "hermes-agent", false),
			intDef(prefix+"MAX_CONCURRENT_RUNS", agent.concurrent, 1, 100000, "runs", agent.agent+" maximum concurrent runs per Hermes instance; values below one are rejected.", "hermes-agent"),
		)
	}
	return settings
}

func stringDef(name, fallback, purpose, category string, required bool) Definition {
	defaultText := fallback
	if fallback == "" {
		if required {
			defaultText = "required"
		} else {
			defaultText = "empty / unset"
		}
	}
	return Definition{Name: name, Type: "string", Default: defaultText, Range: "not validated", Unit: "—", Purpose: purpose,
		Category: category, Secret: category == "secret", defaultRaw: fallback, required: required}
}

func boolDef(name string, fallback bool, purpose, category string) Definition {
	value := strconv.FormatBool(fallback)
	return Definition{Name: name, Type: "bool", Default: value, Range: "true | false", Unit: "—", Purpose: purpose,
		Category: category, defaultRaw: value}
}

func intDef(name string, fallback, min, max int64, unit, purpose, category string) Definition {
	return Definition{Name: name, Type: "integer", Default: strconv.FormatInt(fallback, 10),
		Range: fmt.Sprintf("%d…%d", min, max), Unit: unit, Purpose: purpose, Category: category,
		defaultRaw: strconv.FormatInt(fallback, 10), minInt: min, maxInt: max}
}

func int64Def(name string, fallback, min, max int64, unit, purpose, category string) Definition {
	definition := intDef(name, fallback, min, max, unit, purpose, category)
	definition.Type = "integer (int64)"
	return definition
}

func durationDef(name string, fallback, min, max time.Duration, unit, purpose, category string) Definition {
	minText, maxText := min.String(), max.String()
	return Definition{Name: name, Type: "duration", Default: fallback.String(), Range: minText + "…" + maxText,
		Unit: unit, Purpose: purpose, Category: category, defaultRaw: fallback.String(),
		minDuration: min, maxDuration: max}
}

func retentionDef(name, fallback, purpose string) Definition {
	return Definition{Name: name, Type: "duration | immediate | never", Default: fallback, Range: "0 | immediate | positive Go duration | never", Unit: "duration",
		Purpose: purpose, Category: "cleanup", defaultRaw: fallback, allowNever: true}
}

func Definitions() []Definition {
	result := make([]Definition, len(definitions))
	copy(result, definitions)
	return result
}

func Load() (Config, error) { return LoadFrom(os.LookupEnv) }

func LoadFrom(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("environment lookup is required")
	}
	var cfg Config
	var err error
	getString := func(name string) (string, error) { return stringValue(lookup, name) }
	getBool := func(name string) (bool, error) { return boolValue(lookup, name) }
	getInt := func(name string) (int, error) { return intValue(lookup, name) }
	getInt64 := func(name string) (int64, error) { return int64Value(lookup, name) }
	getDuration := func(name string) (time.Duration, error) { return durationValue(lookup, name) }

	if cfg.Connections.HTTPAddr, err = getString("HTTP_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.DatabaseURL, err = getString("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.ESBAPIKey, err = getString("ESB_API_KEY"); err != nil {
		return Config{}, err
	}
	cfg.Connections.AgentMCPTokens = make(map[domain.AgentType]string, 5)
	seenTokens := make(map[[32]byte]string, 6)
	seenTokens[sha256.Sum256([]byte(cfg.Connections.ESBAPIKey))] = "ESB_API_KEY"
	for _, agent := range []domain.AgentType{domain.AgentSerezha, domain.AgentSaveliy, domain.AgentValera, domain.AgentKesha, domain.AgentKonstantin} {
		name := "MCP_TOKEN_" + strings.ToUpper(string(agent))
		token, tokenErr := getString(name)
		if tokenErr != nil {
			return Config{}, tokenErr
		}
		if len(token) < 32 || len(token) > maxAgentMCPTokenBytes {
			return Config{}, fmt.Errorf("%s must contain 32 to %d bytes", name, maxAgentMCPTokenBytes)
		}
		digest := sha256.Sum256([]byte(token))
		if previous, exists := seenTokens[digest]; exists {
			return Config{}, fmt.Errorf("%s must be distinct from %s", name, previous)
		}
		seenTokens[digest] = name
		cfg.Connections.AgentMCPTokens[agent] = token
	}
	if cfg.Connections.HermesAPIKey, err = getString("HERMES_API_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.HermesAPIKey == cfg.Connections.ESBAPIKey {
		return Config{}, errors.New("HERMES_API_KEY must be distinct from ESB_API_KEY")
	}
	hermesKeyDigest := sha256.Sum256([]byte(cfg.Connections.HermesAPIKey))
	for agent, token := range cfg.Connections.AgentMCPTokens {
		tokenDigest := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(hermesKeyDigest[:], tokenDigest[:]) == 1 {
			return Config{}, fmt.Errorf("MCP_TOKEN_%s must be distinct from HERMES_API_KEY", strings.ToUpper(string(agent)))
		}
	}
	if cfg.Connections.ClickHouseURL, err = getString("CLICKHOUSE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.CHDatabase, err = getString("CLICKHOUSE_DATABASE"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.CHUser, err = getString("CLICKHOUSE_USER"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.CHPassword, err = getString("CLICKHOUSE_PASSWORD"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3Bucket, err = getString("S3_BUCKET"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3Region, err = getString("S3_REGION"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3Endpoint, err = getString("S3_ENDPOINT"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3AccessKey, err = getString("S3_ACCESS_KEY_ID"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3SecretKey, err = getString("S3_SECRET_ACCESS_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3PathStyle, err = getBool("S3_FORCE_PATH_STYLE"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.S3CreateBucket, err = getBool("S3_CREATE_BUCKET"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.AgentsDir, err = getString("AGENTS_DIR"); err != nil {
		return Config{}, err
	}
	if cfg.Connections.DatabaseURL == "" || cfg.Connections.ESBAPIKey == "" || cfg.Connections.HermesAPIKey == "" || cfg.Connections.S3Bucket == "" {
		return Config{}, errors.New("DATABASE_URL, ESB_API_KEY, HERMES_API_KEY and S3_BUCKET are required")
	}
	if (cfg.Connections.S3AccessKey == "") != (cfg.Connections.S3SecretKey == "") {
		return Config{}, errors.New("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY must be set together")
	}
	cfg.Connections.ClickHouseURL = strings.TrimRight(cfg.Connections.ClickHouseURL, "/")

	if cfg.HTTP.ReadHeaderTimeout, err = getDuration("HTTP_READ_HEADER_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.IdleTimeout, err = getDuration("HTTP_IDLE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.ShutdownTimeout, err = getDuration("HTTP_SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.MaxJSONBodyBytes, err = getInt64("HTTP_MAX_JSON_BODY_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.Queue.HermesPollInterval, err = getDuration("HERMES_POLL_INTERVAL"); err != nil {
		return Config{}, err
	}
	if cfg.Queue.DispatchBatchSize, err = getInt("QUEUE_DISPATCH_BATCH_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Queue.ContextCompactionTimeout, err = getDuration("CONTEXT_COMPACTION_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Queue.ValeraProfileSyncRetries, err = getInt("VALERA_PROFILE_SYNC_MAX_RETRIES"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Enabled, err = getBool("CLEANUP_ENABLED"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.StatusCheckInterval, err = getDuration("CLEANUP_STATUS_CHECK_INTERVAL"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.RetentionCheckInterval, err = getDuration("CLEANUP_RETENTION_CHECK_INTERVAL"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.BatchSize, err = getInt("CLEANUP_BATCH_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.QueuedStaleAfter, err = getDuration("CLEANUP_QUEUED_STALE_AFTER"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.RunningStaleAfter, err = getDuration("CLEANUP_RUNNING_STALE_AFTER"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.ActiveMaxRetries, err = getInt("CLEANUP_ACTIVE_MAX_RETRIES"); err != nil {
		return Config{}, err
	}
	getRetention := func(name string) (domain.RetentionPolicy, error) { return retentionValue(lookup, name) }
	if cfg.Cleanup.CallPayloadRetention, err = getRetention("CLEANUP_CALL_PAYLOAD_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.CallRecordRetention, err = getRetention("CLEANUP_CALL_RECORD_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.ContextSummaryRetention, err = getRetention("CLEANUP_CONTEXT_SUMMARY_RETENTION"); err != nil {
		return Config{}, err
	}
	cfg.Cleanup.MessagesRetention = make(map[domain.AgentType]domain.RetentionPolicy, 5)
	for _, agent := range []domain.AgentType{domain.AgentSerezha, domain.AgentSaveliy, domain.AgentValera, domain.AgentKesha, domain.AgentKonstantin} {
		policy, policyErr := getRetention("CLEANUP_MESSAGES_RETENTION_" + strings.ToUpper(string(agent)))
		if policyErr != nil {
			return Config{}, policyErr
		}
		cfg.Cleanup.MessagesRetention[agent] = policy
	}
	if cfg.Cleanup.Analytics.ProcessedNewsText, err = getRetention("CLEANUP_PROCESSED_NEWS_TEXT_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.UnprocessedNewsText, err = getRetention("CLEANUP_UNPROCESSED_NEWS_TEXT_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.NewsMetadata, err = getRetention("CLEANUP_NEWS_METADATA_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.NewsMarker, err = getRetention("CLEANUP_NEWS_MARKER_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.Facts, err = getRetention("CLEANUP_FACTS_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.FactCompany, err = getRetention("CLEANUP_FACT_COMPANY_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.MetricObservations, err = getRetention("CLEANUP_METRIC_OBSERVATIONS_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.MetricTotals, err = getRetention("CLEANUP_METRIC_TOTALS_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Cleanup.Analytics.CompanyAnalyses, err = getRetention("CLEANUP_COMPANY_ANALYSES_RETENTION"); err != nil {
		return Config{}, err
	}
	if cfg.Hermes.RequestTimeout, err = getDuration("HERMES_REQUEST_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Hermes.HealthTimeout, err = getDuration("HERMES_HEALTH_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Hermes.MaxAttempts, err = getInt("HERMES_RETRY_MAX_ATTEMPTS"); err != nil {
		return Config{}, err
	}
	if cfg.Hermes.RetryBaseDelay, err = getDuration("HERMES_RETRY_BASE_DELAY"); err != nil {
		return Config{}, err
	}
	if cfg.Hermes.MaxResponse, err = getInt64("HERMES_MAX_RESPONSE_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.ClickHouse.RequestTimeout, err = getDuration("CLICKHOUSE_REQUEST_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.ClickHouse.MaxResponse, err = getInt64("CLICKHOUSE_MAX_RESPONSE_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.WebSocket.MaxFrameBytes, err = getInt64("WS_MAX_FRAME_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.WebSocket.ReadTimeout, err = getDuration("WS_READ_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.WebSocket.WriteTimeout, err = getDuration("WS_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.WebSocket.OutgoingQueue, err = getInt("WS_OUTGOING_QUEUE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.WebSocket.ReplayLimit, err = getInt("STREAM_REPLAY_LIMIT"); err != nil {
		return Config{}, err
	}
	if cfg.WebSocket.EventPollInterval, err = getDuration("KONSTANTIN_EVENT_POLL_INTERVAL"); err != nil {
		return Config{}, err
	}

	if cfg.Pagination.FactsDefaultPageSize, err = getInt("FACTS_DEFAULT_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.FactsMaxPageSize, err = getInt("FACTS_MAX_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.FactsMaxPeriodDays, err = getInt("FACTS_MAX_PERIOD_DAYS"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.CompanyItemsDefaultPageSize, err = getInt("COMPANY_ITEMS_DEFAULT_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.CompanyItemsMaxPageSize, err = getInt("COMPANY_ITEMS_MAX_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.ConsultantDefaultPageSize, err = getInt("CONSULTANT_CONTEXT_DEFAULT_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.ConsultantMaxPageSize, err = getInt("CONSULTANT_CONTEXT_MAX_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.KonstantinHistoryDefaultPageSize, err = getInt("KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.KonstantinHistoryMaxPageSize, err = getInt("KONSTANTIN_HISTORY_MAX_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.KonstantinDefaultWindowDays, err = getInt("KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.KonstantinMaxWindowDays, err = getInt("KONSTANTIN_HISTORY_MAX_WINDOW_DAYS"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.CompanyReportDefaultWindowDays, err = getInt("COMPANY_REPORT_DEFAULT_WINDOW_DAYS"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.CompanyReportFactsLimit, err = getInt("COMPANY_REPORT_FACTS_LIMIT"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.CompanyReportAnalysisLimit, err = getInt("COMPANY_REPORT_ANALYSIS_LIMIT"); err != nil {
		return Config{}, err
	}
	if err := validatePagination(cfg.Pagination); err != nil {
		return Config{}, err
	}

	if cfg.Payload.MaxNewsTextBytes, err = getInt64("MAX_NEWS_TEXT_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxQuestionBytes, err = getInt64("MAX_QUESTION_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxProfileBytes, err = getInt64("S3_PROFILE_MAX_BYTES"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxFactsPerNews, err = getInt("MAX_FACTS_PER_NEWS"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxFactCompanyLinks, err = getInt("MAX_FACT_COMPANY_LINKS_PER_REQUEST"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxMetricObservations, err = getInt("MAX_METRIC_OBSERVATIONS_PER_REQUEST"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxCompanyMetricsPerRequest, err = getInt("MAX_COMPANY_METRICS_PER_REQUEST"); err != nil {
		return Config{}, err
	}
	if cfg.Payload.MaxNewsTextBytes+maxNewsRequestMetadataBytes > cfg.HTTP.MaxJSONBodyBytes {
		return Config{}, errors.New("MAX_NEWS_TEXT_BYTES must fit inside HTTP_MAX_JSON_BODY_BYTES with request metadata")
	}
	if cfg.Payload.MaxQuestionBytes > cfg.WebSocket.MaxFrameBytes {
		return Config{}, errors.New("MAX_QUESTION_BYTES cannot exceed WS_MAX_FRAME_BYTES")
	}

	root := cfg.Connections.AgentsDir
	if root == "" {
		root = "../agents"
	}
	cfg.AgentMCPTools = make(map[domain.AgentType][]string, 5)
	for _, name := range []string{"serezha", "saveliy", "valera", "kesha", "konstantin"} {
		definitionPath := filepath.Join(root, name, "agent.json")
		data, readErr := os.ReadFile(definitionPath)
		if readErr != nil {
			return Config{}, fmt.Errorf("read agent definition %s: %w", definitionPath, readErr)
		}
		var definition agentFile
		if err := json.Unmarshal(data, &definition); err != nil {
			return Config{}, fmt.Errorf("parse agent definition %s: %w", definitionPath, err)
		}
		if definition.Name != name || definition.ScopeField == "" || definition.SkillFile == "" || definition.RuntimeURLEnv == "" {
			return Config{}, fmt.Errorf("agent definition %s is incomplete", definitionPath)
		}
		mcpTools, parseErr := parseAgentMCPTools(definition.MCPTools)
		if parseErr != nil {
			return Config{}, fmt.Errorf("agent definition %s: %w", definitionPath, parseErr)
		}
		cfg.AgentMCPTools[domain.AgentType(name)] = mcpTools
		skillPath := filepath.Join(root, name, definition.SkillFile)
		skill, err := os.ReadFile(skillPath)
		if err != nil {
			return Config{}, fmt.Errorf("read skill for %s: %w", name, err)
		}
		runtimeURL, err := stringValue(lookup, definition.RuntimeURLEnv)
		if err != nil {
			return Config{}, err
		}
		instancesRaw := ""
		if definition.RuntimeInstancesEnv != "" {
			instancesRaw, err = stringValue(lookup, definition.RuntimeInstancesEnv)
			if err != nil {
				return Config{}, err
			}
		}
		if strings.TrimSpace(runtimeURL) != "" && strings.TrimSpace(instancesRaw) != "" {
			return Config{}, fmt.Errorf("set either %s or %s, not both", definition.RuntimeURLEnv, definition.RuntimeInstancesEnv)
		}
		maxConcurrentRuns, err := intValue(lookup, "AGENT_"+strings.ToUpper(name)+"_MAX_CONCURRENT_RUNS")
		if err != nil {
			return Config{}, err
		}
		var runtimeInstances []domain.RuntimeInstance
		if strings.TrimSpace(instancesRaw) != "" {
			runtimeInstances, err = parseRuntimeInstances(instancesRaw, domain.AgentType(name), maxConcurrentRuns)
			if err != nil {
				return Config{}, fmt.Errorf("parse %s: %w", definition.RuntimeInstancesEnv, err)
			}
		} else {
			baseURL, parseErr := normalizedRuntimeURL(runtimeURL)
			if parseErr != nil {
				return Config{}, fmt.Errorf("%s is required and must be a valid URL", definition.RuntimeURLEnv)
			}
			runtimeInstances = []domain.RuntimeInstance{{ID: "default", Agent: domain.AgentType(name), BaseURL: baseURL,
				State: domain.RuntimeAvailable, MaxConcurrentRuns: maxConcurrentRuns}}
		}
		policy, err := parseContextPolicy(definition.ContextPolicy)
		if err != nil {
			return Config{}, fmt.Errorf("agent definition %s: %w", definitionPath, err)
		}
		historyLimit := 0
		if definition.ContextPolicy != nil {
			historyLimit = definition.ContextPolicy.MaxHistoryMessages
		}
		cfg.Agents = append(cfg.Agents, domain.Agent{Type: domain.AgentType(name), ScopeField: definition.ScopeField,
			RequiredFields: definition.RequiredFields, MaxHistory: historyLimit,
			ContextPolicy: policy, RuntimeURL: strings.TrimRight(runtimeURL, "/"), RuntimeInstances: runtimeInstances,
			Instructions: skillInstructions(string(skill))})
	}
	return cfg, nil
}

type agentFile struct {
	Name                string             `json:"name"`
	ScopeField          string             `json:"scope_field"`
	SkillFile           string             `json:"skill_file"`
	RuntimeURLEnv       string             `json:"runtime_url_env"`
	RuntimeInstancesEnv string             `json:"runtime_instances_env"`
	RequiredFields      []string           `json:"required_fields"`
	MCPTools            []string           `json:"mcp_tools"`
	ContextPolicy       *contextPolicyFile `json:"context_policy"`
}

func parseAgentMCPTools(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("mcp_tools must list the agent's allowed ESB tools")
	}
	seen := make(map[string]struct{}, len(raw))
	tools := make([]string, 0, len(raw))
	for _, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 128 {
			return nil, errors.New("mcp_tools contains an invalid tool name")
		}
		for _, char := range name {
			if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
				continue
			}
			return nil, errors.New("mcp_tools contains an invalid tool name")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("mcp_tools contains duplicate tool %q", name)
		}
		seen[name] = struct{}{}
		tools = append(tools, name)
	}
	return tools, nil
}

func parseRuntimeInstances(raw string, agent domain.AgentType, maxConcurrentRuns int) ([]domain.RuntimeInstance, error) {
	if maxConcurrentRuns < 1 {
		return nil, errors.New("max concurrent runs per instance must be at least one")
	}
	entries := strings.Split(raw, ";")
	instances := make([]domain.RuntimeInstance, 0, len(entries))
	ids := make(map[string]struct{}, len(entries))
	urls := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, errors.New("pool contains an empty instance")
		}
		id, stateAndURL, ok := strings.Cut(entry, "=")
		state, baseURL, stateOK := strings.Cut(stateAndURL, "@")
		id = strings.TrimSpace(id)
		state = strings.TrimSpace(state)
		if !ok || !stateOK || !validRuntimeID(id) {
			return nil, errors.New("expected id=state@url entries")
		}
		if _, exists := ids[id]; exists {
			return nil, errors.New("pool contains a duplicate instance ID")
		}
		ids[id] = struct{}{}
		var runtimeState domain.RuntimeState
		switch domain.RuntimeState(state) {
		case domain.RuntimeAvailable:
			runtimeState = domain.RuntimeAvailable
		case domain.RuntimeDraining:
			runtimeState = domain.RuntimeDraining
		case domain.RuntimeUnavailable:
			runtimeState = domain.RuntimeUnavailable
		default:
			return nil, errors.New("instance state must be available, draining, or unavailable")
		}
		baseURL, err := normalizedRuntimeURL(baseURL)
		if err != nil {
			return nil, errors.New("instance URL must be an absolute HTTP or HTTPS URL without credentials, query, or fragment")
		}
		if _, exists := urls[baseURL]; exists {
			return nil, errors.New("pool contains a duplicate instance URL")
		}
		urls[baseURL] = struct{}{}
		instances = append(instances, domain.RuntimeInstance{ID: id, Agent: agent, BaseURL: baseURL,
			State: runtimeState, MaxConcurrentRuns: maxConcurrentRuns})
	}
	if len(instances) == 0 {
		return nil, errors.New("pool must contain at least one instance")
	}
	return instances, nil
}

func validRuntimeID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for index, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') ||
			(index > 0 && (char == '.' || char == '_' || char == '-')) {
			continue
		}
		return false
	}
	return true
}

func normalizedRuntimeURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid runtime URL")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

type contextPolicyFile struct {
	MaxHistoryMessages    int    `json:"max_history_messages"`
	CompressAfterMessages int    `json:"compress_after_messages"`
	BatchMessages         int    `json:"batch_messages"`
	SummaryMaxBytes       int    `json:"summary_max_bytes"`
	SummaryStorage        string `json:"summary_storage"`
}

func parseContextPolicy(raw *contextPolicyFile) (domain.ContextPolicy, error) {
	if raw == nil {
		return domain.ContextPolicy{}, nil
	}
	if raw.MaxHistoryMessages < 0 || raw.MaxHistoryMessages > 10000 || raw.CompressAfterMessages < 0 || raw.BatchMessages < 0 || raw.SummaryMaxBytes < 0 {
		return domain.ContextPolicy{}, errors.New("context policy values are outside supported bounds")
	}
	if raw.CompressAfterMessages == 0 {
		if raw.BatchMessages != 0 || raw.SummaryMaxBytes != 0 || raw.SummaryStorage != "" {
			return domain.ContextPolicy{}, errors.New("disabled context compression cannot set summary options")
		}
		return domain.ContextPolicy{}, nil
	}
	if raw.BatchMessages < 1 || raw.SummaryMaxBytes < 256 || raw.SummaryStorage != "postgres" {
		return domain.ContextPolicy{}, errors.New("enabled context compression requires batch_messages, summary_max_bytes >= 256 and summary_storage postgres")
	}
	return domain.ContextPolicy{CompressAfterMessages: raw.CompressAfterMessages, BatchMessages: raw.BatchMessages,
		SummaryMaxBytes: raw.SummaryMaxBytes, SummaryStorage: raw.SummaryStorage}, nil
}

func skillInstructions(markdown string) string {
	markdown = strings.TrimSpace(markdown)
	if strings.HasPrefix(markdown, "---\n") {
		if end := strings.Index(markdown[4:], "\n---"); end >= 0 {
			markdown = markdown[4+end+4:]
		}
	}
	return strings.TrimSpace(markdown)
}

func validatePagination(p Pagination) error {
	limits := []struct {
		name         string
		defaultValue int
		maximum      int
	}{
		{"FACTS_DEFAULT_PAGE_SIZE", p.FactsDefaultPageSize, p.FactsMaxPageSize},
		{"COMPANY_ITEMS_DEFAULT_PAGE_SIZE", p.CompanyItemsDefaultPageSize, p.CompanyItemsMaxPageSize},
		{"CONSULTANT_CONTEXT_DEFAULT_PAGE_SIZE", p.ConsultantDefaultPageSize, p.ConsultantMaxPageSize},
		{"KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE", p.KonstantinHistoryDefaultPageSize, p.KonstantinHistoryMaxPageSize},
	}
	for _, limit := range limits {
		if limit.defaultValue > limit.maximum {
			return fmt.Errorf("%s cannot exceed its corresponding maximum", limit.name)
		}
	}
	if p.KonstantinDefaultWindowDays > p.KonstantinMaxWindowDays {
		return errors.New("KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS cannot exceed KONSTANTIN_HISTORY_MAX_WINDOW_DAYS")
	}
	return nil
}

func stringValue(lookup LookupEnv, name string) (string, error) {
	spec := definition(name)
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		if spec.required {
			return "", fmt.Errorf("%s is required", name)
		}
		return spec.defaultRaw, nil
	}
	return value, nil
}

func boolValue(lookup LookupEnv, name string) (bool, error) {
	value, err := stringValue(lookup, name)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s: expected true or false", name)
	}
}

func intValue(lookup LookupEnv, name string) (int, error) {
	value, err := parseInt64(lookup, name)
	if err != nil {
		return 0, err
	}
	if int64(int(value)) != value {
		return 0, fmt.Errorf("invalid %s: integer overflows this platform", name)
	}
	return int(value), nil
}

func int64Value(lookup LookupEnv, name string) (int64, error) { return parseInt64(lookup, name) }

func parseInt64(lookup LookupEnv, name string) (int64, error) {
	spec := definition(name)
	value, err := stringValue(lookup, name)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: expected integer in %s", name, spec.Range)
	}
	if parsed < spec.minInt || parsed > spec.maxInt {
		return 0, fmt.Errorf("invalid %s: expected integer in %s", name, spec.Range)
	}
	return parsed, nil
}

func durationValue(lookup LookupEnv, name string) (time.Duration, error) {
	spec := definition(name)
	value, err := stringValue(lookup, name)
	if err != nil {
		return 0, err
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: expected duration in %s", name, spec.Range)
	}
	if parsed < spec.minDuration || parsed > spec.maxDuration {
		return 0, fmt.Errorf("invalid %s: expected duration in %s", name, spec.Range)
	}
	return parsed, nil
}

func retentionValue(lookup LookupEnv, name string) (domain.RetentionPolicy, error) {
	spec := definition(name)
	if !spec.allowNever {
		return domain.RetentionPolicy{}, fmt.Errorf("invalid %s: retention policy is not enabled", name)
	}
	value, err := stringValue(lookup, name)
	if err != nil {
		return domain.RetentionPolicy{}, err
	}
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "never") {
		return domain.RetentionPolicy{Never: true}, nil
	}
	if strings.EqualFold(value, "immediate") {
		return domain.RetentionPolicy{Immediate: true}, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return domain.RetentionPolicy{}, fmt.Errorf("invalid %s: expected 0, immediate, a positive Go duration, or never", name)
	}
	if parsed == 0 {
		return domain.RetentionPolicy{Immediate: true}, nil
	}
	return domain.RetentionPolicy{Age: parsed}, nil
}

func definition(name string) Definition {
	for _, spec := range definitions {
		if spec.Name == name {
			return spec
		}
	}
	panic("unknown ESB configuration setting: " + name)
}
