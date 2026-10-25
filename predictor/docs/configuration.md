<!-- Generated from the typed ESB environment setting catalog. Do not edit by hand. -->
# Конфигурация Predictor

ESB загружает настройки из переменных окружения при старте. Файл `predictor/.env` не попадает в Git. Для Compose из корня репозитория передавайте его явно: `docker compose --env-file predictor/.env up -d`. Прямой `go run` не загружает env-файл автоматически.

## Переменные окружения

Диапазоны для длительностей включают обе границы. Поддерживаются единицы Go duration `ns`, `us`/`µs`, `ms`, `s`, `m`, `h`; день указывайте как `24h`, неделю как `168h`. Для retention-политик `0`, `0s` или `immediate` означает очистку сразу после завершения вызова и проверки зависимостей; положительная длительность задаёт возраст, `never` отключает очистку категории. Немедленные и периодические политики выполняются только при `CLEANUP_ENABLED=true`. Отрицательные значения запрещены. Пустые optional string используют default; пустые required значения вызывают ошибку запуска.

| Имя | Тип | Default | Допустимый диапазон | Единица | Назначение | Категория |
| --- | --- | --- | --- | --- | --- | --- |
| `AGENTS_DIR` | string | ../agents | not validated | — | Directory containing agent.json, skills, and Hermes configuration. | runtime ESB |
| `CLICKHOUSE_MAX_RESPONSE_BYTES` | integer (int64) | 4194304 | 1024…67108864 | bytes | Maximum ClickHouse response body size accepted by ESB. | runtime ESB |
| `CLICKHOUSE_REQUEST_TIMEOUT` | duration | 45s | 1ms…1h0m0s | duration | HTTP client timeout for ClickHouse requests. | runtime ESB |
| `COMPANY_ITEMS_DEFAULT_PAGE_SIZE` | integer | 100 | 1…100000 | items | Default page size for company fact and metric lists. | runtime ESB |
| `COMPANY_ITEMS_MAX_PAGE_SIZE` | integer | 500 | 1…100000 | items | Maximum page size for company fact and metric lists. | runtime ESB |
| `COMPANY_REPORT_ANALYSIS_LIMIT` | integer | 100 | 1…100000 | analyses | Maximum stored company analyses included in one report. | runtime ESB |
| `COMPANY_REPORT_DEFAULT_WINDOW_DAYS` | integer | 30 | 1…3650 | days | Default lookback period for company reports when dates are omitted. | runtime ESB |
| `COMPANY_REPORT_FACTS_LIMIT` | integer | 200 | 1…100000 | facts | Maximum company-linked facts included in one report. | runtime ESB |
| `CONSULTANT_CONTEXT_DEFAULT_PAGE_SIZE` | integer | 100 | 1…100000 | items | Default fact/observation page size in real-time consultant context. | runtime ESB |
| `CONSULTANT_CONTEXT_MAX_PAGE_SIZE` | integer | 500 | 1…100000 | items | Maximum fact/observation page size in real-time consultant context. | runtime ESB |
| `CONTEXT_COMPACTION_TIMEOUT` | duration | 30s | 1ms…1h0m0s | duration | Maximum time spent requesting a compressed agent-context summary from Hermes. | runtime ESB |
| `FACTS_DEFAULT_PAGE_SIZE` | integer | 100 | 1…100000 | facts | Number of facts returned when a facts query omits limit. | runtime ESB |
| `FACTS_MAX_PAGE_SIZE` | integer | 100 | 1…100000 | facts | Maximum facts returned by one paged facts query. | runtime ESB |
| `FACTS_MAX_PERIOD_DAYS` | integer | 90 | 1…3650 | days | Maximum allowed time span for a filtered facts query. | runtime ESB |
| `HERMES_HEALTH_TIMEOUT` | duration | 2s | 1ms…1m0s | duration | Per-agent readiness probe timeout. | runtime ESB |
| `HERMES_KESHA_URL` | string | required | not validated | — | Kesha Hermes runtime URL; Compose supplies its service URL. | runtime ESB |
| `HERMES_KONSTANTIN_URL` | string | required | not validated | — | Konstantin Hermes runtime URL; Compose supplies its service URL. | runtime ESB |
| `HERMES_MAX_RESPONSE_BYTES` | integer (int64) | 1048576 | 1024…16777216 | bytes | Maximum response body size accepted from Hermes. | runtime ESB |
| `HERMES_POLL_INTERVAL` | duration | 2s | 1ms…10m0s | interval | Queue dispatch and Hermes status polling interval. | runtime ESB |
| `HERMES_REQUEST_TIMEOUT` | duration | 45s | 1ms…10m0s | duration | HTTP client timeout for Hermes run, status, and health requests. | runtime ESB |
| `HERMES_RETRY_BASE_DELAY` | duration | 1s | 0s…5m0s | duration | Base delay between Hermes submission retries; retry delay grows linearly by attempt. | runtime ESB |
| `HERMES_RETRY_MAX_ATTEMPTS` | integer | 5 | 1…20 | attempts | Total attempts to submit a run to Hermes, including the first attempt. | runtime ESB |
| `HERMES_SAVELIY_URL` | string | required | not validated | — | Saveliy Hermes runtime URL; Compose supplies its service URL. | runtime ESB |
| `HERMES_SEREZHA_URL` | string | required | not validated | — | Serezha Hermes runtime URL; Compose supplies its service URL. | runtime ESB |
| `HERMES_VALERA_URL` | string | required | not validated | — | Valera Hermes runtime URL; Compose supplies its service URL. | runtime ESB |
| `HTTP_ADDR` | string | :8081 | not validated | — | ESB listen address inside its network namespace. | runtime ESB |
| `HTTP_IDLE_TIMEOUT` | duration | 1m0s | 1ms…24h0m0s | duration | HTTP server timeout for idle keep-alive connections. | runtime ESB |
| `HTTP_MAX_JSON_BODY_BYTES` | integer (int64) | 2097152 | 1024…67108864 | bytes | Maximum JSON request body accepted by HTTP endpoints. | runtime ESB |
| `HTTP_READ_HEADER_TIMEOUT` | duration | 5s | 1ms…10m0s | duration | HTTP server timeout for receiving request headers. | runtime ESB |
| `HTTP_SHUTDOWN_TIMEOUT` | duration | 10s | 1ms…10m0s | duration | Grace period for HTTP server shutdown after a termination signal. | runtime ESB |
| `KONSTANTIN_EVENT_POLL_INTERVAL` | duration | 300ms | 1ms…1m0s | interval | Interval for checking PostgreSQL for a completed real-time consultant answer. | runtime ESB |
| `KONSTANTIN_HISTORY_DEFAULT_PAGE_SIZE` | integer | 100 | 1…100000 | messages | Default number of chat-history messages returned by one request. | runtime ESB |
| `KONSTANTIN_HISTORY_DEFAULT_WINDOW_DAYS` | integer | 30 | 1…3650 | days | Default lookback period for Konstantin chat history. | runtime ESB |
| `KONSTANTIN_HISTORY_MAX_PAGE_SIZE` | integer | 100 | 1…100000 | messages | Maximum number of chat-history messages returned by one request. | runtime ESB |
| `KONSTANTIN_HISTORY_MAX_WINDOW_DAYS` | integer | 90 | 1…3650 | days | Maximum time span for Konstantin chat-history requests. | runtime ESB |
| `MAX_COMPANY_METRICS_PER_REQUEST` | integer | 500 | 1…100000 | metrics | Maximum company metric settings accepted in one update. | runtime ESB |
| `MAX_FACTS_PER_NEWS` | integer | 500 | 1…100000 | facts | Maximum facts stored for a single news item in one MCP call. | runtime ESB |
| `MAX_FACT_COMPANY_LINKS_PER_REQUEST` | integer | 1000 | 1…100000 | links | Maximum fact-to-company links stored in one MCP call. | runtime ESB |
| `MAX_METRIC_OBSERVATIONS_PER_REQUEST` | integer | 1000 | 1…100000 | observations | Maximum metric observations stored in one MCP call. | runtime ESB |
| `MAX_NEWS_TEXT_BYTES` | integer (int64) | 1048576 | 1…67108864 | bytes | Maximum raw news text accepted before Hermes processing. | runtime ESB |
| `MAX_QUESTION_BYTES` | integer (int64) | 1048576 | 1…16777216 | bytes | Maximum UTF-8 question text accepted over Konstantin WebSocket. | runtime ESB |
| `QUEUE_DISPATCH_BATCH_SIZE` | integer | 100 | 1…10000 | calls | Maximum queued calls claimed in one dispatch cycle; each claim can start one worker. | runtime ESB |
| `S3_PROFILE_MAX_BYTES` | integer (int64) | 2097152 | 1…67108864 | bytes | Maximum company Markdown profile read from or written to S3. | runtime ESB |
| `STREAM_REPLAY_LIMIT` | integer | 500 | 1…10000 | events | Maximum prior Konstantin events replayed after WebSocket subscription. | runtime ESB |
| `VALERA_PROFILE_SYNC_MAX_RETRIES` | integer | 1 | 0…1 | retries | Maximum automatic retries after Valera output is completed but its profile revision is not confirmed; PostgreSQL retry_count allows 0..1. | runtime ESB |
| `WS_MAX_FRAME_BYTES` | integer (int64) | 1048576 | 1024…16777216 | bytes | Maximum inbound WebSocket frame size. | runtime ESB |
| `WS_OUTGOING_QUEUE_SIZE` | integer | 64 | 1…65536 | frames | Bounded queue of outbound WebSocket frames per connection. | runtime ESB |
| `WS_READ_TIMEOUT` | duration | 0s | 0s…1h0m0s | duration | Maximum idle time waiting for one WebSocket frame; zero keeps subscriptions open indefinitely. | runtime ESB |
| `WS_WRITE_TIMEOUT` | duration | 15s | 1ms…10m0s | duration | Maximum time to write one WebSocket frame. | runtime ESB |
| `CLICKHOUSE_DATABASE` | string | default | not validated | — | ClickHouse database name. | подключение к хранилищу |
| `CLICKHOUSE_URL` | string | http://localhost:8123 | not validated | — | ClickHouse HTTP interface URL. | подключение к хранилищу |
| `CLICKHOUSE_USER` | string | predictor | not validated | — | ClickHouse account used by ESB. | подключение к хранилищу |
| `S3_BUCKET` | string | required | not validated | — | S3 bucket for company Markdown profiles. | подключение к хранилищу |
| `S3_CREATE_BUCKET` | bool | false | true \| false | — | Create the configured S3 bucket during ESB startup when missing. | подключение к хранилищу |
| `S3_ENDPOINT` | string | empty / unset | not validated | — | Optional S3-compatible endpoint; empty selects AWS endpoint discovery. | подключение к хранилищу |
| `S3_FORCE_PATH_STYLE` | bool | false | true \| false | — | Use path-style S3 URLs; typically enabled for MinIO. | подключение к хранилищу |
| `S3_REGION` | string | us-east-1 | not validated | — | S3 signing region. | подключение к хранилищу |
| `AGENT_KESHA_MAX_HISTORY_MESSAGES` | integer | 40 | 0…10000 | messages | Maximum prior messages passed to Kesha's Hermes run; zero disables history. | контекст агента |
| `AGENT_KONSTANTIN_MAX_HISTORY_MESSAGES` | integer | 80 | 0…10000 | messages | Maximum prior messages passed to Konstantin's Hermes run; zero disables history. | контекст агента |
| `AGENT_SAVELIY_MAX_HISTORY_MESSAGES` | integer | 40 | 0…10000 | messages | Maximum prior messages passed to Saveliy's Hermes run; zero disables history. | контекст агента |
| `AGENT_SEREZHA_MAX_HISTORY_MESSAGES` | integer | 40 | 0…10000 | messages | Maximum prior messages passed to Serezha's Hermes run; zero disables history. | контекст агента |
| `AGENT_VALERA_MAX_HISTORY_MESSAGES` | integer | 0 | 0…10000 | messages | Maximum prior messages passed to Valera's Hermes run; zero disables history. | контекст агента |
| `CLEANUP_ACTIVE_MAX_RETRIES` | integer | 1 | 0…32767 | retries | Maximum cleanup retries for calls Hermes confirms failed or interrupted; active or uncertain runs are never retried. | очистка и хранение |
| `CLEANUP_BATCH_SIZE` | integer | 500 | 1…10000 | rows | Maximum candidate rows handled per cleanup category in one pass. | очистка и хранение |
| `CLEANUP_CALL_PAYLOAD_RETENTION` | duration \| immediate \| never | 2160h | 0 \| immediate \| positive Go duration \| never | duration | Age before large request and output fields of terminal calls are cleared; Serezha retry payloads stay available until processing succeeds. | очистка и хранение |
| `CLEANUP_CALL_RECORD_RETENTION` | duration \| immediate \| never | 2160h | 0 \| immediate \| positive Go duration \| never | duration | Age before terminal call rows are deleted when safe or reduced to idempotency and history tombstones. | очистка и хранение |
| `CLEANUP_COMPANY_ANALYSES_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for generated company analysis text. | очистка и хранение |
| `CLEANUP_CONTEXT_SUMMARY_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Independent retention for compacted agent context summaries. | очистка и хранение |
| `CLEANUP_ENABLED` | bool | true | true \| false | — | Enable ESB stale-call recovery and retention cleanup loops, including immediate cleanup after terminal calls. | очистка и хранение |
| `CLEANUP_FACTS_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for ClickHouse universal facts. | очистка и хранение |
| `CLEANUP_FACT_COMPANY_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for ClickHouse fact-to-company links. | очистка и хранение |
| `CLEANUP_MESSAGES_RETENTION_KESHA` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for Kesha conversation messages. | очистка и хранение |
| `CLEANUP_MESSAGES_RETENTION_KONSTANTIN` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for Konstantin chat history exposed to Domain. | очистка и хранение |
| `CLEANUP_MESSAGES_RETENTION_SAVELIY` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for Saveliy conversation messages. | очистка и хранение |
| `CLEANUP_MESSAGES_RETENTION_SEREZHA` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for Serezha conversation messages. | очистка и хранение |
| `CLEANUP_MESSAGES_RETENTION_VALERA` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for Valera conversation messages. | очистка и хранение |
| `CLEANUP_METRIC_OBSERVATIONS_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for metric observations. | очистка и хранение |
| `CLEANUP_METRIC_TOTALS_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for calculated metric totals. | очистка и хранение |
| `CLEANUP_NEWS_MARKER_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for processing markers only when no raw text or analytic rows depend on them. | очистка и хранение |
| `CLEANUP_NEWS_METADATA_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for unreferenced ClickHouse news metadata; metadata required by retained facts or observations is preserved. | очистка и хранение |
| `CLEANUP_PROCESSED_NEWS_TEXT_RETENTION` | duration \| immediate \| never | 168h | 0 \| immediate \| positive Go duration \| never | duration | Retention after ClickHouse confirms news processing and persisted facts; this age starts at the processing marker. | очистка и хранение |
| `CLEANUP_QUEUED_STALE_AFTER` | duration | 10m0s | 1ms…8760h0m0s | duration | Age after which a submitting call without a saved Hermes run ID can be safely returned to the queue. | очистка и хранение |
| `CLEANUP_RETENTION_CHECK_INTERVAL` | duration | 24h0m0s | 1ms…8760h0m0s | interval | Interval between bounded PostgreSQL and ClickHouse retention passes. | очистка и хранение |
| `CLEANUP_RUNNING_STALE_AFTER` | duration | 30m0s | 1ms…8760h0m0s | duration | Minimum age before cleanup independently checks the Hermes status of an active call. | очистка и хранение |
| `CLEANUP_STATUS_CHECK_INTERVAL` | duration | 1h0m0s | 1ms…8760h0m0s | interval | Interval between stale-call reconciliation passes; kept separate from data-retention cleanup. | очистка и хранение |
| `CLEANUP_UNPROCESSED_NEWS_TEXT_RETENTION` | duration \| immediate \| never | never | 0 \| immediate \| positive Go duration \| never | duration | Retention for raw text only when its news call is terminal and the text is not marked processed. | очистка и хранение |
| `ATLAS_TOKEN` | string | empty / unset | not validated | — | Optional Atlas token used by the ClickHouse Atlas driver. | секрет / credential |
| `CLICKHOUSE_PASSWORD` | string | required | not validated | — | ClickHouse account password. | секрет / credential |
| `DATABASE_URL` | string | required | not validated | — | PostgreSQL connection URL used only by ESB. | секрет / credential |
| `ESB_API_KEY` | string | required | not validated | — | Bearer key required by Domain and Hermes MCP clients. | секрет / credential |
| `HERMES_API_KEY` | string | required | not validated | — | Bearer key ESB uses for Hermes runtime calls. | секрет / credential |
| `LLM_API_KEY` | string | required | not validated | — | LLM provider key consumed by Hermes, never by ESB. | секрет / credential |
| `MINIO_ROOT_PASSWORD` | string | локальный placeholder из `predictor/example.env` | not validated | — | Local MinIO root password; replace for private deployments. | секрет / credential |
| `MINIO_ROOT_USER` | string | локальный placeholder из `predictor/example.env` | not validated | — | Local MinIO root account. | секрет / credential |
| `POSTGRES_PASSWORD` | string | локальный placeholder из `predictor/example.env` | not validated | — | Local Compose PostgreSQL password; replace for private deployments. | секрет / credential |
| `S3_ACCESS_KEY_ID` | string | empty / unset | not validated | — | Optional static S3 access key; pair with S3_SECRET_ACCESS_KEY. | секрет / credential |
| `S3_SECRET_ACCESS_KEY` | string | empty / unset | not validated | — | Optional static S3 secret key; pair with S3_ACCESS_KEY_ID. | секрет / credential |
| `CLICKHOUSE_HEALTHCHECK_INTERVAL` | duration | 5s | 1ms…1h0m0s | duration | ClickHouse container healthcheck interval in Compose. | Docker Compose |
| `CLICKHOUSE_HEALTHCHECK_RETRIES` | integer | 20 | 1…100 | retries | Consecutive ClickHouse healthcheck failures before the container is unhealthy. | Docker Compose |
| `CLICKHOUSE_HEALTHCHECK_TIMEOUT` | duration | 3s | 1ms…1h0m0s | duration | ClickHouse container healthcheck command timeout in Compose. | Docker Compose |
| `ESB_HEALTHCHECK_INTERVAL` | duration | 10s | 1ms…1h0m0s | duration | ESB container healthcheck interval in Compose. | Docker Compose |
| `ESB_HEALTHCHECK_RETRIES` | integer | 20 | 1…100 | retries | Consecutive ESB healthcheck failures before the container is unhealthy. | Docker Compose |
| `ESB_HEALTHCHECK_TIMEOUT` | duration | 3s | 1ms…1h0m0s | duration | ESB container healthcheck command timeout in Compose. | Docker Compose |
| `ESB_HOST_PORT` | integer | 8081 | 1…65535 | port | Host port published for the ESB HTTP service. | Docker Compose |
| `MINIO_API_HOST_PORT` | integer | 9000 | 1…65535 | port | Host port published for the MinIO S3 API. | Docker Compose |
| `MINIO_CONSOLE_HOST_PORT` | integer | 9001 | 1…65535 | port | Host port published for the MinIO console. | Docker Compose |
| `MINIO_HEALTHCHECK_INTERVAL` | duration | 3s | 1ms…1h0m0s | duration | MinIO container healthcheck interval in Compose. | Docker Compose |
| `MINIO_HEALTHCHECK_RETRIES` | integer | 30 | 1…100 | retries | Consecutive MinIO healthcheck failures before the container is unhealthy. | Docker Compose |
| `MINIO_HEALTHCHECK_TIMEOUT` | duration | 2s | 1ms…1h0m0s | duration | MinIO container healthcheck command timeout in Compose. | Docker Compose |
| `MINIO_IMAGE` | string | dappros/minio:RELEASE.2025-09-07T16-13-09Z | not validated | — | Image used for the local S3-compatible MinIO service. | Docker Compose |
| `POSTGRES_DB` | string | predictor | not validated | — | Database name for the PostgreSQL Compose service. | Docker Compose |
| `POSTGRES_HEALTHCHECK_INTERVAL` | duration | 5s | 1ms…1h0m0s | duration | PostgreSQL container healthcheck interval in Compose. | Docker Compose |
| `POSTGRES_HEALTHCHECK_RETRIES` | integer | 20 | 1…100 | retries | Consecutive PostgreSQL healthcheck failures before the container is unhealthy. | Docker Compose |
| `POSTGRES_HEALTHCHECK_TIMEOUT` | duration | 3s | 1ms…1h0m0s | duration | PostgreSQL container healthcheck command timeout in Compose. | Docker Compose |
| `POSTGRES_USER` | string | predictor | not validated | — | Account name for the PostgreSQL Compose service. | Docker Compose |
| `ESB_E2E_BASE_URL` | string | http://127.0.0.1:18081 | not validated | — | Test-only base URL for the ESB E2E suite. | только тесты |
| `MOCK_PORT` | string | 8090 | not validated | — | Test-only listening port for the mock Hermes runtime. | только тесты |

