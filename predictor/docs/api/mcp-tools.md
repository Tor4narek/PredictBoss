<!-- Generated from the MCP tools/list registry. Do not edit by hand. -->
# Инструменты MCP ESB

Источник схем запросов и ответов: [`mcp-tools.json`](mcp-tools.json). Список генерируется из тех же зарегистрированных инструментов, которые ESB отдаёт агентам через `tools/list`.

| Инструмент | Назначение |
| --- | --- |
| `calculate_all_metric_totals` | Read a company's metric rows from PostgreSQL, recalculate each row from ClickHouse, and return the catalog-ordered results without persisting totals. |
| `calculate_metric_total` | Recalculate one metric row from ClickHouse for an inclusive news-time range; no total is persisted. |
| `get_company_facts` | Read universal facts that have already been linked to a company. Limit accepts an integer or numeric string. |
| `get_company_metrics` | Read the company's structured metric settings from PostgreSQL through the ESB. Limit accepts an integer or numeric string. |
| `get_company_profile` | Read the company's Markdown profile from S3 through the ESB. |
| `get_company_report` | Read a company report containing linked facts, configured metrics, totals and analyses. |
| `get_consultant_context` | Read linked facts and observations from ClickHouse and recalculate configured company metric totals through ESB. Results include source IDs; totals use news publication times across stored history. Limit accepts an integer or numeric string. |
| `get_facts` | Read universal facts for a news_id from ClickHouse. Limit accepts an integer or numeric string. |
| `save_company_analysis` | Persist a probability-based analysis. Probability accepts a JSON number or numeric string; ESB normalizes numeric strings. |
| `save_company_metrics` | Idempotently upsert structured metric settings for a company in PostgreSQL through the ESB. |
| `save_company_profile_and_metrics` | Idempotently save the company Markdown profile to S3 and upsert its structured metric settings in PostgreSQL through the ESB. |
| `save_fact_company_links` | Persist Savely's supported links between universal facts and a company. |
| `save_metric_observations` | Store metric observations linked to news and facts. Values and probabilities accept JSON numbers or numeric strings; ESB normalizes numeric strings. Use null for unknown values. Period bounds are not stored on observations. ESB derives a stable observation ID from company, metric, news, and source fact IDs; supply a distinct observation_id when separate predictions share the same fact IDs. |
| `save_news_and_facts` | Store submitted news and company-independent facts in ClickHouse. Confidence accepts a JSON number or a numeric string; ESB normalizes numeric strings. |
