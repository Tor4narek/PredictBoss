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

`profile_revision` is a positive, monotonically increasing revision supplied by Domain. Repeating the same revision is safe; an older revision cannot replace the profile or metric settings.

All metric settings are stored together in PostgreSQL in `company_metric_settings`, keyed by `(company_id, metric_id)`. `metric_type` is a PostgreSQL enum. PostgreSQL is the source of truth; ClickHouse contains no metric catalog or company metric characteristics.

Valera writes the Markdown and settings through one ESB MCP tool. S3 uses a stable profile key and PostgreSQL uses an upsert, so the tool can be retried safely after a partial failure.

## Inputs

The Domain request accepted by `POST /call?agent=valera` uses `companyId`, `profileRevision`, and `company`. ESB normalizes it before invoking Hermes; the agent receives:

- `company_id`: required UUID of the company.
- `company_data`: required company attributes imported from the bank's CSV, such as `syntheticU`, `mainOkvedName`, `revenueCompany`, and `region`.
- `profile_revision`: required positive number that grows with every change of the source data.

`call-u80.json` is a normalized Hermes input example. The HTTP payload, including the optional `{"agent": ..., "payload": ...}` wrapper, is documented in `predictor/docs/esb-contracts.md`.
