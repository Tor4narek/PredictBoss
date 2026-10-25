# PredictBoss

Каркас асинхронной новостной аналитики на Hermes, ESB, PostgreSQL, ClickHouse и S3. ESB — единственный сервис, подключённый к хранилищам: он принимает `/call`, хранит очередь и чаты в PostgreSQL, передаёт задачи Hermes и предоставляет один MCP endpoint для ClickHouse и S3. Hermes обращается к данным только через MCP ESB.

## Архитектура ESB: четыре слоя DDD

Зависимости направлены внутрь: `presentation → application → domain`. Адаптеры `infrastructure` реализуют порты domain. `cmd/esb` — только composition root, который собирает слои и запускает серверы.

- `internal/domain` — типы предметной области, статусы вызова, факты, метрики и интерфейсы репозиториев/runtime.
- `internal/application` — сценарии создания и обработки вызова, асинхронный worker, аналитические операции и проверки инвариантов.
- `internal/infrastructure` — адаптеры PostgreSQL, Hermes API, ClickHouse HTTP и S3 API.
- `internal/presentation` — HTTP `/call` и единый MCP endpoint `/mcp` для аналитики ClickHouse, профилей S3 и настроек метрик PostgreSQL.

PostgreSQL хранит очередь вызовов, сообщения истории и отдельные чаты по паре `agent_type + scope_id`. Один чат выполняется последовательно; разные чаты могут обрабатываться параллельно. Ограничение параллельных запусков Hermes равно `0` (без лимита на уровне runtime).

PostgreSQL также является источником истины для настроек метрик компании: `company_metric_settings` хранит `company_id`, `metric_id`, имя, описание, enum `metric_type`, характеристику, единицу измерения и дополнительные JSON-параметры. ESB атомарно upsert-ит настройки в PostgreSQL. Валера сохраняет профиль и настройки одним MCP-вызовом; при частичном сбое тот же вызов безопасно повторить.

## Тип агента — Hermes skill

Роль агента описана в `SKILL.md`. ESB загружает этот skill как инструкции задачи, а Hermes одновременно видит его как внешний skill. Для каждого типа есть отдельный runtime, конфигурация Hermes и allowlist MCP-инструментов.

- **Серёжа (`serezha`)** — принимает news id, source id и текст; сохраняет новость и извлечённые универсальные факты в ClickHouse.
- **Савелий (`saveliy`)** — читает Markdown-профиль компании через ESB и связывает существующие факты с компанией.
- **Валера (`valera`)** — принимает JSON компании, формирует `profile.md` в S3 и сохраняет определения метрик и характеристики компании в PostgreSQL через ESB.
- **Кеша (`kesha`)** — читает профиль, факты и настройки метрик через ESB; сохраняет наблюдения, вероятностный анализ и итоги метрик в ClickHouse.

Исходные материалы ролей находятся в `predictor/agents/{serezha,saveliy,valera,kesha}/`: `agent.json`, `hermes.yaml`, `skills/<slug>/SKILL.md` и `references/data-contract.md`. `agent.json` задаёт область независимого чата, обязательные поля и skill, `hermes.yaml` — модель, runtime и список доступных MCP-инструментов.

## ClickHouse

ClickHouse хранит аналитические данные:

- `news` — исходные очищенные новости и идентификатор источника;
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

На новом PostgreSQL volume bootstrap-схема и настройки метрик применяются из `predictor/storage/postgres/init/`. Для уже существующего PostgreSQL volume примените `predictor/storage/postgres/migrations/20261025110000_company_metric_settings.sql` вручную перед обновлением ESB.

Снаружи доступен только ESB на `localhost:8081`. PostgreSQL и ClickHouse находятся во внутренней storage-сети Docker; для администрирования используйте `docker compose exec`. `/mcp` — единый endpoint ESB для агентов; отдельного S3 MCP-сервиса нет.

## Единый endpoint `/call`

`POST /call?agent=<тип>` создаёт асинхронный вызов. Тип агента передаётся только в query-параметре; данные задачи идут JSON-объектом в теле. ESB отвечает `202 Accepted` с `call_id` и текущим статусом. `Idempotency-Key` необязателен.

Пример входа Серёжи:

```bash
curl -X POST 'http://localhost:8081/call?agent=serezha' \
  -H "Authorization: Bearer $ESB_API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: wire-9-news-123-v1' \
  -d '{"news_id":"news-123","source_id":"wire-9","source_url":"https://example.org/news/123","news_text":"Очищенный текст новости"}'
```

Для Валеры тело содержит `company_id` и `company_data` — объект исходных данных компании. Савелию передаются `company_id` и `news_id`, а при наличии можно приложить факты. Кеше передаётся `company_id` и, при необходимости, период анализа.

Пример запроса Валере:

```bash
curl -X POST 'http://localhost:8081/call?agent=valera' \
  -H "Authorization: Bearer $ESB_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"company_id":"company-42","company_data":{"name":"Пример","industry":"retail","products":["..."],"metrics":[{"metric_id":"revenue","name":"Выручка","metric_type":"currency","unit":"RUB","characteristic":"Месячная выручка компании"}]}}'
```

Савелий принимает `{"company_id":"...","news_id":"..."}` и может получить факты из ClickHouse через MCP ESB. Для передачи фактов в теле можно добавить `facts`. Кеша принимает `{"company_id":"...","period_from":"YYYY-MM-DD","period_to":"YYYY-MM-DD"}`.

`GET /call?agent=<тип>` использует JSON-тело. Передайте `call_id`, чтобы получить состояние асинхронного вызова, либо `company_id`, чтобы получить отчёт Кеши. Для отчёта можно указать `period_from` и `period_to` в формате `YYYY-MM-DD`.

```bash
curl --request GET 'http://localhost:8081/call?agent=kesha' \
  -H "Authorization: Bearer $ESB_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"company_id":"company-42","period_from":"2026-01-01","period_to":"2026-10-25"}'
```

Проверка статуса запуска:

```bash
curl --request GET 'http://localhost:8081/call?agent=serezha' \
  -H "Authorization: Bearer $ESB_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"call_id":"<call_id из ответа POST>"}'
```

`GET /healthz` открыт для liveness. `/readyz`, `/call` и единый `/mcp` требуют `Authorization: Bearer $ESB_API_KEY`.
