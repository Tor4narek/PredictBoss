<!-- Generated from the typed ESB environment setting catalog. Do not edit by hand. -->
# Конфигурация Predictor

`predictor/.env` — общий env-файл для ESB, Hermes и Atlas; он не хранится в Git. Скопируйте `predictor/example.env`, задайте credentials и запустите Compose с `--env-file predictor/.env`. Прямой запуск Go или Hermes не загружает файл автоматически.

## Переменные окружения

| Имя | Тип | Default | Единица / диапазон | Назначение |
| --- | --- | --- | --- | --- |
| `AGENTS_DIR` | string | ../agents | —; not validated | Directory containing agent.json, skills, and Hermes configuration. |
| `CLICKHOUSE_MAX_RESPONSE_BYTES` | integer (int64) | 4194304 | bytes; 1024…67108864 | Maximum ClickHouse response body size accepted by ESB. |
| `CLICKHOUSE_REQUEST_TIMEOUT` | duration | 45s | duration; 1ms…1h0m0s | HTTP client timeout for ClickHouse requests. |
| `COMPANY_ITEMS_DEFAULT_PAGE_SIZE` | integer | 100 | items; 1…100000 | Default page size for company fact and metric lists. |
| `COMPANY_ITEMS_MAX_PAGE_SIZE` | integer | 500 | items; 1…100000 | Maximum page size for company fact and metric lists. |
| `COMPANY_REPORT_ANALYSIS_LIMIT` | integer | 100 | analyses; 1…100000 | Maximum stored company analyses included in one report. |
| `COMPANY_REPORT_DEFAULT_WINDOW_DAYS` | integer | 30 | days; 1…3650 | Default lookback period for company reports when dates are omitted. |
| `COMPANY_REPORT_FACTS_LIMIT` | integer | 200 | facts; 1…100000 | Maximum company-linked facts included in one report. |
| `CONSULTANT_CONTEXT_DEFAULT_PAGE_SIZE` | integer | 100 | items; 1…100000 | Default fact/observation page size in real-time consultant context. |
| `CONSULTANT_CONTEXT_MAX_PAGE_SIZE` | integer | 500 | items; 1…100000 | Maximum fact/observation page size in real-time consultant context. |
| `CONTEXT_COMPACTION_TIMEOUT` | duration | 30s | duration; 1ms…1h0m0s | Maximum time spent requesting a compressed agent-context summary from Hermes. |
| `FACTS_DEFAULT_PAGE_SIZE` | integer | 100 | facts; 1…100000 | Number of facts returned when a facts query omits limit. |
| `FACTS_MAX_PAGE_SIZE` | integer | 100 | facts; 1…100000 | Maximum facts returned by one paged facts query. |
| `FACTS_MAX_PERIOD_DAYS` | integer | 90 | days; 1…3650 | Maximum allowed time span for a filtered facts query. |
| `HERMES_HEALTH_TIMEOUT` | duration | 2s | duration; 1ms…1m0s | Per-agent readiness probe timeout. |
| `HERMES_KESHA_INSTANCES` | string | empty / unset | —; not validated | Named Kesha runtime pool as id=state@url entries separated by semicolons. |
| `HERMES_KESHA_URL` | string | empty / unset | —; not validated | Legacy single Kesha Hermes URL; use HERMES_KESHA_INSTANCES for a pool. |
| `HERMES_KONSTANTIN_INSTANCES` | string | empty / unset | —; not validated | Named Konstantin runtime pool as id=state@url entries separated by semicolons. |
| `HERMES_KONSTANTIN_URL` | string | empty / unset | —; not validated | Legacy single Konstantin Hermes URL; use HERMES_KONSTANTIN_INSTANCES for a pool. |
| `HERMES_MAX_RESPONSE_BYTES` | integer (int64) | 1048576 | bytes; 1024…16777216 | Maximum response body size accepted from Hermes. |
| `HERMES_POLL_INTERVAL` | duration | 2s | interval; 1ms…10m0s | Queue dispatch and Hermes status polling interval. |
| `HERMES_REQUEST_TIMEOUT` | duration | 45s | duration; 1ms…10m0s | HTTP client timeout for Hermes run, status, and health requests. |
| `HERMES_RETRY_BASE_DELAY` | duration | 1s | duration; 0s…5m0s | Base delay between Hermes submission retries; retry delay grows linearly by attempt. |
| `HERMES_RETRY_MAX_ATTEMPTS` | integer | 5 | attempts; 1…20 | Total attempts to submit a run to Hermes, including the first attempt. |
| `HERMES_SAVELIY_INSTANCES` | string | empty / unset | —; not validated | Named Saveliy runtime pool as id=state@url entries separated by semicolons. |
| `HERMES_SAVELIY_URL` | string | empty / unset | —; not validated | Legacy single Saveliy Hermes URL; use HERMES_SAVELIY_INSTANCES for a pool. |
| `HERMES_SEREZHA_INSTANCES` | string | empty / unset | —; not validated | Named Serezha runtime pool as id=state@url entries separated by semicolons. |
| `HERMES_SEREZHA_URL` | string | empty / unset | —; not validated | Legacy single Serezha Hermes URL; use HERMES_SEREZHA_INSTANCES for a pool. |
| `HERMES_VALERA_INSTANCES` | string | empty / unset | —; not validated | Named Valera runtime pool as id=state@url entries separated by semicolons. |
| `HERMES_VALERA_URL` | string | empty / unset | —; not validated | Legacy single Valera Hermes URL; use HERMES_VALERA_INSTANCES for a pool. |
| `HTTP_ADDR` | string | :8081 | —; not validated | ESB listen address inside its network namespace. |
| `HTTP_IDLE_TIMEOUT` | duration | 1m0s | duration; 1ms…24h0m0s | HTTP server timeout for idle keep-alive connections. |
| `HTTP_MAX_JSON_BODY_BYTES` | integer (int64) | 2097152 | bytes; 1024…67108864 | Maximum JSON request body accepted by HTTP endpoints. |
| `HTTP_READ_HEADER_TIMEOUT` | duration | 5s | duration; 1ms…10m0s | HTTP server timeout for receiving request headers. |
| `HTTP_SHUTDOWN_TIMEOUT` | duration | 10s | duration; 1ms…10m0s | Grace period for HTTP server shutdown after a termination signal. |
| `KONSTANTIN_EVENT_POLL_INTERVAL` | duration | 300ms | interval; 1ms…1m0s | Interval for checking PostgreSQL for a completed real-time consultant answer. |
| `KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE` | integer | 100 | messages; 1…100000 | Default number of chat-history messages returned by one request. |
| `KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS` | integer | 30 | days; 1…3650 | Default lookback period for Konstantin chat history. |
| `KONSTANTIN_HISTORY_MAX_PAGE_SIZE` | integer | 100 | messages; 1…100000 | Maximum number of chat-history messages returned by one request. |
| `KONSTANTIN_HISTORY_MAX_WINDOW_DAYS` | integer | 90 | days; 1…3650 | Maximum time span for Konstantin chat-history requests. |
| `MAX_COMPANY_METRICS_PER_REQUEST` | integer | 500 | metrics; 1…100000 | Maximum company metric settings accepted in one update. |
| `MAX_FACTS_PER_NEWS` | integer | 500 | facts; 1…100000 | Maximum facts stored for a single news item in one MCP call. |
| `MAX_FACT_COMPANY_LINKS_PER_REQUEST` | integer | 1000 | links; 1…100000 | Maximum fact-to-company links stored in one MCP call. |
| `MAX_METRIC_OBSERVATIONS_PER_REQUEST` | integer | 1000 | observations; 1…100000 | Maximum metric observations stored in one MCP call. |
| `MAX_NEWS_TEXT_BYTES` | integer (int64) | 1048576 | bytes; 1…67108864 | Maximum raw news text accepted before Hermes processing. |
| `MAX_QUESTION_BYTES` | integer (int64) | 1048576 | bytes; 1…16777216 | Maximum UTF-8 question text accepted over Konstantin WebSocket. |
| `QUEUE_DISPATCH_BATCH_SIZE` | integer | 100 | calls; 1…10000 | Maximum queued calls claimed in one dispatch cycle; each claim can start one worker. |
| `S3_PROFILE_MAX_BYTES` | integer (int64) | 2097152 | bytes; 1…67108864 | Maximum company Markdown profile read from or written to S3. |
| `STREAM_REPLAY_LIMIT` | integer | 500 | events; 1…10000 | Maximum prior Konstantin events replayed after WebSocket subscription. |
| `VALERA_PROFILE_SYNC_MAX_RETRIES` | integer | 1 | retries; 0…1 | Maximum automatic retries after Valera output is completed but its profile revision is not confirmed; PostgreSQL retry_count allows 0..1. |
| `WS_MAX_FRAME_BYTES` | integer (int64) | 1048576 | bytes; 1024…16777216 | Maximum inbound WebSocket frame size. |
| `WS_OUTGOING_QUEUE_SIZE` | integer | 64 | frames; 1…65536 | Bounded queue of outbound WebSocket frames per connection. |
| `WS_READ_TIMEOUT` | duration | 0s | duration; 0s…1h0m0s | Maximum idle time waiting for one WebSocket frame; zero keeps subscriptions open indefinitely. |
| `WS_WRITE_TIMEOUT` | duration | 15s | duration; 1ms…10m0s | Maximum time to write one WebSocket frame. |
| `HERMES_MODEL_PROVIDER` | string | custom | —; not validated | Shared Hermes model-provider identifier for all agents. |
| `AGENT_KESHA_MAX_CONCURRENT_RUNS` | integer | 1 | runs; 1…100000 | KESHA maximum concurrent runs per Hermes instance; values below one are rejected. |
| `AGENT_KESHA_MAX_TURNS` | integer | 12 | turns; 1…1000 | KESHA Hermes maximum turns per run. |
| `AGENT_KESHA_MODEL_ID` | string | google/gemma-4-26b-a4b-it | —; not validated | KESHA model identifier sent to the configured provider. |
| `AGENT_KESHA_REASONING_EFFORT` | string | low | —; not validated | KESHA Hermes reasoning-effort setting supported by its model/provider. |
| `AGENT_KONSTANTIN_MAX_CONCURRENT_RUNS` | integer | 1 | runs; 1…100000 | KONSTANTIN maximum concurrent runs per Hermes instance; values below one are rejected. |
| `AGENT_KONSTANTIN_MAX_TURNS` | integer | 12 | turns; 1…1000 | KONSTANTIN Hermes maximum turns per run. |
| `AGENT_KONSTANTIN_MODEL_ID` | string | google/gemma-4-26b-a4b-it | —; not validated | KONSTANTIN model identifier sent to the configured provider. |
| `AGENT_KONSTANTIN_REASONING_EFFORT` | string | low | —; not validated | KONSTANTIN Hermes reasoning-effort setting supported by its model/provider. |
| `AGENT_SAVELIY_MAX_CONCURRENT_RUNS` | integer | 1 | runs; 1…100000 | SAVELIY maximum concurrent runs per Hermes instance; values below one are rejected. |
| `AGENT_SAVELIY_MAX_TURNS` | integer | 8 | turns; 1…1000 | SAVELIY Hermes maximum turns per run. |
| `AGENT_SAVELIY_MODEL_ID` | string | google/gemma-4-26b-a4b-it | —; not validated | SAVELIY model identifier sent to the configured provider. |
| `AGENT_SAVELIY_REASONING_EFFORT` | string | low | —; not validated | SAVELIY Hermes reasoning-effort setting supported by its model/provider. |
| `AGENT_SEREZHA_MAX_CONCURRENT_RUNS` | integer | 1 | runs; 1…100000 | SEREZHA maximum concurrent runs per Hermes instance; values below one are rejected. |
| `AGENT_SEREZHA_MAX_TURNS` | integer | 8 | turns; 1…1000 | SEREZHA Hermes maximum turns per run. |
| `AGENT_SEREZHA_MODEL_ID` | string | google/gemma-4-26b-a4b-it | —; not validated | SEREZHA model identifier sent to the configured provider. |
| `AGENT_SEREZHA_REASONING_EFFORT` | string | low | —; not validated | SEREZHA Hermes reasoning-effort setting supported by its model/provider. |
| `AGENT_VALERA_MAX_CONCURRENT_RUNS` | integer | 1 | runs; 1…100000 | VALERA maximum concurrent runs per Hermes instance; values below one are rejected. |
| `AGENT_VALERA_MAX_TURNS` | integer | 10 | turns; 1…1000 | VALERA Hermes maximum turns per run. |
| `AGENT_VALERA_MODEL_ID` | string | google/gemma-4-26b-a4b-it | —; not validated | VALERA model identifier sent to the configured provider. |
| `AGENT_VALERA_REASONING_EFFORT` | string | low | —; not validated | VALERA Hermes reasoning-effort setting supported by its model/provider. |
| `CLICKHOUSE_DATABASE` | string | default | —; not validated | ClickHouse database name. |
| `CLICKHOUSE_URL` | string | http://localhost:8123 | —; not validated | ClickHouse HTTP interface URL. |
| `CLICKHOUSE_USER` | string | predictor | —; not validated | ClickHouse account used by ESB. |
| `S3_BUCKET` | string | required | —; not validated | S3 bucket for company Markdown profiles. |
| `S3_CREATE_BUCKET` | bool | false | —; true \| false | Create the configured S3 bucket during ESB startup when missing. |
| `S3_ENDPOINT` | string | empty / unset | —; not validated | Optional S3-compatible endpoint; empty selects AWS endpoint discovery. |
| `S3_FORCE_PATH_STYLE` | bool | false | —; true \| false | Use path-style S3 URLs; typically enabled for MinIO. |
| `S3_REGION` | string | us-east-1 | —; not validated | S3 signing region. |
| `CLEANUP_ACTIVE_MAX_RETRIES` | integer | 1 | retries; 0…32767 | Maximum cleanup retries for calls Hermes confirms failed or interrupted; active or uncertain runs are never retried. |
| `CLEANUP_BATCH_SIZE` | integer | 500 | rows; 1…10000 | Maximum candidate rows handled per cleanup category in one pass. |
| `CLEANUP_CALL_PAYLOAD_RETENTION` | duration \| immediate \| never | 2160h | duration; 0 \| immediate \| positive Go duration \| never | Age before large request and output fields of terminal calls are cleared; Serezha retry payloads stay available until processing succeeds. |
| `CLEANUP_CALL_RECORD_RETENTION` | duration \| immediate \| never | 2160h | duration; 0 \| immediate \| positive Go duration \| never | Age before terminal call rows are deleted when safe or reduced to idempotency and history tombstones. |
| `CLEANUP_COMPANY_ANALYSES_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for generated company analysis text. |
| `CLEANUP_CONTEXT_SUMMARY_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Independent retention for compacted agent context summaries. |
| `CLEANUP_ENABLED` | bool | true | —; true \| false | Enable ESB stale-call recovery and retention cleanup loops, including immediate cleanup after terminal calls. |
| `CLEANUP_FACTS_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for ClickHouse universal facts. |
| `CLEANUP_FACT_COMPANY_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for ClickHouse fact-to-company links. |
| `CLEANUP_MESSAGES_RETENTION_KESHA` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for Kesha conversation messages. |
| `CLEANUP_MESSAGES_RETENTION_KONSTANTIN` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for Konstantin chat history exposed to Domain. |
| `CLEANUP_MESSAGES_RETENTION_SAVELIY` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for Saveliy conversation messages. |
| `CLEANUP_MESSAGES_RETENTION_SEREZHA` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for Serezha conversation messages. |
| `CLEANUP_MESSAGES_RETENTION_VALERA` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for Valera conversation messages. |
| `CLEANUP_METRIC_OBSERVATIONS_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for metric observations. |
| `CLEANUP_METRIC_TOTALS_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for calculated metric totals. |
| `CLEANUP_NEWS_MARKER_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for processing markers only when no raw text or analytic rows depend on them. |
| `CLEANUP_NEWS_METADATA_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for unreferenced ClickHouse news metadata; metadata required by retained facts or observations is preserved. |
| `CLEANUP_PROCESSED_NEWS_TEXT_RETENTION` | duration \| immediate \| never | 168h | duration; 0 \| immediate \| positive Go duration \| never | Retention after ClickHouse confirms news processing and persisted facts; this age starts at the processing marker. |
| `CLEANUP_QUEUED_STALE_AFTER` | duration | 10m0s | duration; 1ms…8760h0m0s | Age after which a submitting call without a saved Hermes run ID can be safely returned to the queue. |
| `CLEANUP_RETENTION_CHECK_INTERVAL` | duration | 24h0m0s | interval; 1ms…8760h0m0s | Interval between bounded PostgreSQL and ClickHouse retention passes. |
| `CLEANUP_RUNNING_STALE_AFTER` | duration | 30m0s | duration; 1ms…8760h0m0s | Minimum age before cleanup independently checks the Hermes status of an active call. |
| `CLEANUP_STATUS_CHECK_INTERVAL` | duration | 1h0m0s | interval; 1ms…8760h0m0s | Interval between stale-call reconciliation passes; kept separate from data-retention cleanup. |
| `CLEANUP_UNPROCESSED_NEWS_TEXT_RETENTION` | duration \| immediate \| never | never | duration; 0 \| immediate \| positive Go duration \| never | Retention for raw text only when its news call is terminal and the text is not marked processed. |
| `ATLAS_TOKEN` | string | empty / unset | —; not validated | Optional Atlas token used by the ClickHouse Atlas driver. |
| `CLICKHOUSE_PASSWORD` | string | required | —; not validated | ClickHouse account password. |
| `DATABASE_URL` | string | required | —; not validated | PostgreSQL connection URL used only by ESB. |
| `ESB_API_KEY` | string | required | —; not validated | Bearer key for Domain HTTP and WebSocket integration; it does not authorize MCP tools. |
| `HERMES_API_KEY` | string | required | —; not validated | Bearer key ESB uses for Hermes runtime calls. |
| `LLM_API_KEY` | string | required | —; not validated | LLM provider key consumed by Hermes, never by ESB. |
| `MCP_TOKEN_KESHA` | string | required | bytes; 32…256 | Agent-only Bearer token for the ESB MCP endpoint; unique and separate from integration keys. |
| `MCP_TOKEN_KONSTANTIN` | string | required | bytes; 32…256 | Agent-only Bearer token for the ESB MCP endpoint; unique and separate from integration keys. |
| `MCP_TOKEN_SAVELIY` | string | required | bytes; 32…256 | Agent-only Bearer token for the ESB MCP endpoint; unique and separate from integration keys. |
| `MCP_TOKEN_SEREZHA` | string | required | bytes; 32…256 | Agent-only Bearer token for the ESB MCP endpoint; unique and separate from integration keys. |
| `MCP_TOKEN_VALERA` | string | required | bytes; 32…256 | Agent-only Bearer token for the ESB MCP endpoint; unique and separate from integration keys. |
| `MINIO_ROOT_PASSWORD` | string | placeholder в `predictor/example.env` | —; not validated | Local MinIO root password; replace for private deployments. |
| `MINIO_ROOT_USER` | string | placeholder в `predictor/example.env` | —; not validated | Local MinIO root account. |
| `POSTGRES_PASSWORD` | string | placeholder в `predictor/example.env` | —; not validated | Local Compose PostgreSQL password; replace for private deployments. |
| `S3_ACCESS_KEY_ID` | string | empty / unset | —; not validated | Optional static S3 access key; pair with S3_SECRET_ACCESS_KEY. |
| `S3_SECRET_ACCESS_KEY` | string | empty / unset | —; not validated | Optional static S3 secret key; pair with S3_ACCESS_KEY_ID. |
| `CLICKHOUSE_HEALTHCHECK_INTERVAL` | duration | 5s | duration; 1ms…1h0m0s | ClickHouse container healthcheck interval in Compose. |
| `CLICKHOUSE_HEALTHCHECK_RETRIES` | integer | 20 | retries; 1…100 | Consecutive ClickHouse healthcheck failures before the container is unhealthy. |
| `CLICKHOUSE_HEALTHCHECK_TIMEOUT` | duration | 3s | duration; 1ms…1h0m0s | ClickHouse container healthcheck command timeout in Compose. |
| `ESB_HEALTHCHECK_INTERVAL` | duration | 10s | duration; 1ms…1h0m0s | ESB container healthcheck interval in Compose. |
| `ESB_HEALTHCHECK_RETRIES` | integer | 20 | retries; 1…100 | Consecutive ESB healthcheck failures before the container is unhealthy. |
| `ESB_HEALTHCHECK_TIMEOUT` | duration | 3s | duration; 1ms…1h0m0s | ESB container healthcheck command timeout in Compose. |
| `ESB_HOST_PORT` | integer | 8081 | port; 1…65535 | Host port published for the ESB HTTP service. |
| `MINIO_API_HOST_PORT` | integer | 9000 | port; 1…65535 | Host port published for the MinIO S3 API. |
| `MINIO_CONSOLE_HOST_PORT` | integer | 9001 | port; 1…65535 | Host port published for the MinIO console. |
| `MINIO_HEALTHCHECK_INTERVAL` | duration | 3s | duration; 1ms…1h0m0s | MinIO container healthcheck interval in Compose. |
| `MINIO_HEALTHCHECK_RETRIES` | integer | 30 | retries; 1…100 | Consecutive MinIO healthcheck failures before the container is unhealthy. |
| `MINIO_HEALTHCHECK_TIMEOUT` | duration | 2s | duration; 1ms…1h0m0s | MinIO container healthcheck command timeout in Compose. |
| `POSTGRES_DB` | string | predictor | —; not validated | Database name for the PostgreSQL Compose service. |
| `POSTGRES_HEALTHCHECK_INTERVAL` | duration | 5s | duration; 1ms…1h0m0s | PostgreSQL container healthcheck interval in Compose. |
| `POSTGRES_HEALTHCHECK_RETRIES` | integer | 20 | retries; 1…100 | Consecutive PostgreSQL healthcheck failures before the container is unhealthy. |
| `POSTGRES_HEALTHCHECK_TIMEOUT` | duration | 3s | duration; 1ms…1h0m0s | PostgreSQL container healthcheck command timeout in Compose. |
| `POSTGRES_USER` | string | predictor | —; not validated | Account name for the PostgreSQL Compose service. |
| `ESB_E2E_BASE_URL` | string | http://127.0.0.1:18081 | —; not validated | Test-only base URL for the ESB E2E suite. |
| `MOCK_PORT` | string | 8090 | —; not validated | Test-only listening port for the mock Hermes runtime. |

