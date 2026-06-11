#!/usr/bin/env bash
# Ensure the solo-postgres container is up and the database is ready.
# Idempotent: returns immediately if the container is already healthy.
set -euo pipefail

CONTAINER="${SOLO_POSTGRES_CONTAINER:-solo-postgres}"
DB_USER="${POSTGRES_USER:-solo}"
DB_NAME="${POSTGRES_DB:-solo}"

if docker exec "$CONTAINER" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
  exit 0
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "==> Starting PostgreSQL container..."
docker compose up -d --remove-orphans postgres >/dev/null 2>&1 || true

echo "==> Waiting for PostgreSQL to be ready..."
for i in $(seq 1 30); do
  if docker exec "$CONTAINER" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
    echo "✓ PostgreSQL ready"
    exit 0
  fi
  sleep 1
done

echo "ERROR: PostgreSQL not ready after 30s" >&2
exit 1
