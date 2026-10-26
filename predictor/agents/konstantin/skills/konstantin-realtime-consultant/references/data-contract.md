# Konstantin data contract

The ESB sends `company_id`, a request kind, and the request or analysis identifiers to Hermes. The ESB stores the finished response and evidence in its PostgreSQL chat history before making the message available on `/stream`.

Use `get_consultant_context` for company-linked facts and pre-aggregated metric observations and totals from ClickHouse. Each fact and observation may include `news_id`, `source_id`, `source_url`, title, publication time, and material kind. Do not return source IDs, evidence arrays, or links: ESB records only the publication metadata actually returned by this tool for the signed call and adds it to the final response. The company profile is Markdown in S3 and metric definitions are structured PostgreSQL settings, both read through ESB MCP.

Output JSON fields:

- `text`: complete user-facing answer.
- `evidence`: populated by ESB from unique publications returned by `get_consultant_context` for this call and company. It includes source/news IDs and available publication metadata; model-generated evidence is ignored.

ESB sets `originKind` from the verified material kind: `external_news` becomes `fact`, and `alpha_analysis` becomes `bank_view`. It does not accept model-generated source IDs or create `ai_inference` citations. No date range is mandatory.