`ESB_E2E_BASE_URL` и `MOCK_PORT` относятся только к тестовой среде. `HERMES_*_URL` обязательны для прямого запуска; Compose подставляет URL одноимённых Hermes-сервисов, если переменная не задана.

## Настройки агентов

Роль, порядок работы, политика сжатия истории, модель, reasoning effort, MCP allowlist, инструменты и runtime-лимиты Hermes остаются в `predictor/agents/<agent>/`. `AGENT_<TYPE>_MAX_HISTORY_MESSAGES` ограничивает только контекст Hermes; `0` означает не передавать историю. Отдельные `CLEANUP_MESSAGES_RETENTION_<TYPE>` и `CLEANUP_CONTEXT_SUMMARY_RETENTION` управляют хранением полной истории и резюме в PostgreSQL. По умолчанию история всех агентов хранится бессрочно.

| Файл | Настройки | Кто использует |
| --- | --- | --- |
| `agents/<type>/agent.json` | `scope_field`, `required_fields`, `context_policy` (порог, размер batch сжатия, размер summary и PostgreSQL как место хранения) | ESB: контракт вызова и сжатие контекста модели |
| `agents/<type>/hermes.yaml` | `model.provider/default/base_url/key_env`, `gateway.api_server`, `max_concurrent_runs`, `agent.max_turns`, `reasoning_effort`, disabled toolsets, skill directory, ESB MCP URL, allowlist и режим параллельных вызовов | Hermes: runtime и роль агента |
| `agents/<type>/skills/**` | Skill, порядок анализа, доменные инструкции и ссылки на data contract | Hermes: содержание задачи |

