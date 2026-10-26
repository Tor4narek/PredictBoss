# Разработка

Команды запускаются из корня репозитория.

| Команда | Проверка |
| --- | --- |
| `make lint` | `gofmt` и закреплённый golangci-lint для production-кода ESB. |
| `make test` | Unit-тесты и E2E через ESB с изолированными PostgreSQL, ClickHouse, MinIO и mock Hermes. |
| `make test-unit` | Unit-тесты без Docker; исходники тестов лежат в `predictor/tests/unit/`. |
| `make test-e2e` | HTTP, WebSocket, MCP и хранилища; нужны Docker Compose v2 и запущенный Docker daemon. |
| `make test-race` | Unit-тесты с детектором гонок. |
| `make coverage` | Покрытие unit-тестов по пакетам и общий процент; E2E в отчёт не входит. |
| `make build` | Сборка ESB. |
| `make docs` / `make docs-check` | Генерация и проверка API и конфигурационной документации. |
| `make tools` | Установка golangci-lint версии `2.8.0`. |

`make lint` не проверяет тесты. Точечный `//nolint:<linter>` допустим с коротким объяснением; не добавляйте глобальные исключения ради прохождения проверки.

Тесты ESB физически хранятся вне модуля `predictor/esb`. `predictor/tests/run-unit.py` подключает их к исходным пакетам через Go overlay, сохраняя тестирование непубличного поведения. E2E использует отдельный Compose project и удаляет его контейнеры и тома после прогона.