Длительности используют единицы Go `ns`, `us`/`µs`, `ms`, `s`, `m`, `h` (день — `24h`). Для retention `0`, `0s` и `immediate` означают очистку после подтверждения операции, `never` — хранение без срока. Политики очистки выполняются при `CLEANUP_ENABLED=true`.

## Настройки агентов

У всех агентов общий `HERMES_MODEL_PROVIDER` и `LLM_API_KEY`. Модель и runtime-лимиты задаются в env отдельно для агента. Для MCP задайте пять различных `MCP_TOKEN_<TYPE>`; ESB определяет роль по токену, а не по query-параметрам или аргументам модели. `ESB_API_KEY` предназначен для Domain HTTP/WebSocket и не даёт доступ к MCP. Ролевой allowlist MCP находится в `agent.json` и проверяется серверной политикой ESB; Hermes `tools.include` только повторяет этот allowlist. Остальная конфигурация находится в `predictor/agents/<type>/hermes.yaml`, роль — в `skills/`.

`HERMES_<TYPE>_INSTANCES` задаёт runtime-пул в формате `id=state@url;id=state@url`; состояния: `available`, `draining`, `unavailable`. `AGENT_<TYPE>_MAX_CONCURRENT_RUNS` — лимит одновременных вызовов на один runtime. Все ESB-процессы должны использовать одинаковую конфигурацию пула. PostgreSQL сохраняет runtime каждого вызова и координирует резервирование слотов между ESB-репликами. Удаляйте runtime ID только после завершения закреплённых за ним вызовов. `HERMES_<TYPE>_URL` поддерживается для совместимости с одиночным runtime.

`context_policy.max_history_messages=0` отключает передачу истории модели. Хранение полной истории и резюме PostgreSQL задаётся независимо через `CLEANUP_MESSAGES_RETENTION_<TYPE>` и `CLEANUP_CONTEXT_SUMMARY_RETENTION`.
