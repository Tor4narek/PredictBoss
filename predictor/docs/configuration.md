<!-- Generated from the typed ESB environment setting catalog. Do not edit by hand. -->
# Конфигурация Predictor

ESB загружает конфигурацию один раз при старте из унаследованных переменных окружения. Compose автоматически читает `.env` из каталога проекта и передаёт значения сервисам. Прямой `go run` не читает `.env`: экспортируйте переменные в оболочке или IDE. Секреты из локального `.env` не включаются в документацию.

## Переменные окружения

Диапазоны для длительностей включают обе границы. Некорректные числа, длительности и bool останавливают запуск; сообщение содержит имя переменной, но не её значение. Ноль допустим только там, где он указан в диапазоне. Пустые optional string используют default; пустые required значения вызывают ошибку запуска.

| Имя | Тип | Default | Допустимый диапазон | Единица | Назначение | Категория |
| --- | --- | --- | --- | --- | --- | --- |
| `AGENT_KESHA_MAX_HISTORY_MESSAGES` | integer | 40 | 0…10000 | messages | Maximum prior messages passed to Kesha's Hermes run; zero disables history. | контекст агента |
| `AGENT_KONSTANTIN_MAX_HISTORY_MESSAGES` | integer | 80 | 0…10000 | messages | Maximum prior messages passed to Konstantin's Hermes run; zero disables history. | контекст агента |
| `AGENT_SAVELIY_MAX_HISTORY_MESSAGES` | integer | 40 | 0…10000 | messages | Maximum prior messages passed to Saveliy's Hermes run; zero disables history. | контекст агента |
| `AGENT_SEREZHA_MAX_HISTORY_MESSAGES` | integer | 40 | 0…10000 | messages | Maximum prior messages passed to Serezha's Hermes run; zero disables history. | контекст агента |
| `AGENT_VALERA_MAX_HISTORY_MESSAGES` | integer | 0 | 0…10000 | messages | Maximum prior messages passed to Valera's Hermes run; zero disables history. | контекст агента |
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
| `HISTORY_RETENTION_CLEANUP_INTERVAL` | duration | 24h0m0s | 1s…8760h0m0s | duration | Interval between PostgreSQL history-retention cleanup passes. | runtime ESB |
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
| `ATLAS_TOKEN` | string | empty / unset | not validated | — | Optional Atlas token used by the ClickHouse Atlas driver. | секрет / credential |
| `CLICKHOUSE_PASSWORD` | string | required | not validated | — | ClickHouse account password. | секрет / credential |
| `DATABASE_URL` | string | required | not validated | — | PostgreSQL connection URL used only by ESB. | секрет / credential |
| `ESB_API_KEY` | string | required | not validated | — | Bearer key required by Domain and Hermes MCP clients. | секрет / credential |
| `HERMES_API_KEY` | string | required | not validated | — | Bearer key ESB uses for Hermes runtime calls. | секрет / credential |
| `LLM_API_KEY` | string | required | not validated | — | LLM provider key consumed by Hermes, never by ESB. | секрет / credential |
| `MINIO_ROOT_PASSWORD` | string | локальный placeholder из `.env.example` | not validated | — | Local MinIO root password; replace for private deployments. | секрет / credential |
| `MINIO_ROOT_USER` | string | локальный placeholder из `.env.example` | not validated | — | Local MinIO root account. | секрет / credential |
| `POSTGRES_PASSWORD` | string | локальный placeholder из `.env.example` | not validated | — | Local Compose PostgreSQL password; replace for private deployments. | секрет / credential |
| `S3_ACCESS_KEY_ID` | string | empty / unset | not validated | — | Optional static S3 access key; pair with S3_SECRET_ACCESS_KEY. | секрет / credential |
| `S3_SECRET_ACCESS_KEY` | string | empty / unset | not validated | — | Optional static S3 secret key; pair with S3_ACCESS_KEY_ID. | секрет / credential |
| `CLICKHOUSE_DATABASE` | string | default | not validated | — | ClickHouse database name. | подключение к хранилищу |
| `CLICKHOUSE_URL` | string | http://localhost:8123 | not validated | — | ClickHouse HTTP interface URL. | подключение к хранилищу |
| `CLICKHOUSE_USER` | string | predictor | not validated | — | ClickHouse account used by ESB. | подключение к хранилищу |
| `S3_BUCKET` | string | required | not validated | — | S3 bucket for company Markdown profiles. | подключение к хранилищу |
| `S3_CREATE_BUCKET` | bool | false | true \| false | — | Create the configured S3 bucket during ESB startup when missing. | подключение к хранилищу |
| `S3_ENDPOINT` | string | empty / unset | not validated | — | Optional S3-compatible endpoint; empty selects AWS endpoint discovery. | подключение к хранилищу |
| `S3_FORCE_PATH_STYLE` | bool | false | true \| false | — | Use path-style S3 URLs; typically enabled for MinIO. | подключение к хранилищу |
| `S3_REGION` | string | us-east-1 | not validated | — | S3 signing region. | подключение к хранилищу |
| `ESB_E2E_BASE_URL` | string | http://127.0.0.1:18081 | not validated | — | Test-only base URL for the ESB E2E suite. | только тесты |
| `MOCK_PORT` | string | 8090 | not validated | — | Test-only listening port for the mock Hermes runtime. | только тесты |

