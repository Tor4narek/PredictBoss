# PredictBoss

Асинхронная система новостной аналитики. ESB принимает вызовы Domain, ведёт очередь и чаты в PostgreSQL, запускает агентов Hermes и предоставляет им единый MCP endpoint для аналитических данных и профилей компаний.

## Документация

- [Domain API, Worker и миграции EF Core](predictor/domain/README.md)
- [Контракт Domain ↔ ESB](predictor/docs/esb-contracts.md)
- [Архитектура и границы компонентов](predictor/docs/architecture.md)
- [Подключение Domain к ESB](predictor/docs/integration.md)
- [Конфигурация окружения](predictor/docs/configuration.md) — переменные, defaults, диапазоны и назначение.
- [Контракты HTTP, WebSocket и MCP](predictor/docs/api/README.md)
- [Спецификация E2E](predictor/tests/SPEC.md)

OpenAPI, AsyncAPI и каталог MCP генерируются из Go-кода и зарегистрированных инструментов. Не редактируйте файлы в `predictor/docs/api/` вручную; выполните `make docs` после изменения контракта.

## Быстрый запуск

Нужны Docker Compose и Go 1.24. Скопируйте `.env.example` в `.env`, замените локальные placeholders и передавайте файл Compose явно. Полный список настроек ESB приведён в `predictor/example.env` и справочнике конфигурации. URL Hermes внутри Compose уже настроены:

Для пустой базы примените единственную init-миграцию PostgreSQL через Atlas и init-схему ClickHouse перед запуском ESB:

```sh
cp .env.example .env
docker compose --env-file .env up -d postgres clickhouse
docker compose --env-file .env --profile migrations run --rm atlas-postgres
# Для ClickHouse через Atlas нужен ATLAS_TOKEN.
docker compose --env-file .env --profile migrations run --rm atlas-clickhouse
docker compose --env-file .env up --build
```

В каждом каталоге `predictor/storage/{postgres,clickhouse}/migrations/` хранится одна версия `*_init.sql` с конечной схемой, состоящей из `CREATE`. Это исходное состояние проекта: миграций для обновления существующих баз нет. Применяйте init только к пустой базе; существующие данные и volumes автоматически не удаляются.

Для запуска Domain API и фронтенда `DOMAIN_CSV_PATH` в `.env.example` указывает на CSV в `predictor/domain/data/`. В публичном репозитории находится исходный датасет; деплой workflow переносит его на сервер отдельно от архива исходников. EF Core миграции базы Domain применяются при старте API.

## Автодеплой на Fedora

Workflow `.github/workflows/deploy-fedora.yml` обновляет выделенную копию `$HOME/predictboss` после каждого push в `main`. Он подключается к серверу по SSH, запускает `deploy/fedora/deploy.sh`, применяет Atlas-миграции и пересобирает Compose-сервисы. Установленный на Fedora Caddy проксирует HTTPS-трафик на `127.0.0.1:3000`; остальные опубликованные порты Compose также привязаны к loopback.

Первичная настройка сервера, `.env`, CSV, SSH-ключа и GitHub Actions secrets описана в [deploy/fedora/README.md](deploy/fedora/README.md). Для выпуска TLS-сертификата DNS `PUBLIC_DOMAIN` должен указывать на Fedora, а входящие TCP-порты 80/443 должны быть доступны.

## Разработка

```sh
make build       # собрать ESB
make lint        # gofmt + закреплённый golangci-lint только для production-кода ESB
make test        # unit-тесты ESB и полный E2E на изолированных контейнерах
make tools       # установить закреплённый golangci-lint
make docs        # обновить сгенерированные API-контракты
make docs-check  # проверить, что контракты не устарели
```

Роли агентов, `SKILL.md` и конфигурации Hermes находятся в `predictor/agents/`. Для Compose всегда передавайте `--env-file .env`; прямой запуск Go-приложения получает только экспортированные переменные и сам env-файл не загружает. Полный список настроек и их единиц — в [справочнике конфигурации](predictor/docs/configuration.md). Инструкция по lint, тестам и исключениям — в [руководстве разработчика](predictor/docs/development.md). Ручные архитектурные решения поддерживаются в `predictor/docs/`; детали протокола берутся из сгенерированных спецификаций.
