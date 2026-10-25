# PredictBoss

Каркас асинхронной новостной аналитики на Hermes, ESB, PostgreSQL, ClickHouse и S3. ESB — единственный сервис, подключённый к хранилищам: он принимает `/call` и `/news`, хранит очередь и чаты в PostgreSQL, передаёт задачи Hermes и предоставляет единый MCP endpoint для ClickHouse, S3 и настроек метрик PostgreSQL. Hermes не имеет подключений или учётных данных хранилищ.

## Архитектура ESB: четыре слоя DDD

Зависимости направлены внутрь: `presentation → application → domain`. Адаптеры `infrastructure` реализуют порты domain. `cmd/esb` — только composition root, который собирает слои и запускает серверы.

- `internal/domain` — типы предметной области, статусы вызова, факты, метрики и интерфейсы репозиториев/runtime.
- `internal/application` — сценарии создания и обработки вызова, асинхронный worker, аналитические операции и проверки инвариантов.
- `internal/infrastructure` — адаптеры PostgreSQL, Hermes API, ClickHouse HTTP и S3 API.
- `internal/presentation` — HTTP `/call` и единый MCP endpoint `/mcp` для аналитики ClickHouse, профилей S3 и настроек метрик PostgreSQL.

PostgreSQL хранит очередь вызовов, сообщения истории и отдельные чаты по паре `agent_type + scope_id`. Один чат выполняется последовательно; разные чаты могут обрабатываться параллельно. `company_profile_state` хранит принятые ревизии профиля, `company_news_sources` — идемпотентный реестр новых источников, `konstantin_messages` — готовые ответы с доказательствами и курсором для replay. Ограничение параллельных запусков Hermes равно `0` (без лимита на уровне runtime).

PostgreSQL также является источником истины для настроек метрик компании: `company_metric_settings` хранит `company_id`, `metric_id`, имя, описание, enum `metric_type`, характеристику, единицу измерения и дополнительные JSON-параметры. ESB атомарно upsert-ит настройки в PostgreSQL. Валера сохраняет профиль и настройки одним MCP-вызовом; при частичном сбое тот же вызов безопасно повторить.

## Тип агента — Hermes skill

Роль агента описана в `SKILL.md`. ESB загружает этот skill как инструкции задачи, а Hermes одновременно видит его как внешний skill. Для каждого типа есть отдельный runtime, конфигурация Hermes и allowlist MCP-инструментов.

- **Серёжа (`serezha`)** — принимает news id, source id и текст; сохраняет новость и извлечённые универсальные факты в ClickHouse.
- **Савелий (`saveliy`)** — читает Markdown-профиль компании через ESB и связывает существующие факты с компанией.
- **Валера (`valera`)** — принимает JSON компании, формирует `profile.md` в S3 и сохраняет определения метрик и характеристики компании в PostgreSQL через ESB.
- **Кеша (`kesha`)** — читает профиль, факты и настройки метрик через ESB; сохраняет наблюдения, вероятностный анализ и итоги метрик в ClickHouse.
- **Константин (`konstantin`)** — консультирует в реальном времени по профилю и предварительно агрегированным данным ClickHouse; отвечает на вопросы по WebSocket и указывает `sourceId` каждого источника.

Исходные материалы ролей находятся в `predictor/agents/{serezha,saveliy,valera,kesha,konstantin}/`: `agent.json`, `hermes.yaml`, `skills/<slug>/SKILL.md` и `references/data-contract.md`. `agent.json` задаёт область независимого чата, обязательные поля и skill, `hermes.yaml` — модель, runtime и список доступных MCP-инструментов.

## ClickHouse

ClickHouse хранит аналитические данные:

- `news` — очищенный текст новости, `sourceId`, компания, URL, тип материала, заголовок и время публикации;
- `facts` — универсальные факты со ссылкой на новость, доказательством и уверенностью;
- `fact_company` — связь факт–компания;
- `metric_observations` — значение, вероятность, период, `news_id` и подтверждающие fact ids;
- `metric_totals` — итоги по метрике, компании и периоду;
- `company_analyses` — текст анализа и вероятность по периоду.

