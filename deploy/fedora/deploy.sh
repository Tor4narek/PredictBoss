#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# This directory is a dedicated deployment checkout; local changes are not preserved.
git fetch --prune origin
git checkout main
git reset --hard origin/main

if [[ ! -f .env ]]; then
  echo "Missing $ROOT_DIR/.env; create it from .env.example and set production values." >&2
  exit 1
fi

csv_path="$(sed -n 's/^DOMAIN_CSV_PATH=//p' .env | tail -n 1 | tr -d '\"\r')"
if [[ -z "$csv_path" ]]; then
  echo "Set DOMAIN_CSV_PATH in .env to the company CSV on Fedora." >&2
  exit 1
fi
if [[ "$csv_path" != /* ]]; then
  csv_path="$ROOT_DIR/$csv_path"
fi
if [[ ! -f "$csv_path" ]]; then
  echo "Company CSV not found: $csv_path" >&2
  exit 1
fi

compose=(docker compose -f docker-compose.yml)
"${compose[@]}" config --quiet

# Start storage first so Atlas can apply database migrations before app containers update.
"${compose[@]}" up -d postgres clickhouse minio domain-postgres
"${compose[@]}" --profile migrations run --rm atlas-postgres
"${compose[@]}" --profile migrations run --rm atlas-clickhouse
"${compose[@]}" up -d --build --remove-orphans
"${compose[@]}" ps
