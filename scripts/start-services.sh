#!/usr/bin/env bash
# Start server, daemon, and frontend in the background.
# Idempotent: skips services that are already running.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

PID_DIR="$REPO_ROOT/.pids"
mkdir -p "$PID_DIR"

is_running() {
  local pidfile="$1"
  [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null
}

# ── Server ─────────────────────────────────────────────────────────────────
if is_running "$PID_DIR/server.pid"; then
  echo "Server already running"
else
  if [ ! -f "$PID_DIR/server" ]; then
    echo "Building server..."
    go build -o "$PID_DIR/server" ./cmd/server/
  fi
  "$PID_DIR/server" > server.log 2>&1 &
  echo $! > "$PID_DIR/server.pid"
  ok=0
  for i in $(seq 1 30); do
    if curl -sf http://localhost:8080/readyz >/dev/null 2>&1; then ok=1; break; fi
    sleep 0.5
  done
  if [ "$ok" -ne 1 ]; then
    echo "ERROR: Server on :8080 did not become ready, recent logs:" >&2
    tail -20 server.log >&2
    exit 1
  fi
  echo "Server :8080 ✓"
fi

# ── Daemon ─────────────────────────────────────────────────────────────────
if is_running "$PID_DIR/daemon.pid"; then
  echo "Daemon already running"
else
  if [ ! -f "$PID_DIR/daemon" ]; then
    echo "Building daemon..."
    go build -o "$PID_DIR/daemon" ./cmd/daemon/
    go build -o "$PID_DIR/solo" ./cmd/solo/
  fi
  "$PID_DIR/daemon" > daemon.log 2>&1 &
  echo $! > "$PID_DIR/daemon.pid"
  sleep 2
  if ! kill -0 "$(cat "$PID_DIR/daemon.pid")" 2>/dev/null; then
    echo "ERROR: Daemon failed to start, recent logs:" >&2
    tail -20 daemon.log >&2
    exit 1
  fi
  echo "Daemon :8081 ✓"
fi

# ── Frontend ───────────────────────────────────────────────────────────────
if is_running "$PID_DIR/frontend.pid"; then
  echo "Frontend already running"
else
  cd frontend
  npm run dev > /dev/null 2>&1 &
  FRONTEND_PID=$!
  cd "$REPO_ROOT"
  echo "$FRONTEND_PID" > "$PID_DIR/frontend.pid"
  echo "Frontend :3000 ✓"
fi

echo ""
echo "=== All services started ==="
echo "  http://localhost:3000"
