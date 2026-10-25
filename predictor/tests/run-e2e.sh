#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

compose=(docker compose --project-name predictor-esb-e2e --file predictor/tests/compose.e2e.yml)
cleanup() {
	status=$?
	cd "$ROOT_DIR"
	if [[ "$status" -ne 0 ]]; then
	  "${compose[@]}" logs --no-color || true
	fi
  "${compose[@]}" down --volumes --remove-orphans
  return "$status"
}
trap cleanup EXIT

"${compose[@]}" down --volumes --remove-orphans
"${compose[@]}" up --detach --wait postgres clickhouse mock-runtime
for migration in predictor/storage/migrations/*.sql; do
  "${compose[@]}" exec --no-TTY clickhouse clickhouse-client --multiquery < "$migration"
done
"${compose[@]}" up --detach --build --wait esb

cd predictor/tests
ESB_E2E_BASE_URL="http://127.0.0.1:18081" go test -count=1 -v ./...
