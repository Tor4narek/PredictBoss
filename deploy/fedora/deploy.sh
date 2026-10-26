#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# GitHub Actions provides the source archive during automated deployments.
# Manual deployments from a Git checkout continue to update from origin/main.
if [[ "${DEPLOY_FROM_ARCHIVE:-false}" != "true" ]]; then
  git fetch --prune origin
  git checkout main
  git reset --hard origin/main
fi

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

retry_pull() {
  local image="$1"
  local attempt=1
  local delay=10

  while true; do
    if docker pull "$image"; then
      return 0
    fi

    if (( attempt >= 5 )); then
      echo "Failed to pull $image after $attempt attempts." >&2
      return 1
    fi

    echo "Pull failed for $image (attempt $attempt/5); retrying in ${delay}s." >&2
    sleep "$delay"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
  done
}

# Pull images one at a time so a slow registry connection does not fail every
# concurrent image request. Compose config resolves the runtime image names
# from the production .env; the remaining images are Docker build bases.
mapfile -t runtime_images < <("${compose[@]}" config --images | sort -u | grep -v '^predictor-')
build_base_images=(
  arigaio/atlas:latest
  mcr.microsoft.com/dotnet/sdk:10.0
  mcr.microsoft.com/dotnet/aspnet:10.0
  mcr.microsoft.com/dotnet/runtime:10.0-noble
  node:22-alpine
  nginx:1.27-alpine
  golang:1.24-alpine
  alpine:3.20
)
for image in "${runtime_images[@]}" "${build_base_images[@]}"; do
  retry_pull "$image"
done

# Start storage first so Atlas can apply database migrations before app containers update.
"${compose[@]}" up -d postgres clickhouse minio domain-postgres
"${compose[@]}" --profile migrations run --rm atlas-postgres
"${compose[@]}" --profile migrations run --rm atlas-clickhouse
"${compose[@]}" build --parallel=1 domain-api domain-worker frontend esb
"${compose[@]}" up -d --no-build --remove-orphans
"${compose[@]}" ps
