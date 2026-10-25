package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func testLookup(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func testEnvironment(t *testing.T) map[string]string {
	t.Helper()
	agentsDir, err := filepath.Abs(filepath.Join("..", "..", "..", "agents"))
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"DATABASE_URL":          "postgres://predictor:local-test@localhost/predictor?sslmode=disable",
		"ESB_API_KEY":           "test-esb-key",
		"HERMES_API_KEY":        "test-hermes-key",
		"CLICKHOUSE_PASSWORD":   "test-clickhouse-password",
		"S3_BUCKET":             "test-profile-bucket",
		"AGENTS_DIR":            agentsDir,
		"HERMES_SEREZHA_URL":    "http://127.0.0.1:9001",
		"HERMES_SAVELIY_URL":    "http://127.0.0.1:9002",
		"HERMES_VALERA_URL":     "http://127.0.0.1:9003",
		"HERMES_KESHA_URL":      "http://127.0.0.1:9004",
		"HERMES_KONSTANTIN_URL": "http://127.0.0.1:9005",
	}
}

func TestLoadDefaultsAndAgentHistoryZero(t *testing.T) {
	values := testEnvironment(t)
	cfg, err := LoadFrom(testLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Queue.HermesPollInterval != 2*time.Second || cfg.Queue.DispatchBatchSize != 100 {
		t.Fatalf("queue defaults = (%s, %d), want (2s, 100)", cfg.Queue.HermesPollInterval, cfg.Queue.DispatchBatchSize)
	}
	if cfg.Queue.ValeraProfileSyncRetries != 1 {
		t.Fatalf("Valera profile sync retries = %d, want 1 (the PostgreSQL schema maximum)", cfg.Queue.ValeraProfileSyncRetries)
	}
	if !cfg.Cleanup.Enabled || cfg.Cleanup.StatusCheckInterval != time.Hour || cfg.Cleanup.RetentionCheckInterval != 24*time.Hour || cfg.Cleanup.BatchSize != 500 {
		t.Fatalf("cleanup defaults = %#v", cfg.Cleanup)
	}
	if cfg.Cleanup.CallPayloadRetention.Age != 2160*time.Hour || cfg.Cleanup.CallRecordRetention.Age != 2160*time.Hour ||
		cfg.Cleanup.Analytics.ProcessedNewsText.Age != 168*time.Hour || !cfg.Cleanup.Analytics.UnprocessedNewsText.Never ||
		!cfg.Cleanup.MessagesRetention[domain.AgentKonstantin].Never {
		t.Fatalf("cleanup retention defaults = %#v", cfg.Cleanup)
	}
	if cfg.HTTP.MaxJSONBodyBytes != 2<<20 || cfg.ClickHouse.MaxResponse != 4<<20 || cfg.Payload.MaxProfileBytes != 2<<20 {
		t.Fatalf("payload defaults changed: http=%d clickhouse=%d profile=%d", cfg.HTTP.MaxJSONBodyBytes, cfg.ClickHouse.MaxResponse, cfg.Payload.MaxProfileBytes)
	}
	wantHistory := map[string]int{"serezha": 40, "saveliy": 40, "valera": 0, "kesha": 40, "konstantin": 80}
	for _, agent := range cfg.Agents {
		if agent.MaxHistory != wantHistory[string(agent.Type)] {
			t.Errorf("%s max history = %d, want %d", agent.Type, agent.MaxHistory, wantHistory[string(agent.Type)])
		}
	}
}

func TestRetentionPolicyParsesDurationsImmediateAndNever(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantAge   time.Duration
		wantNever bool
		wantNow   bool
		wantErr   bool
	}{
		{name: "never", value: "never", wantNever: true},
		{name: "case insensitive never", value: " NEVER ", wantNever: true},
		{name: "valid duration", value: "168h", wantAge: 168 * time.Hour},
		{name: "zero", value: "0", wantNow: true},
		{name: "zero duration", value: "0s", wantNow: true},
		{name: "immediate", value: "immediate", wantNow: true},
		{name: "case insensitive immediate", value: " IMMEDIATE ", wantNow: true},
		{name: "negative", value: "-1h", wantErr: true},
		{name: "invalid", value: "tomorrow", wantErr: true},
		{name: "overflow", value: "999999999999999999999999h", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := retentionValue(testLookup(map[string]string{"CLEANUP_FACTS_RETENTION": test.value}), "CLEANUP_FACTS_RETENTION")
			if (err != nil) != test.wantErr {
				t.Fatalf("retentionValue() error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && (got.Never != test.wantNever || got.Immediate != test.wantNow || got.Age != test.wantAge) {
				t.Fatalf("retentionValue() = %#v, want never=%t immediate=%t age=%s", got, test.wantNever, test.wantNow, test.wantAge)
			}
		})
	}
}

func TestCleanupConfigurationRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"CLEANUP_CONTEXT_SUMMARY_RETENTION", "-1s"},
		{"CLEANUP_STATUS_CHECK_INTERVAL", "0s"},
		{"CLEANUP_BATCH_SIZE", "0"},
		{"CLEANUP_ACTIVE_MAX_RETRIES", "-1"},
		{"CLEANUP_ENABLED", "yes"},
	} {
		t.Run(test.key, func(t *testing.T) {
			values := testEnvironment(t)
			values[test.key] = test.value
			if _, err := LoadFrom(testLookup(values)); err == nil {
				t.Fatalf("LoadFrom accepted %s=%q", test.key, test.value)
			}
		})
	}
}

func TestCleanupConfigurationAcceptsImmediateRetention(t *testing.T) {
	values := testEnvironment(t)
	values["CLEANUP_CALL_RECORD_RETENTION"] = "0s"
	values["CLEANUP_CONTEXT_SUMMARY_RETENTION"] = "immediate"
	cfg, err := LoadFrom(testLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Cleanup.CallRecordRetention.Immediate || !cfg.Cleanup.ContextSummaryRetention.Immediate {
		t.Fatalf("immediate retention was not applied: call=%#v summary=%#v", cfg.Cleanup.CallRecordRetention, cfg.Cleanup.ContextSummaryRetention)
	}
}

func TestLoadAgentHistoryOverrideHonorsZero(t *testing.T) {
	values := testEnvironment(t)
	values["AGENT_KESHA_MAX_HISTORY_MESSAGES"] = "0"
	values["AGENT_KONSTANTIN_MAX_HISTORY_MESSAGES"] = "123"
	cfg, err := LoadFrom(testLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range cfg.Agents {
		if agent.Type == "kesha" && agent.MaxHistory != 0 {
			t.Errorf("explicit zero was replaced: Kesha history=%d", agent.MaxHistory)
		}
		if agent.Type == "konstantin" && agent.MaxHistory != 123 {
			t.Errorf("history override = %d, want 123", agent.MaxHistory)
		}
	}
}

func TestTypedEnvironmentParsingBoundariesAndErrors(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantInt int64
		wantDur time.Duration
		wantErr bool
	}{
		{name: "integer default", key: "QUEUE_DISPATCH_BATCH_SIZE", wantInt: 100},
		{name: "integer lower boundary", key: "QUEUE_DISPATCH_BATCH_SIZE", value: "1", wantInt: 1},
		{name: "integer upper boundary", key: "QUEUE_DISPATCH_BATCH_SIZE", value: "10000", wantInt: 10000},
		{name: "integer below range", key: "QUEUE_DISPATCH_BATCH_SIZE", value: "0", wantErr: true},
		{name: "integer overflow", key: "QUEUE_DISPATCH_BATCH_SIZE", value: "999999999999999999999999999999", wantErr: true},
		{name: "valid zero", key: "AGENT_VALERA_MAX_HISTORY_MESSAGES", value: "0", wantInt: 0},
		{name: "Valera retries can be disabled", key: "VALERA_PROFILE_SYNC_MAX_RETRIES", value: "0", wantInt: 0},
		{name: "Valera retries schema maximum", key: "VALERA_PROFILE_SYNC_MAX_RETRIES", value: "1", wantInt: 1},
		{name: "Valera retries exceed schema", key: "VALERA_PROFILE_SYNC_MAX_RETRIES", value: "2", wantErr: true},
		{name: "negative history", key: "AGENT_VALERA_MAX_HISTORY_MESSAGES", value: "-1", wantErr: true},
		{name: "duration default", key: "HERMES_POLL_INTERVAL", wantDur: 2 * time.Second},
		{name: "duration lower boundary", key: "HERMES_POLL_INTERVAL", value: "1ms", wantDur: time.Millisecond},
		{name: "duration upper boundary", key: "HERMES_POLL_INTERVAL", value: "10m", wantDur: 10 * time.Minute},
		{name: "invalid duration", key: "HERMES_POLL_INTERVAL", value: "not-a-duration", wantErr: true},
		{name: "duration overflow", key: "HERMES_POLL_INTERVAL", value: "999999999999999999999999h", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]string{}
			if tt.value != "" {
				values[tt.key] = tt.value
			}
			lookup := testLookup(values)
			var err error
			if definition(tt.key).Type == "duration" {
				got, parseErr := durationValue(lookup, tt.key)
				err = parseErr
				if err == nil && got != tt.wantDur {
					t.Fatalf("duration = %s, want %s", got, tt.wantDur)
				}
			} else {
				got, parseErr := parseInt64(lookup, tt.key)
				err = parseErr
				if err == nil && got != tt.wantInt {
					t.Fatalf("integer = %d, want %d", got, tt.wantInt)
				}
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("parse error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestNumericSettingCatalogBoundaries(t *testing.T) {
	for _, setting := range Definitions() {
		setting := setting
		switch setting.Type {
		case "integer", "integer (int64)":
			t.Run(setting.Name, func(t *testing.T) {
				parse := func(value int64) error {
					_, err := parseInt64(testLookup(map[string]string{setting.Name: strconv.FormatInt(value, 10)}), setting.Name)
					return err
				}
				if err := parse(setting.minInt); err != nil {
					t.Errorf("lower boundary %d rejected: %v", setting.minInt, err)
				}
				if err := parse(setting.maxInt); err != nil {
					t.Errorf("upper boundary %d rejected: %v", setting.maxInt, err)
				}
				if setting.minInt == 0 {
					if err := parse(0); err != nil {
						t.Errorf("allowed zero rejected: %v", err)
					}
				} else if err := parse(setting.minInt - 1); err == nil {
					t.Errorf("value below lower boundary %d was accepted", setting.minInt)
				}
				if err := parse(setting.maxInt + 1); err == nil {
					t.Errorf("value above upper boundary %d was accepted", setting.maxInt)
				}
			})
		case "duration":
			t.Run(setting.Name, func(t *testing.T) {
				parse := func(value time.Duration) error {
					_, err := durationValue(testLookup(map[string]string{setting.Name: value.String()}), setting.Name)
					return err
				}
				if err := parse(setting.minDuration); err != nil {
					t.Errorf("lower boundary %s rejected: %v", setting.minDuration, err)
				}
				if err := parse(setting.maxDuration); err != nil {
					t.Errorf("upper boundary %s rejected: %v", setting.maxDuration, err)
				}
				if setting.minDuration == 0 {
					if err := parse(0); err != nil {
						t.Errorf("allowed zero rejected: %v", err)
					}
				} else if err := parse(setting.minDuration - time.Nanosecond); err == nil {
					t.Errorf("duration below lower boundary %s was accepted", setting.minDuration)
				}
				if err := parse(setting.maxDuration + time.Nanosecond); err == nil {
					t.Errorf("duration above upper boundary %s was accepted", setting.maxDuration)
				}
			})
		}
	}
}

func TestLoadAppliesOperationalOverrides(t *testing.T) {
	values := testEnvironment(t)
	values["QUEUE_DISPATCH_BATCH_SIZE"] = "7"
	values["VALERA_PROFILE_SYNC_MAX_RETRIES"] = "0"
	values["HERMES_REQUEST_TIMEOUT"] = "1m"
	values["CLICKHOUSE_REQUEST_TIMEOUT"] = "2m"
	values["HTTP_READ_HEADER_TIMEOUT"] = "7s"
	values["HTTP_MAX_JSON_BODY_BYTES"] = "16384"
	values["MAX_NEWS_TEXT_BYTES"] = "12000"
	values["MAX_QUESTION_BYTES"] = "2048"
	values["WS_MAX_FRAME_BYTES"] = "4096"
	values["FACTS_DEFAULT_PAGE_SIZE"] = "7"
	values["FACTS_MAX_PAGE_SIZE"] = "13"
	values["KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE"] = "4"
	values["KONSTANTIN_HISTORY_MAX_PAGE_SIZE"] = "8"
	values["STREAM_REPLAY_LIMIT"] = "17"

	cfg, err := LoadFrom(testLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Queue.DispatchBatchSize != 7 || cfg.Queue.ValeraProfileSyncRetries != 0 {
		t.Errorf("queue overrides not applied: batch=%d Valera retries=%d", cfg.Queue.DispatchBatchSize, cfg.Queue.ValeraProfileSyncRetries)
	}
	if cfg.Hermes.RequestTimeout != time.Minute || cfg.ClickHouse.RequestTimeout != 2*time.Minute || cfg.HTTP.ReadHeaderTimeout != 7*time.Second {
		t.Errorf("timeout overrides not applied: Hermes=%s ClickHouse=%s HTTP=%s", cfg.Hermes.RequestTimeout, cfg.ClickHouse.RequestTimeout, cfg.HTTP.ReadHeaderTimeout)
	}
	if cfg.HTTP.MaxJSONBodyBytes != 16384 || cfg.Payload.MaxNewsTextBytes != 12000 || cfg.Payload.MaxQuestionBytes != 2048 || cfg.WebSocket.MaxFrameBytes != 4096 {
		t.Errorf("payload overrides not applied: http=%d news=%d question=%d frame=%d", cfg.HTTP.MaxJSONBodyBytes, cfg.Payload.MaxNewsTextBytes, cfg.Payload.MaxQuestionBytes, cfg.WebSocket.MaxFrameBytes)
	}
	if cfg.Pagination.FactsDefaultPageSize != 7 || cfg.Pagination.FactsMaxPageSize != 13 || cfg.Pagination.KonstantinHistoryDefaultPageSize != 4 || cfg.Pagination.KonstantinHistoryMaxPageSize != 8 || cfg.WebSocket.ReplayLimit != 17 {
		t.Errorf("pagination overrides not applied: %#v replay=%d", cfg.Pagination, cfg.WebSocket.ReplayLimit)
	}
}

func TestBoolParsingIsStrict(t *testing.T) {
	tests := []struct {
		value string
		want  bool
		bad   bool
	}{
		{value: "true", want: true}, {value: "false"}, {value: "TRUE", want: true},
		{value: "sometimes", bad: true}, {value: "2", bad: true}, {value: "1", bad: true}, {value: "t", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := boolValue(testLookup(map[string]string{"S3_FORCE_PATH_STYLE": tt.value}), "S3_FORCE_PATH_STYLE")
			if (err != nil) != tt.bad {
				t.Fatalf("bool parse error = %v, bad=%t", err, tt.bad)
			}
			if err == nil && got != tt.want {
				t.Fatalf("bool = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestInvalidEnvironmentErrorNamesVariableButOmitsValue(t *testing.T) {
	const marker = "sensitive-value-marker"
	_, err := intValue(testLookup(map[string]string{"QUEUE_DISPATCH_BATCH_SIZE": marker}), "QUEUE_DISPATCH_BATCH_SIZE")
	if err == nil || !strings.Contains(err.Error(), "QUEUE_DISPATCH_BATCH_SIZE") || strings.Contains(err.Error(), marker) {
		t.Fatalf("invalid env error must name the variable and omit its value: %v", err)
	}
}

func TestLoadRejectsInconsistentPaginationAndInvalidBool(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
		want string
	}{
		{name: "page default exceeds maximum", set: map[string]string{"FACTS_DEFAULT_PAGE_SIZE": "5", "FACTS_MAX_PAGE_SIZE": "4"}, want: "FACTS_DEFAULT_PAGE_SIZE"},
		{name: "history default window exceeds maximum", set: map[string]string{"KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS": "91"}, want: "KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS"},
		{name: "bad bool", set: map[string]string{"S3_CREATE_BUCKET": "yes-please"}, want: "S3_CREATE_BUCKET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := testEnvironment(t)
			for key, value := range tt.set {
				values[key] = value
			}
			_, err := LoadFrom(testLookup(values))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("config error = %v, want it to name %s", err, tt.want)
			}
		})
	}
}

func TestSettingsCatalogHasUniqueNamesAndAgentConfigExists(t *testing.T) {
	seen := map[string]bool{}
	for _, setting := range Definitions() {
		if setting.Name == "" || seen[setting.Name] {
			t.Fatalf("empty or duplicated setting name: %q", setting.Name)
		}
		seen[setting.Name] = true
		if setting.Type == "" || setting.Purpose == "" || setting.Range == "" || setting.Unit == "" {
			t.Errorf("incomplete setting documentation metadata for %s", setting.Name)
		}
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "agents", "serezha", "agent.json")); err != nil {
		t.Fatal(err)
	}
	documentation := string(RenderDocumentation())
	for _, secret := range []string{"local-clickhouse-password", "local-predictor-password", "local-minio-password-change-me", "replace-with-a-random-secret"} {
		if strings.Contains(documentation, secret) {
			t.Errorf("generated config documentation exposes local secret placeholder %q", secret)
		}
	}
}

func TestSettingsCatalogMatchesEnvExampleAndCompose(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	envExamplePath := filepath.Join(root, "predictor", "example.env")
	envExample, err := os.ReadFile(envExamplePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envExample), "CLEANUP_PROCESSED_NEWS_TEXT_RETENTION=168h") ||
		!strings.Contains(string(envExample), "CLEANUP_CALL_RECORD_RETENTION=2160h") ||
		!strings.Contains(string(envExample), "CLEANUP_MESSAGES_RETENTION_KONSTANTIN=never") {
		t.Fatal("example.env does not include representative retention policies")
	}
	if !strings.Contains(string(envExample), "Company metric units") || !strings.Contains(string(envExample), "0, 0s, or immediate") ||
		!bytes.Equal(envExample, RenderExampleEnv()) {
		t.Fatal("example.env is missing unit guidance or differs from the generated configuration catalog")
	}
	compose, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	e2eCompose, err := os.ReadFile(filepath.Join(root, "predictor", "tests", "compose.e2e.yml"))
	if err != nil {
		t.Fatal(err)
	}
	e2eTest, err := os.ReadFile(filepath.Join(root, "predictor", "tests", "e2e_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range Definitions() {
		if setting.Category == "test" {
			referenced := strings.Contains(string(e2eCompose), setting.Name) || strings.Contains(string(e2eTest), setting.Name)
			if !referenced {
				t.Errorf("test-only setting %s is not referenced by the E2E harness", setting.Name)
			}
			continue
		}
		if !strings.Contains(string(envExample), setting.Name+"=") {
			t.Errorf("%s is missing from predictor/example.env", setting.Name)
		}
		if !strings.Contains(string(envExample), "# "+setting.Purpose+" (type:") {
			t.Errorf("%s is missing its explanatory comment in predictor/example.env", setting.Name)
		}
		if !strings.Contains(string(compose), setting.Name) {
			t.Errorf("%s is missing from Docker Compose", setting.Name)
		}
	}
}
