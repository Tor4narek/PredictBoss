# Metric analysis contract

PostgreSQL is the source of truth for company metric rows. Each row has an opaque, PostgreSQL-generated UUID `metric_id`, `company_id`, `type_code`, `metric_type`, `characteristic`, `unit`, and optional parameters. `type_code` follows the shared order: `ставка`, `депозит`, `курс`, `логистика`, `цена`, `спрос`, `запас`, `контрагент`, `зарплата`. Never parse company or type information from `metric_id`.

ClickHouse stores news, universal facts and links, plus metric observations. An observation references `company_id`, the opaque `metric_id`, `news_id`, a nullable numeric `value`, a nullable probability, and `source_fact_ids` (an empty array when there are no linked fact IDs). It has no period fields. ESB derives a stable observation ID from the company, metric, news, and source fact IDs; use an explicit distinct `observation_id` if separate predictions share the same fact IDs. News publication time, URL, and source kind are read from the linked news row. Bank material maps to `source_kind: bank`; external news maps to `source_kind: external`.

## Calculation

`calculate_metric_total` takes only `metric_id`, `period_from`, and `period_to`. It filters by linked news publication time with inclusive bounds. A prediction's insertion time does not affect the period. A missing news row, missing publication time, or null value excludes that observation. Zero is included. Multiple observations for one news item each contribute to the averages.

- `total_value` is `avgOrNull(value)` over selected observations. Probability does not weight value.
- `total_probability` is `avgOrNull(probability)` over the same selected observations, excluding null probabilities only from this average.
- `fact_count` counts selected non-null values.
- `probability_count` counts selected non-null probabilities.
- `links` contains each selected `news_id` once, ordered by news time then news ID. Empty URLs do not exclude links.

No selected rows means null totals, zero counts, and an empty links array. Do not replace null with zero. If `period_to` is before `period_from`, return the same empty aggregate without swapping bounds.

`calculate_all_metric_totals` reads the company's configured rows from PostgreSQL in catalog order, calls the first calculation for each UUID, and returns `company_id`, period bounds, and `items`. Each item contains `metric_id`, `type_code`, `characteristic`, and the calculation fields. Empty metric results remain in `items`; companies with no configured rows return an empty `items` array. Configured rows are never averaged together.

Totals are computed on every request and are not persisted. Agent access to PostgreSQL, ClickHouse, and S3 goes only through ESB MCP tools.
