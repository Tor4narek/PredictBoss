# ESB end-to-end test specification

The Domain/ESB payload and event contract is documented in [`../docs/esb-contracts.md`](../docs/esb-contracts.md). This test specification turns that contract into executable checks against the running service.

## Goal and test boundary

Run the built ESB service and exercise its public HTTP, WebSocket, and MCP interfaces. PostgreSQL, ClickHouse, and MinIO are real disposable containers. Hermes and Domain API are deterministic mocks. This verifies routing, authentication, asynchronous call handling, database migrations and repositories, MCP tool schemas, real S3 profile persistence, source metadata, and WebSocket replay without calling an LLM or a production Domain API.

The test project uses a dedicated Compose project name and disposable volumes. It never uses the normal `predictor` Compose project or its volumes. ESB creates the test bucket on startup, then the MCP profile write/read verifies path-style S3 `PutObject` and `GetObject` against MinIO.

## Contract matrix

| Surface | Scenarios |
| --- | --- |
| HTTP health/auth | Public `GET /healthz`; bearer-protected `GET /readyz`; unauthorized access to protected routes |
| `POST /call?agent=<type>` | Valera profile call and idempotent retry; Konstantin analysis enqueue; Savely and Kesha calls; invalid agent/input and conflicting profile revision |
| `GET /call?agent=<type>` | Read call by ID; read Kesha report by company; Konstantin returns a ready analysis or processing status plus `/stream` URL; wrong agent and missing body |
| `POST /news` | External news and Alpha analysis metadata; Serezha enqueue; idempotent repeat; unknown `sourceId`; 1 MiB input limit |
| WebSocket `/stream` | Authenticated connection without an `agent` query; analysis delivery; question acceptance and answer; reconnect replay; stale-analysis rejection; invalid route/query/cursor |
| MCP `/mcp` | Authenticated initialize, discovery, and calls to every tool: `get_company_profile`, `save_company_profile_and_metrics`, `save_news_and_facts`, `get_facts`, `save_fact_company_links`, `get_company_facts`, `save_company_metrics`, `get_company_metrics`, `save_metric_observations`, `calculate_metric_total`, `calculate_all_metric_totals`, `save_company_analysis`, `get_company_report`, and `get_consultant_context` |
| Storage invariants | PostgreSQL owns metric settings and chat state; ClickHouse owns news/facts/links/observations/totals/analysis; S3-compatible storage owns Markdown profiles; a weighted total checks probability precision; new source or profile revision invalidates the prior analysis |

## Execution

From the repository root:

```sh
./predictor/tests/run-e2e.sh
```

The script starts the isolated dependencies, applies the ClickHouse migration chain, starts the production ESB image, runs the Go API client tests, and removes only the test project's containers and volumes on exit. Go tests can also be run directly with `cd predictor/tests && ESB_E2E_BASE_URL=http://127.0.0.1:18081 go test -v -count=1 ./...` while that stack is running.

## Expected behavior

- Agent requests return promptly with `202 Accepted` and a call identifier; status/result reads are separate.
- A profile revision is considered pending immediately on acceptance. The saved Markdown and metric settings make it ready only after the combined MCP save succeeds.
- News is registered against its company before Serezha is queued; retries with the same source/text are idempotent.
- Konstantin's analysis and answers are persisted before they are delivered. WebSocket frames are complete messages, not token/progress updates.
- WebSocket reconnect uses `afterMessageId`, and evidence is returned only for registered source IDs with compatible origin kinds.
- New source metadata and a newer saved profile revision each make the previous analysis unavailable for new questions and trigger a new analysis request.

## Non-goals

The mock Hermes does not evaluate prompt quality or model accuracy. These tests validate the runtime protocol and data-flow contract, not LLM reasoning quality or production Domain API availability.