## Инвентаризация остальных настроек

| Источник | Классификация | Что остаётся там | Причина |
| --- | --- | --- | --- |
| `docker-compose.yml` | эксплуатационная настройка — env | host ports, healthcheck интервалы/таймауты/повторы, адреса сервисов и deployment credentials; значения ESB перечислены выше | параметры окружения и развёртывания, меняются без перекомпиляции |
| `predictor/storage/{postgres,clickhouse}/migrations/*_init.sql` | доменный/API-инвариант | таблицы, типы, статусы, enum, ограничения и индексы схемы | начальная схема хранит форму данных; runtime значения в неё не переносятся |
| `predictor/tests/compose.e2e.yml`, `e2e_test.go`, `mock_server.py` | только тестовая конфигурация | тестовые credentials, фиксированные fixture ID, адреса и короткие ожидания | изолированный mock-прогон не использует production secret или LLM; `ESB_E2E_BASE_URL` и `MOCK_PORT` перечислены выше |
| `agents/<type>/agent.json` | роль/контекст агента | область чата, обязательные поля и политика сжатия контекста | хранение полной истории задаётся отдельно через `CLEANUP_*_RETENTION` |
| `agents/<type>/hermes.yaml` | runtime-настройка агента | модель, reasoning, max turns, MCP allowlist и `max_concurrent_runs` | настройки исполнения роли принадлежат Hermes; `0` в текущем runtime означает отсутствие per-agent concurrency cap |
| очередь ESB | неизменяемая архитектурная граница | отдельного глобального concurrency cap нет; размер `QUEUE_DISPATCH_BATCH_SIZE` ограничивает число задач, извлекаемых за цикл | параллельное исполнение остаётся под контролем каждого Hermes runtime и ресурсов развёртывания |

