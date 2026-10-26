package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

var categoryOrder = map[string]int{
	"runtime": 1, "hermes": 2, "hermes-agent": 3, "storage": 4,
	"agent-context": 5, "cleanup": 6, "secret": 7, "compose": 8, "test": 9,
}

func RenderDocumentation() []byte {
	settings := Definitions()
	sort.Slice(settings, func(i, j int) bool {
		if categoryOrder[settings[i].Category] != categoryOrder[settings[j].Category] {
			return categoryOrder[settings[i].Category] < categoryOrder[settings[j].Category]
		}
		return settings[i].Name < settings[j].Name
	})

	var document bytes.Buffer
	document.WriteString("<!-- Generated from the typed ESB environment setting catalog. Do not edit by hand. -->\n")
	document.WriteString("# Конфигурация Predictor\n\n")
	document.WriteString("`predictor/.env` — общий env-файл для ESB, Hermes и Atlas; он не хранится в Git. Скопируйте `predictor/example.env`, задайте credentials и запустите Compose с `--env-file predictor/.env`. Прямой запуск Go или Hermes не загружает файл автоматически.\n\n")
	document.WriteString("## Переменные окружения\n\n")
	document.WriteString("| Имя | Тип | Default | Единица / диапазон | Назначение |\n| --- | --- | --- | --- | --- |\n")
	for _, setting := range settings {
		defaultValue := setting.Default
		if setting.Secret && defaultValue != "required" && defaultValue != "empty / unset" {
			defaultValue = "placeholder в `predictor/example.env`"
		}
		unitAndRange := setting.Unit
		if setting.Range != "" {
			unitAndRange += "; " + setting.Range
		}
		document.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
			setting.Name, escapeCell(setting.Type), escapeCell(defaultValue), escapeCell(unitAndRange), escapeCell(setting.Purpose)))
	}
	document.WriteString("\n")
	document.WriteString("Длительности используют единицы Go `ns`, `us`/`µs`, `ms`, `s`, `m`, `h` (день — `24h`). Для retention `0`, `0s` и `immediate` означают очистку после подтверждения операции, `never` — хранение без срока. Политики очистки выполняются при `CLEANUP_ENABLED=true`.\n\n")
	document.WriteString("## Настройки агентов\n\n")
	document.WriteString("У всех агентов общий `HERMES_MODEL_PROVIDER` и `LLM_API_KEY`. Модель и runtime-лимиты задаются в env отдельно для агента. Остальная структура и полномочия находятся в `predictor/agents/<type>/hermes.yaml`; роль и сжатие контекста — в `agent.json` и `skills/`.\n\n")
	document.WriteString("`HERMES_<TYPE>_INSTANCES` задаёт runtime-пул в формате `id=state@url;id=state@url`; состояния: `available`, `draining`, `unavailable`. `AGENT_<TYPE>_MAX_CONCURRENT_RUNS` — лимит одновременных вызовов на один runtime. Все ESB-процессы должны использовать одинаковую конфигурацию пула. PostgreSQL сохраняет runtime каждого вызова и координирует резервирование слотов между ESB-репликами. Удаляйте runtime ID только после завершения закреплённых за ним вызовов. `HERMES_<TYPE>_URL` поддерживается для совместимости с одиночным runtime.\n\n")
	document.WriteString("`context_policy.max_history_messages=0` отключает передачу истории модели. Хранение полной истории и резюме PostgreSQL задаётся независимо через `CLEANUP_MESSAGES_RETENTION_<TYPE>` и `CLEANUP_CONTEXT_SUMMARY_RETENTION`.\n")
	return document.Bytes()
}

