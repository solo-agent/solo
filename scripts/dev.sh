#!/usr/bin/env bash
set -euo pipefail

# Development runner for Solo
DIR="$(cd "$(dirname "$0")/.." && pwd)"

export PORT="${PORT:-8080}"
export DATABASE_URL="${DATABASE_URL:-postgres://solo:solo@localhost:5432/solo?sslmode=disable}"
export JWT_SECRET="${JWT_SECRET:-solo-dev-secret-change-in-production}"
export LOG_LEVEL="${LOG_LEVEL:-debug}"

echo "Starting Solo server on port $PORT..."
echo "Database: $DATABASE_URL"
echo "Log level: $LOG_LEVEL"

cd "$DIR"
go run ./cmd/server/