## Фиксированные инварианты

Эти значения задают смысл контракта или безопасную валидацию, поэтому не становятся deployment env:

| Инвариант | Почему фиксирован |
| --- | --- |
| Типы агентов (`serezha`, `saveliy`, `valera`, `kesha`, `konstantin`), поля области и маршруты `/call`, `/stream`, `/mcp`, health routes | Идентифицируют компоненты и публичный контракт |
| Статусы вызовов и роли сообщений | Состояния конечного автомата и ограничения схемы PostgreSQL |
| `retry_count` Валеры в PostgreSQL: 0…1 | `VALERA_PROFILE_SYNC_MAX_RETRIES` имеет диапазон 0…1 и default 1; верхняя граница обязана совпадать с init CHECK constraint |
| Каталог метрик, enum типа метрики, виды материалов и медиа | Бизнес-словарь; хранится в коде/схеме, а не в deployment env |
| Идентификаторы: строковый ключ до 128 символов, UUID там, где контракт требует UUID; вероятности в диапазоне 0…1 | Проверка формата и математический смысл входных данных |
| `integerValue` MCP ограничен 1 000 000 | Общая защита от некорректных целочисленных аргументов; конкретные list limits дополнительно применяют env-границы |
| Конкурентное сжатие истории повторяет compare-and-swap не более 16 раз | Ограниченный внутренний цикл защищает от бесконечной гонки; при исчерпании ESB использует обычную историю |
| HTTP body резервирует 4 KiB под JSON metadata сверх `MAX_NEWS_TEXT_BYTES`; диагностический ответ ClickHouse при `INSERT` ограничен 4 KiB | Небольшие фиксированные границы для структуры запроса и текста ошибки; успешные ответы ограничиваются `CLICKHOUSE_MAX_RESPONSE_BYTES` |
| `EventsAfter` принимает 1…10 000 событий и использует fallback 100 для внутреннего неверного аргумента | Защитная валидация адаптера; публичный WS replay отдельно ограничен `STREAM_REPLAY_LIMIT` |
| Схема PostgreSQL и ClickHouse в `storage/*/migrations/*_init.sql` | Init описывает состояние данных, а не runtime-параметры |
| Очистка ClickHouse удаляет строки через `mutations_sync = 2` и проверяет `FINAL`-состояние | Асинхронное применение мутаций не должно приводить к ложному подтверждению удаления |

