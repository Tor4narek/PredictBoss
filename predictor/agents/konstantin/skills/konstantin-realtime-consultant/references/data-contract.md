# Konstantin data contract

The ESB sends `company_id`, a request kind, and the request or analysis identifiers to Hermes. The ESB stores the finished response and evidence in its PostgreSQL chat history before making the message available on `/stream`.

Use `get_consultant_context` for company-linked facts and pre-aggregated metric observations and totals from ClickHouse. Each fact and observation includes `source_id`, `source_url`, and `material_kind`. Copy the exact `source_id` UUID into the final evidence field named `sourceId`; do not use `news_id` or `fact_id` as a substitute. The company profile is Markdown in S3 and metric definitions are structured PostgreSQL settings, both read through ESB MCP.

Output JSON fields:

- `text`: complete user-facing answer.
- `evidence`: unique `{ "sourceId": "...", "originKind": "fact|bank_view|ai_inference" }` entries.

Use `fact` for external news, `bank_view` for Alpha Bank analyses, and `ai_inference` for a clearly identified inference. No date range is mandatory.