`ESB_E2E_BASE_URL` и `MOCK_PORT` относятся только к тестовой среде. `HERMES_*_URL` обязательны для прямого запуска; Compose подставляет URL одноимённых Hermes-сервисов, если переменная не задана.

## Настройки агентов

Роль, порядок работы, политика сжатия истории, модель, reasoning effort, MCP allowlist, инструменты и runtime-лимиты Hermes остаются в `predictor/agents/<agent>/`. `AGENT_<TYPE>_MAX_HISTORY_MESSAGES` — отдельное эксплуатационное исключение: предел сообщений для контекста Hermes задан через env, `0` означает не передавать историю. История PostgreSQL продолжает управляться `history_retention_days` из `agent.json`.

| Файл | Настройки | Кто использует |
| --- | --- | --- |
| `agents/<type>/agent.json` | `scope_field`, `required_fields`, `history_retention_days`, `context_policy` (порог, размер batch сжатия, размер summary и PostgreSQL как место хранения) | ESB: правила вызова и хранения контекста агента |
| `agents/<type>/hermes.yaml` | `model.provider/default/base_url/key_env`, `gateway.api_server`, `max_concurrent_runs`, `agent.max_turns`, `reasoning_effort`, disabled toolsets, skill directory, ESB MCP URL, allowlist и режим параллельных вызовов | Hermes: runtime и роль агента |
| `agents/<type>/skills/**` | Skill, порядок анализа, доменные инструкции и ссылки на data contract | Hermes: содержание задачи |

## Инвентаризация остальных настроек

| Источник | Классификация | Что остаётся там | Причина |
| --- | --- | --- | --- |
| `docker-compose.yml` | эксплуатационная настройка — env | host ports, healthcheck интервалы/таймауты/повторы, адреса сервисов и deployment credentials; значения ESB перечислены выше | параметры окружения и развёртывания, меняются без перекомпиляции |
| `predictor/storage/{postgres,clickhouse}/migrations/*_init.sql` | доменный/API-инвариант | таблицы, типы, статусы, enum, ограничения и индексы схемы | начальная схема хранит форму данных; runtime значения в неё не переносятся |
| `predictor/tests/compose.e2e.yml`, `e2e_test.go`, `mock_server.py` | только тестовая конфигурация | тестовые credentials, фиксированные fixture ID, адреса и короткие ожидания | изолированный mock-прогон не использует production secret или LLM; `ESB_E2E_BASE_URL` и `MOCK_PORT` перечислены выше |
| `agents/<type>/agent.json` | роль/контекст агента | область чата, обязательные поля, срок хранения полной истории и политика сжатия | политика применяется отдельно к контексту конкретного агента |
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

## Прямой запуск ESB

Скопируйте `.env.example` в `.env`. `docker compose` подхватит его автоматически. При запуске Go-процесса напрямую задайте нужные переменные в оболочке/IDE (например, через свой менеджер окружения); бинарник намеренно не загружает `.env` неявно. Укажите `AGENTS_DIR` и пять `HERMES_<TYPE>_URL` адресов, доступных из процесса.
