#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

compose=(docker compose --project-name predictor-esb-e2e --file predictor/tests/compose.e2e.yml)
migrations=("${compose[@]}" --profile migrations)
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
"${compose[@]}" up --detach --wait postgres clickhouse minio mock-runtime
"${migrations[@]}" run --rm atlas-postgres
"${compose[@]}" exec --no-TTY clickhouse clickhouse-client --user predictor --password test-clickhouse-password --database default --multiquery < predictor/storage/clickhouse/migrations/20261025180000_init.sql
"${compose[@]}" up --detach --build --wait esb

cd predictor/tests
ESB_E2E_BASE_URL="http://127.0.0.1:18081" go test -count=1 -v ./...

test_news_id="55555555-5555-4555-8555-555555555555"
"${compose[@]}" exec --no-TTY clickhouse clickhouse-client --user predictor --password test-clickhouse-password --database default --query \
  "SELECT throwIf(count() != 0, 'raw source text remains after confirmed completion') FROM news_content FINAL WHERE news_id = '${test_news_id}'"
"${compose[@]}" exec --no-TTY clickhouse clickhouse-client --user predictor --password test-clickhouse-password --database default --query \
  "SELECT throwIf(count() = 0, 'news metadata was removed during text cleanup') FROM news FINAL WHERE news_id = '${test_news_id}'"
"${compose[@]}" exec --no-TTY clickhouse clickhouse-client --user predictor --password test-clickhouse-password --database default --query \
  "SELECT throwIf(count() = 0, 'facts were removed during text cleanup') FROM facts FINAL WHERE news_id = '${test_news_id}'"
