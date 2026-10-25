# Company profile and metric contract

The company profile is Markdown at companies/{company_id}/profile.md in S3. It is the readable source of general company context used by the other agents. ESB is the only component that accesses S3.

Each metric has:

- metric_id: stable lowercase snake_case identifier, reused for the same concept.
- name: short human-readable metric name.
- description: general meaning of the metric.
- unit: unit or scale, such as RUB, percent, count, or an empty string when not applicable.
- metric_type: numeric, currency, percentage, count, ratio, score, or text.
- characteristic: how the metric is defined and interpreted for this company.
- parameters: optional structured JSON settings, such as calculation rules, thresholds, or period conventions.

Metrics come from one shared catalog of eight words: rate (ставка), deposit (депозит), fx_rate (курс), logistics (логистика), price (цена), demand (спрос), inventory (запас), and wage (зарплата). A company gets a row only when its company_data has the trigger for that word; the conditions are listed in SKILL.md. For every row, name is the catalog word, description states what growth means, metric_type is score, unit is empty, characteristic is a short phrase for this company such as "кредита и лизинга", and parameters.source_fields lists the triggering fields. The catalog word контрагент is not used because payment data is not part of the input.

`profile_revision` is a positive, monotonically increasing revision supplied by Domain. Repeating the same revision is safe; an older revision cannot replace the profile or metric settings.

All metric settings are stored together in PostgreSQL in `company_metric_settings`, keyed by `(company_id, metric_id)`. `metric_type` is a PostgreSQL enum. PostgreSQL is the source of truth; ClickHouse contains no metric catalog or company metric characteristics.

`save_company_profile_and_metrics` treats `metrics` as the complete authoritative snapshot for the supplied profile revision. It atomically removes settings omitted from the snapshot and upserts the supplied settings. An empty array removes all metric settings for that company. Repeating the same revision and payload is idempotent.

Valera writes the Markdown and settings through one ESB MCP tool. S3 uses a stable profile key and PostgreSQL uses an upsert, so the tool can be retried safely after a partial failure.

## Inputs

The Domain request accepted by `POST /call?agent=valera` uses `companyId`, `profileRevision`, and `company`. ESB normalizes it before invoking Hermes; the agent receives:

- `company_id`: required UUID of the company.
- `company_data`: required company attributes imported from the bank's CSV, such as `syntheticU`, `mainOkvedName`, `revenueCompany`, and `region`.
- `profile_revision`: required positive number that grows with every change of the source data.

`call-u80.json` is a normalized Hermes input example. The HTTP payload, including the optional `{"agent": ..., "payload": ...}` wrapper, is documented in `predictor/docs/esb-contracts.md`.
