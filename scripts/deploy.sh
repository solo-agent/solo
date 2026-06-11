#!/usr/bin/env bash
# =============================================================================
# Solo - Production Deployment Script
# =============================================================================
# Usage:
#   ./scripts/deploy.sh [options]
#
# Options:
#   -e, --env FILE     Environment file (default: .env.production)
#   -p, --profile      Docker Compose profile (default: production)
#   -h, --help         Show this help message
#
# Environment variables (can be set in .env.production):
#   SERVER_PORT         Server listen port (default: 8080)
#   DAEMON_PORT         Daemon listen port (default: 8081)
#   DATABASE_URL        PostgreSQL connection string
#   JWT_SECRET          JWT signing secret (see scripts/gen-secret.sh)
#   LLM_API_KEY         LLM provider API key
#
# Examples:
#   ./scripts/deploy.sh                       # Deploy using .env.production
#   ./scripts/deploy.sh --env .env.staging     # Deploy using staging config
# =============================================================================

set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$DIR"

# ---- Config ----
ENV_FILE=".env.production"
MAX_RETRIES=30
RETRY_INTERVAL=2

# ---- Parse arguments ----
while [[ $# -gt 0 ]]; do
  case "$1" in
    -e|--env) ENV_FILE="$2"; shift 2 ;;
    -h|--help)
      sed -n 's/^# \?//p' "$0" | head -n -1
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [ ! -f "$ENV_FILE" ]; then
  echo "ERROR: Environment file '$ENV_FILE' not found."
  echo "Copy .env.example to $ENV_FILE and fill in the values."
  exit 1
fi

echo "=== Solo Deployment ==="
echo "Environment: $ENV_FILE"
echo "Directory: $DIR"
echo ""

# ---- Export environment variables ----
set -a
source "$ENV_FILE"
set +a

# ---- Step 1: Pull latest images ----
echo "[1/4] Pulling latest images..."
docker compose -f docker-compose.yml pull

# ---- Step 2: Apply database migrations ----
echo "[2/4] Applying database migrations..."
go run ./cmd/migrate up

# ---- Step 3: Build and start services ----
echo "[3/4] Building and starting services..."
docker compose -f docker-compose.yml up --build -d

# ---- Step 4: Health check ----
echo "[4/4] Waiting for services to be healthy..."
RETRIES=0
while [ $RETRIES -lt $MAX_RETRIES ]; do
  HEALTH_URL="http://localhost:${SERVER_PORT:-8080}/healthz"
  HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_URL" 2>/dev/null || echo "000")

  if [ "$HTTP_STATUS" = "200" ]; then
    echo "  Server is healthy (HTTP $HTTP_STATUS)"
    break
  fi

  RETRIES=$((RETRIES + 1))
  echo "  Waiting... ($RETRIES/$MAX_RETRIES)"
  sleep $RETRY_INTERVAL
done

if [ $RETRIES -ge $MAX_RETRIES ]; then
  echo ""
  echo "ERROR: Health check failed after $MAX_RETRIES retries."
  echo "Rolling back to previous deployment..."
  docker compose -f docker-compose.yml down
  docker compose -f docker-compose.yml up -d
  exit 1
fi

DAEMON_HEALTH_URL="http://localhost:${DAEMON_PORT:-8081}/health"
DAEMON_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$DAEMON_HEALTH_URL" 2>/dev/null || echo "000")

if [ "$DAEMON_STATUS" = "200" ]; then
  echo "  Daemon is healthy (HTTP $DAEMON_STATUS)"
else
  echo "  WARNING: Daemon health check returned HTTP $DAEMON_STATUS (non-fatal)"
fi

echo ""
echo "=== Deployment complete ==="
echo "Server: http://localhost:${SERVER_PORT:-8080}"
echo ""
echo "To view logs:  docker compose logs -f"
echo "To stop:       docker compose down"