## Безопасность очистки

ESB запускает отдельные циклы сверки зависших вызовов и очистки по сроку хранения. PostgreSQL advisory lock не допускает параллельное выполнение одного прохода несколькими экземплярами. Все запросы ограничены `CLEANUP_BATCH_SIZE`; ошибки повтора безопасны, а ошибки Hermes/хранилищ не считаются подтверждением удаления. Активные вызовы и вызовы с неустановленным статусом остаются в PostgreSQL.

Текст обработанной новости в ClickHouse хранится `CLEANUP_PROCESSED_NEWS_TEXT_RETENTION` после подтверждения маркера, метаданных и числа фактов. `0s`/`immediate` удаляет его после подтверждённой обработки; этот сигнал также запускает очистку сразу после терминального состояния вызова. Политика необработанного текста ClickHouse применяется только к новости с терминальным вызовом Серёжи. Необработанный `news_text` в PostgreSQL сохраняется, пока вызов остаётся повторяемым; очистка не удаляет этот payload только из-за срока хранения вызовов. Метаданные, нужные сохранённым фактам или наблюдениям, остаются до снятия зависимостей; факты сохраняются, пока на них ссылаются связи с компаниями или наблюдения. Порядок очистки сначала удаляет зависимые связи и наблюдения, а затем факты и метаданные. Сообщения, резюме, payload и запись вызова используют независимые сроки хранения. Немедленная очистка `CLEANUP_CALL_PAYLOAD_RETENTION` удаляет вывод, который вызывающий клиент ещё мог бы запросить; немедленная очистка сообщений Константина удаляет доступную Domain историю. Установите для этих категорий положительный срок или `never`, если чтение нужно после завершения. История Константина и аналитические данные по умолчанию не удаляются. Профили компаний и текущие настройки метрик очистка не затрагивает.

Единица измерения метрики компании (`unit`) — данные конкретной компании, а не переменная процесса. Валера записывает её в `company_metric_settings` PostgreSQL через MCP ESB; агрегаты и отчёты возвращают единицу из PostgreSQL. ClickHouse хранит только `company_id`, `metric_id` и аналитические значения. Допустимы значения домена: `₽`, `%`, `тонн/месяц`, `USD/баррель` и другие строки, смысл которых задаётся профилем компании.

## Прямой запуск ESB

Скопируйте `predictor/example.env` в `predictor/.env`, задайте credentials и запустите `docker compose --env-file predictor/.env up -d` из корня репозитория. При прямом запуске Go-процесса экспортируйте переменные в оболочке/IDE; бинарник намеренно не загружает env-файл неявно. Укажите `AGENTS_DIR` и пять `HERMES_<TYPE>_URL` адресов, доступных из процесса.
