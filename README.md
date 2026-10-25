# PredictBoss

Асинхронная система новостной аналитики. ESB принимает вызовы Domain, ведёт очередь и чаты в PostgreSQL, запускает агентов Hermes и предоставляет им единый MCP endpoint для аналитических данных и профилей компаний.

## Документация

- [Архитектура и границы компонентов](predictor/docs/architecture.md)
- [Подключение Domain к ESB](predictor/docs/integration.md)
- [Контракты HTTP, WebSocket и MCP](predictor/docs/api/README.md)
- [Спецификация E2E](predictor/tests/SPEC.md)

OpenAPI, AsyncAPI и каталог MCP генерируются из Go-кода и зарегистрированных инструментов. Не редактируйте файлы в `predictor/docs/api/` вручную; выполните `make docs` после изменения контракта.

## Быстрый запуск

Нужны Docker Compose и Go 1.24. Скопируйте `.env.example` в `.env` и задайте локальные секреты. URL Hermes внутри Compose уже настроены:

Для новой базы примените Atlas baseline-миграции перед запуском ESB:

```sh
cp .env.example .env
docker compose up -d postgres clickhouse
docker compose --profile migrations run --rm atlas-postgres
# Для ClickHouse через Atlas нужен ATLAS_TOKEN.
docker compose --profile migrations run --rm atlas-clickhouse
docker compose up --build
```

Миграции — начальные схемы `CREATE` в `predictor/storage/postgres/` и `predictor/storage/clickhouse/`. Это новая baseline-история; не применяйте её поверх существующей базы со старой историей Atlas без сверки версии и текущей схемы. Существующие данные и volumes автоматически не удаляются.

## Разработка

```sh
make build       # собрать ESB
cd predictor/esb && go test ./... && cd ../..
make docs        # обновить сгенерированные API-контракты
make docs-check  # проверить, что контракты не устарели
./predictor/tests/run-e2e.sh
```

Роли агентов, `SKILL.md` и конфигурации Hermes находятся в `predictor/agents/`. Ручные архитектурные решения поддерживаются в `predictor/docs/`; детали протокола берутся из сгенерированных спецификаций.
