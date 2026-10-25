package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

func RenderDocumentation() []byte {
	settings := Definitions()
	sort.Slice(settings, func(i, j int) bool {
		if settings[i].Category != settings[j].Category {
			return settings[i].Category < settings[j].Category
		}
		return settings[i].Name < settings[j].Name
	})

	var document bytes.Buffer
	document.WriteString("<!-- Generated from the typed ESB environment setting catalog. Do not edit by hand. -->\n")
	document.WriteString("# Конфигурация Predictor\n\n")
	document.WriteString("ESB загружает конфигурацию один раз при старте из унаследованных переменных окружения. Compose автоматически читает `.env` из каталога проекта и передаёт значения сервисам. Прямой `go run` не читает `.env`: экспортируйте переменные в оболочке или IDE. Секреты из локального `.env` не включаются в документацию.\n\n")
	document.WriteString("## Переменные окружения\n\n")
	document.WriteString("Диапазоны для длительностей включают обе границы. Некорректные числа, длительности и bool останавливают запуск; сообщение содержит имя переменной, но не её значение. Ноль допустим только там, где он указан в диапазоне. Пустые optional string используют default; пустые required значения вызывают ошибку запуска.\n\n")
	document.WriteString("| Имя | Тип | Default | Допустимый диапазон | Единица | Назначение | Категория |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, setting := range settings {
		defaultValue := setting.Default
		if setting.Secret && defaultValue != "required" && defaultValue != "empty / unset" {
			defaultValue = "локальный placeholder из `.env.example`"
		}
		document.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s | %s |\n",
			setting.Name, escapeCell(setting.Type), escapeCell(defaultValue), escapeCell(setting.Range),
			escapeCell(setting.Unit), escapeCell(setting.Purpose), categoryName(setting.Category)))
	}
	document.WriteString("\n")
	document.WriteString("`ESB_E2E_BASE_URL` и `MOCK_PORT` относятся только к тестовой среде. `HERMES_*_URL` обязательны для прямого запуска; Compose подставляет URL одноимённых Hermes-сервисов, если переменная не задана.\n\n")
	document.WriteString("## Настройки агентов\n\n")
	document.WriteString("Роль, порядок работы, политика сжатия истории, модель, reasoning effort, MCP allowlist, инструменты и runtime-лимиты Hermes остаются в `predictor/agents/<agent>/`. `AGENT_<TYPE>_MAX_HISTORY_MESSAGES` — отдельное эксплуатационное исключение: предел сообщений для контекста Hermes задан через env, `0` означает не передавать историю. История PostgreSQL продолжает управляться `history_retention_days` из `agent.json`.\n\n")
	document.WriteString("| Файл | Настройки | Кто использует |\n| --- | --- | --- |\n")
	document.WriteString("| `agents/<type>/agent.json` | `scope_field`, `required_fields`, `history_retention_days`, `context_policy` (порог, размер batch сжатия, размер summary и PostgreSQL как место хранения) | ESB: правила вызова и хранения контекста агента |\n")
	document.WriteString("| `agents/<type>/hermes.yaml` | `model.provider/default/base_url/key_env`, `gateway.api_server`, `max_concurrent_runs`, `agent.max_turns`, `reasoning_effort`, disabled toolsets, skill directory, ESB MCP URL, allowlist и режим параллельных вызовов | Hermes: runtime и роль агента |\n")
	document.WriteString("| `agents/<type>/skills/**` | Skill, порядок анализа, доменные инструкции и ссылки на data contract | Hermes: содержание задачи |\n")
	document.WriteString("\n")
	document.WriteString("## Инвентаризация остальных настроек\n\n")
	document.WriteString("| Источник | Классификация | Что остаётся там | Причина |\n| --- | --- | --- | --- |\n")
	document.WriteString("| `docker-compose.yml` | эксплуатационная настройка — env | host ports, healthcheck интервалы/таймауты/повторы, адреса сервисов и deployment credentials; значения ESB перечислены выше | параметры окружения и развёртывания, меняются без перекомпиляции |\n")
	document.WriteString("| `predictor/storage/{postgres,clickhouse}/migrations/*_init.sql` | доменный/API-инвариант | таблицы, типы, статусы, enum, ограничения и индексы схемы | начальная схема хранит форму данных; runtime значения в неё не переносятся |\n")
	document.WriteString("| `predictor/tests/compose.e2e.yml`, `e2e_test.go`, `mock_server.py` | только тестовая конфигурация | тестовые credentials, фиксированные fixture ID, адреса и короткие ожидания | изолированный mock-прогон не использует production secret или LLM; `ESB_E2E_BASE_URL` и `MOCK_PORT` перечислены выше |\n")
	document.WriteString("| `agents/<type>/agent.json` | роль/контекст агента | область чата, обязательные поля, срок хранения полной истории и политика сжатия | политика применяется отдельно к контексту конкретного агента |\n")
	document.WriteString("| `agents/<type>/hermes.yaml` | runtime-настройка агента | модель, reasoning, max turns, MCP allowlist и `max_concurrent_runs` | настройки исполнения роли принадлежат Hermes; `0` в текущем runtime означает отсутствие per-agent concurrency cap |\n")
	document.WriteString("| очередь ESB | неизменяемая архитектурная граница | отдельного глобального concurrency cap нет; размер `QUEUE_DISPATCH_BATCH_SIZE` ограничивает число задач, извлекаемых за цикл | параллельное исполнение остаётся под контролем каждого Hermes runtime и ресурсов развёртывания |\n")
	document.WriteString("\n")
	document.WriteString("## Фиксированные инварианты\n\n")
	document.WriteString("Эти значения задают смысл контракта или безопасную валидацию, поэтому не становятся deployment env:\n\n")
	document.WriteString("| Инвариант | Почему фиксирован |\n| --- | --- |\n")
	document.WriteString("| Типы агентов (`serezha`, `saveliy`, `valera`, `kesha`, `konstantin`), поля области и маршруты `/call`, `/stream`, `/mcp`, health routes | Идентифицируют компоненты и публичный контракт |\n")
	document.WriteString("| Статусы вызовов и роли сообщений | Состояния конечного автомата и ограничения схемы PostgreSQL |\n")
	document.WriteString("| `retry_count` Валеры в PostgreSQL: 0…1 | `VALERA_PROFILE_SYNC_MAX_RETRIES` имеет диапазон 0…1 и default 1; верхняя граница обязана совпадать с init CHECK constraint |\n")
	document.WriteString("| Каталог метрик, enum типа метрики, виды материалов и медиа | Бизнес-словарь; хранится в коде/схеме, а не в deployment env |\n")
	document.WriteString("| Идентификаторы: строковый ключ до 128 символов, UUID там, где контракт требует UUID; вероятности в диапазоне 0…1 | Проверка формата и математический смысл входных данных |\n")
	document.WriteString("| `integerValue` MCP ограничен 1 000 000 | Общая защита от некорректных целочисленных аргументов; конкретные list limits дополнительно применяют env-границы |\n")
	document.WriteString("| Конкурентное сжатие истории повторяет compare-and-swap не более 16 раз | Ограниченный внутренний цикл защищает от бесконечной гонки; при исчерпании ESB использует обычную историю |\n")
	document.WriteString("| HTTP body резервирует 4 KiB под JSON metadata сверх `MAX_NEWS_TEXT_BYTES`; диагностический ответ ClickHouse при `INSERT` ограничен 4 KiB | Небольшие фиксированные границы для структуры запроса и текста ошибки; успешные ответы ограничиваются `CLICKHOUSE_MAX_RESPONSE_BYTES` |\n")
	document.WriteString("| `EventsAfter` принимает 1…10 000 событий и использует fallback 100 для внутреннего неверного аргумента | Защитная валидация адаптера; публичный WS replay отдельно ограничен `STREAM_REPLAY_LIMIT` |\n")
	document.WriteString("| Схема PostgreSQL и ClickHouse в `storage/*/migrations/*_init.sql` | Init описывает состояние данных, а не runtime-параметры |\n")
	document.WriteString("\n")
	document.WriteString("## Прямой запуск ESB\n\n")
	document.WriteString("Скопируйте `.env.example` в `.env`. `docker compose` подхватит его автоматически. При запуске Go-процесса напрямую задайте нужные переменные в оболочке/IDE (например, через свой менеджер окружения); бинарник намеренно не загружает `.env` неявно. Укажите `AGENTS_DIR` и пять `HERMES_<TYPE>_URL` адресов, доступных из процесса.\n")
	return document.Bytes()
}

func escapeCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func categoryName(value string) string {
	switch value {
	case "runtime":
		return "runtime ESB"
	case "storage":
		return "подключение к хранилищу"
	case "secret":
		return "секрет / credential"
	case "agent-context":
		return "контекст агента"
	case "compose":
		return "Docker Compose"
	case "test":
		return "только тесты"
	default:
		return value
	}
}