// RenderExampleEnv renders a safe-to-commit environment template from the
// same definitions used by LoadFrom and the configuration reference.
func RenderExampleEnv() []byte {
	settings := Definitions()
	sort.Slice(settings, func(i, j int) bool {
		left, right := settings[i], settings[j]
		leftOrder, rightOrder := categoryOrder[left.Category], categoryOrder[right.Category]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return left.Name < right.Name
	})

	var example strings.Builder
	example.WriteString("# Generated from the typed ESB configuration catalog. Run `make docs` after changing it.\n")
	example.WriteString("# Copy this file to predictor/.env. Replace every sample credential before a private deployment.\n")
	example.WriteString("# Duration units: ns, us/µs, ms, s, m, h. One day is 24h; one week is 168h.\n")
	example.WriteString("# Byte limits use bytes (1 MiB = 1048576); *_DAYS settings use days; count limits use their named rows/items.\n")
	example.WriteString("# Retention values: 0, 0s, or immediate = clean after processing and safety checks; a positive duration = retain that long; never = keep.\n")
	example.WriteString("# CLEANUP_ENABLED=false disables scheduled retention and immediate cleanup notifications.\n")
	example.WriteString("# Company metric units (for example ₽, %, тонн/месяц) are saved per company in PostgreSQL by Valera through ESB MCP.\n\n")
	currentCategory := ""
	for _, setting := range settings {
		if setting.Category != currentCategory {
			if currentCategory != "" {
				example.WriteByte('\n')
			}
			currentCategory = setting.Category
			example.WriteString("# " + exampleCategoryName(currentCategory) + "\n")
		}
		defaultValue := setting.Default
		if setting.Secret {
			defaultValue = "sample/required"
		}
		comment := fmt.Sprintf("# %s (type: %s; default: %s; unit: %s; range: %s)",
			setting.Purpose, setting.Type, defaultValue, setting.Unit, setting.Range)
		example.WriteString(comment + "\n")
		example.WriteString(setting.Name + "=" + exampleValue(setting) + "\n")
	}
	return []byte(example.String())
}

func exampleValue(setting Definition) string {
	switch setting.Name {
	case "ESB_API_KEY":
		return "replace-with-a-random-esb-bearer-key"
	case "HERMES_API_KEY":
		return "replace-with-a-random-hermes-bearer-key"
	case "LLM_API_KEY":
		return "replace-with-your-LLM-provider-key"
	case "S3_BUCKET":
		return "company-profiles"
	case "S3_ENDPOINT":
		return "http://minio:9000"
	case "S3_ACCESS_KEY_ID":
		return "predictor-local"
	case "S3_SECRET_ACCESS_KEY":
		return "local-minio-password-change-me"
	case "HERMES_SEREZHA_INSTANCES":
		return "serezha-1=available@http://hermes-serezha:8642;serezha-2=available@http://hermes-serezha-2:8642"
	case "HERMES_SAVELIY_INSTANCES":
		return "saveliy-1=available@http://hermes-saveliy:8642;saveliy-2=available@http://hermes-saveliy-2:8642"
	case "HERMES_VALERA_INSTANCES":
		return "valera-1=available@http://hermes-valera:8642;valera-2=available@http://hermes-valera-2:8642"
	case "HERMES_KESHA_INSTANCES":
		return "kesha-1=available@http://hermes-kesha:8642;kesha-2=available@http://hermes-kesha-2:8642"
	case "HERMES_KONSTANTIN_INSTANCES":
		return "konstantin-1=available@http://hermes-konstantin:8642;konstantin-2=available@http://hermes-konstantin-2:8642"
	case "S3_FORCE_PATH_STYLE", "S3_CREATE_BUCKET":
		return "true"
	case "CLICKHOUSE_PASSWORD":
		return "local-clickhouse-password"
	case "DATABASE_URL", "CLICKHOUSE_URL", "AGENTS_DIR", "HERMES_SEREZHA_URL", "HERMES_SAVELIY_URL",
		"HERMES_VALERA_URL", "HERMES_KESHA_URL", "HERMES_KONSTANTIN_URL":
		return ""
	default:
		return setting.defaultRaw
	}
}

func exampleCategoryName(category string) string {
	switch category {
	case "runtime":
		return "ESB runtime"
	case "storage":
		return "Storage endpoints"
	case "agent-context":
		return "Agent context passed to Hermes"
	case "hermes":
		return "Hermes gateway and MCP"
	case "hermes-agent":
		return "Per-agent model and runtime"
	case "cleanup":
		return "Retention and cleanup"
	case "secret":
		return "Credentials and secrets: placeholders only"
	case "compose":
		return "Docker Compose runtime"
	case "test":
		return "Test-only settings"
	default:
		return category
	}
}

func escapeCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
