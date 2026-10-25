# Документация

- [Архитектура](architecture.md) — компоненты, ответственность за данные и поток вызовов.
- [Интеграция Domain ↔ ESB](integration.md) — короткий маршрут по основным сценариям.
- [Контракты API](api/README.md) — ссылки на OpenAPI, AsyncAPI и каталог MCP.

## Где менять документацию

| Что изменилось | Источник истины |
| --- | --- |
| HTTP-запрос или ответ | Go-контракты в `predictor/esb/internal/presentation/httpapi/` |
| WebSocket-кадр Константина | типы кадров в `httpapi/contracts.go`, обработка в `httpapi/stream.go` |
| Инструмент или схема MCP | регистрация инструмента в `predictor/esb/internal/presentation/mcp/` |
| Роль, контекст или настройки агента | `predictor/agents/<agent>/` |
| Владелец таблицы или схема хранилища | Atlas-конфигурация и миграция в `predictor/storage/` |
| Архитектурное решение и границы сервисов | `architecture.md` |

После изменений HTTP-, WebSocket- или MCP-контрактов выполните `make docs` и включите обновлённые артефакты в тот же коммит. `make docs-check` проверяет, что спецификации соответствуют текущему коду.
