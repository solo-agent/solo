#!/usr/bin/env bash
set -euo pipefail

# ── Solo Startup Script ──────────────────────────────────────────────────────
# Starts PostgreSQL, runs migrations, launches server and daemon.
# Usage: ./scripts/start.sh

DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$DIR"

PID_DIR="$DIR/.pids"
mkdir -p "$PID_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[solo]${NC} $1"; }
warn() { echo -e "${YELLOW}[solo]${NC} $1"; }
err()  { echo -e "${RED}[solo]${NC} $1"; }

# ── .env ─────────────────────────────────────────────────────────────────────
if [ ! -f ".env" ]; then
  warn ".env not found, creating from .env.example"
  cp .env.example .env
fi

# ── PostgreSQL ────────────────────────────────────────────────────────────────
log "Checking PostgreSQL..."
if docker ps --filter name=solo-postgres --format '{{.Status}}' | grep -q healthy; then
  log "PostgreSQL is healthy"
else
  log "Starting PostgreSQL via docker compose..."
  docker compose up -d postgres

  log "Waiting for PostgreSQL to become healthy..."
  for i in $(seq 1 30); do
    if docker ps --filter name=solo-postgres --format '{{.Status}}' | grep -q healthy; then
      log "PostgreSQL is healthy"
      break
    fi
    if [ "$i" -eq 30 ]; then
      err "PostgreSQL failed to become healthy after 30s"
      exit 1
    fi
    sleep 1
  done
fi

# ── Migrations ────────────────────────────────────────────────────────────────
log "Running migrations..."
for f in migrations/*.up.sql; do
  docker exec -i solo-postgres psql -U solo -d solo < "$f" > /dev/null 2>&1 || true
done
log "Migrations done"

# ── Build ─────────────────────────────────────────────────────────────────────
log "Building server, daemon, and solo CLI..."
go build -o "$PID_DIR/server" ./cmd/server/
go build -o "$PID_DIR/daemon" ./cmd/daemon/
go build -o "$PID_DIR/solo" ./cmd/solo/
cp "$PID_DIR/solo" "$DIR/solo"

# ── Stop previous instances ───────────────────────────────────────────────────
"$DIR/scripts/stop.sh" 2>/dev/null || true

# ── Server ────────────────────────────────────────────────────────────────────
log "Starting server on :8080..."
"$PID_DIR/server" >> "$PID_DIR/../server.log" 2>&1 &
echo $! > "$PID_DIR/server.pid"
echo $! > "$PID_DIR/server.pid"

# Wait for server to become ready before starting daemon
log "Waiting for server to be ready..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:8080/readyz > /dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 30 ]; then
    err "Server failed to become ready"
    exit 1
  fi
  sleep 0.5
done

# ── Daemon ────────────────────────────────────────────────────────────────────
log "Starting daemon on :8081..."
"$PID_DIR/daemon" >> "$PID_DIR/../daemon.log" 2>&1 &
echo $! > "$PID_DIR/daemon.pid"

sleep 2

# ── Health check ──────────────────────────────────────────────────────────────
SERVER_OK=0
DAEMON_OK=0

if curl -sf http://localhost:8080/healthz > /dev/null 2>&1; then
  log "Server health check ${GREEN}OK${NC} (port 8080)"
  SERVER_OK=1
else
  err "Server health check FAILED (port 8080)"
fi

if lsof -ti:8081 > /dev/null 2>&1; then
  log "Daemon health check ${GREEN}OK${NC} (port 8081)"
  DAEMON_OK=1
else
  err "Daemon health check FAILED (port 8081)"
fi

if [ "$SERVER_OK" -eq 1 ] && [ "$DAEMON_OK" -eq 1 ]; then
  log "${GREEN}All services started ✓${NC}"
else
  err "Some services failed to start. Check logs with: tail -f nohup.out"
  exit 1
fi
