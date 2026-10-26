#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if ! python3 - <<'PY'
import os
from pathlib import Path

if os.environ.get("LLM_API_KEY", "").strip():
    raise SystemExit(0)
try:
    lines = Path("predictor/.env").read_text().splitlines()
except OSError:
    lines = []
for line in lines:
    if line.startswith("LLM_API_KEY=") and line.split("=", 1)[1].strip():
        raise SystemExit(0)
raise SystemExit(1)
PY
then
	cat >&2 <<'EOF'
LLM_API_KEY is not configured. Add it to predictor/.env or the environment, then run make test-llm-tools.
EOF
	exit 0
fi

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
	echo "Docker Engine and Docker Compose v2 are required for the isolated real-LLM test." >&2
	exit 1
fi

export ESB_LLM_TOOLS_ENABLED=1
export ESB_E2E_BASE_URL="http://127.0.0.1:18081"
export ESB_LLM_TOOLS_OBSERVER_URL="http://127.0.0.1:18095"
export LLM_TEST_CORRELATION_ID_SAVELIY="llm-saveliy-$(python3 -c 'import uuid; print(uuid.uuid4())')"
export LLM_TEST_CORRELATION_ID_KESHA="llm-kesha-$(python3 -c 'import uuid; print(uuid.uuid4())')"
export LLM_TEST_CORRELATION_ID_KONSTANTIN="llm-konstantin-$(python3 -c 'import uuid; print(uuid.uuid4())')"

compose=(docker compose --project-name predictor-esb-llm-tools)
if [[ -f predictor/.env ]]; then
	compose+=(--env-file predictor/.env)
fi
compose+=(--file predictor/tests/compose.e2e.yml --file predictor/tests/compose.llm-tools.yml)
migrations=("${compose[@]}" --profile migrations)
cleanup() {
	status=$?
	cd "$ROOT_DIR"
	"${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
	return "$status"
}
trap cleanup EXIT

"${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
"${compose[@]}" up --detach --wait postgres clickhouse minio mock-runtime mock-runtime-secondary mcp-observer
"${migrations[@]}" run --rm atlas-postgres
"${compose[@]}" exec --no-TTY clickhouse clickhouse-client --user predictor --password test-clickhouse-password \
	--database default --multiquery < predictor/storage/clickhouse/migrations/20261025180000_init.sql
"${compose[@]}" up --detach --build --wait esb hermes-saveliy-llm hermes-kesha-llm hermes-konstantin-llm

cd predictor/tests
go test -run '^TestRealLLMProfileMCPCalls$' -count=1 -timeout=12m -v .