Наблюдения ссылаются на `company_id` и `metric_id` из настроек PostgreSQL; каталог метрик и характеристики в ClickHouse не дублируются. Итог метрики считается как сумма `value × probability`; записи с `value: null` не входят в количество наблюдений. ClickHouse доступен агентам только через фиксированные MCP-инструменты ESB; произвольный SQL от модели не принимается. Миграции находятся в `predictor/storage/migrations/`, а схема настроек PostgreSQL — в `predictor/storage/postgres/init/`.

## Профили компаний

Профиль хранится как Markdown в S3 по ключу `companies/{company_id}/profile.md`. Гермес получает его через единый MCP endpoint ESB. Валера обновляет профиль вместе с настройками метрик одним идемпотентным вызовом; Савелий и Кеша имеют только чтение. Только ESB содержит S3 credentials. Docker-сети разделены: Hermes видит ESB, а PostgreSQL и ClickHouse находятся в storage-сети, к которой подключён ESB.

## Запуск

Скопируйте `.env.example` в `.env`, задайте API key провайдера в `LLM_API_KEY`, а также отдельные случайные значения `HERMES_API_KEY` и `ESB_API_KEY`. Укажите S3 bucket и параметры доступа, если используется не IAM роль окружения. Credentials PostgreSQL, ClickHouse и S3 передаются только контейнеру ESB.

```bash
cp .env.example .env
docker compose up --build
```

Примените миграции ClickHouse:

```bash
ATLAS_TOKEN=... docker compose --profile migrations run --rm atlas
```

На новом PostgreSQL volume bootstrap-схема и настройки метрик применяются из `predictor/storage/postgres/init/`. На существующем volume примените `predictor/storage/postgres/migrations/20261025130000_konstantin_websocket.sql` вручную перед обновлением ESB. Миграция переводит `company_id` настроек метрик в UUID; существующие значения должны быть UUID.

Снаружи доступен только ESB на `localhost:8081`. PostgreSQL и ClickHouse находятся во внутренней storage-сети Docker; для администрирования используйте `docker compose exec`. `/mcp` — единый endpoint ESB для агентов; отдельного S3 MCP-сервиса нет.

## Контракт Domain ↔ ESB

Все защищённые HTTP-вызовы и WebSocket handshake используют `Authorization: Bearer $ESB_API_KEY`. Полные примеры и схемы: [predictor/docs/esb-contracts.md](predictor/docs/esb-contracts.md).

- `POST /call?agent=<тип>` ставит задачу в очередь и отвечает `202 Accepted` с `call_id`. `GET /call?agent=<тип>` с JSON-телом читает статус/отчёт; для Константина тело `{"companyId":"<uuid>"}` возвращает готовый актуальный анализ либо `202 processing`, `conversationId`, `requestId` и `streamUrl`.
- `POST /news` принимает `{"sourceId":"<uuid>","text":"..."}` (не более 1 MiB). ESB с bearer-токеном из `DOMAIN_API_TOKEN` читает `/internal/news-links/{sourceId}`, фиксирует компанию и источник в PostgreSQL и идемпотентно запускает Серёжу.
- `GET /stream?companyId=...&conversationId=...` — WebSocket только Константина. `GET /call?agent=konstantin` возвращает адрес подписки. Клиент отправляет `question` кадры; ESB отвечает `accepted`, затем `assistant_answer` или готовым `analysis_result`. Сообщения и доказательства сначала сохраняются в PostgreSQL. Параметр `afterMessageId` восстанавливает пропущенные кадры при переподключении.
- `POST /call?agent=konstantin` запускает анализ асинхронно по `{companyId}` и возвращает `202` с `call_id`. Для получения состояния и адреса подписки вызовите `GET /call?agent=konstantin` с тем же `companyId`; вопросы отправляются только кадрами WebSocket `/stream`.
- Повтор профиля с тем же `(companyId, profileRevision)` безопасен; более старая ревизия отвергается. Новый `sourceId` либо ревизия профиля делает прежний анализ неактуальным.

Адрес Domain API настраивается через `DOMAIN_API_BASE_URL`, сервисный bearer — через `DOMAIN_API_TOKEN`. При пустых значениях приложение запускается, но `POST /news` отвечает `503`.

`GET /healthz` открыт для liveness. `/readyz`, `/call`, `/news`, `/stream` и единый `/mcp` требуют `Authorization: Bearer $ESB_API_KEY`.
