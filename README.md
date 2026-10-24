# PredictBoss

Стартовый каркас аналитической системы:

- `predictor/agents/hermes` — базовая конфигурация Hermes-агента;
- `predictor/esb` — Go-проект ESB;
- `predictor/storage` — Atlas-конфигурация и все ClickHouse-миграции;
- `predictor/storage/atlas.hcl` и `predictor/storage/migrations` — миграции ClickHouse через Atlas.

## Локальный запуск

Запустить ESB и Hermes вместе:

```bash
docker compose up --build
```

После запуска ClickHouse доступен на `localhost:8123`, ESB — на `localhost:8081`, а Hermes подключается к ESB по адресу `http://esb:8081`.

Применить Atlas-миграции в Compose (для ClickHouse нужен `ATLAS_TOKEN`):

```bash
ATLAS_TOKEN=... docker compose --profile migrations run --rm atlas
```

Запустить ESB Go-проект:

```bash
cd predictor/esb
go run ./cmd/esb
```

Применить миграции к локальному ClickHouse с установленным Atlas:

```bash
cd predictor/storage
ATLAS_DATABASE_URL=clickhouse://localhost:9000/default atlas migrate apply --env local
```
