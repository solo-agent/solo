#!/usr/bin/env bash
# Start server, daemon, and frontend in the background.
# Idempotent: skips services that are already running.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

PID_DIR="$REPO_ROOT/.pids"
mkdir -p "$PID_DIR"

STARTED_PIDS=()
STARTED_PIDFILES=()

descendants_of() {
  local parent="$1"
  local child
  for child in $(pgrep -P "$parent" 2>/dev/null || true); do
    descendants_of "$child"
    printf '%s\n' "$child"
  done
}

cleanup_started() {
  local index pid pidfile descendants
  if [ "${#STARTED_PIDS[@]}" -eq 0 ]; then
    return
  fi
  echo "Cleaning up services started by this attempt..." >&2
  for index in "${!STARTED_PIDS[@]}"; do
    pid="${STARTED_PIDS[$index]}"
    pidfile="${STARTED_PIDFILES[$index]}"
    if kill -0 "$pid" 2>/dev/null; then
      descendants="$(descendants_of "$pid")"
      if [ -n "$descendants" ]; then
        while IFS= read -r child; do
          kill "$child" 2>/dev/null || true
        done <<< "$descendants"
      fi
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$pidfile"
  done
}

cleanup_on_exit() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    cleanup_started
  fi
}
trap cleanup_on_exit EXIT

SERVER_PORT="${SERVER_PORT:-8080}"
DAEMON_PORT="${DAEMON_PORT:-8081}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
NEXT_DEV_ARGS="${NEXT_DEV_ARGS:-}"
SERVER_URL="${DAEMON_SERVER_URL:-http://127.0.0.1:$SERVER_PORT}"
FRONTEND_URL="http://127.0.0.1:$FRONTEND_PORT"
if [ "$SERVER_URL" = "http://localhost:$SERVER_PORT" ]; then
  SERVER_URL="http://127.0.0.1:$SERVER_PORT"
fi

is_running() {
  local pidfile="$1"
  [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null
}

require_free_port() {
  local service="$1"
  local port="$2"
  if lsof -nP -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "ERROR: $service port $port is already in use; refusing to replace its owner" >&2
    exit 1
  fi
}

# ── Server ─────────────────────────────────────────────────────────────────
if is_running "$PID_DIR/server.pid"; then
  echo "Server already running"
else
  require_free_port "Server" "$SERVER_PORT"
  if [ ! -f "$PID_DIR/server" ]; then
    echo "Building server..."
    go build -o "$PID_DIR/server" ./cmd/server/
  fi
  nohup env PORT="$SERVER_PORT" "$PID_DIR/server" > server.log 2>&1 &
  echo $! > "$PID_DIR/server.pid"
  STARTED_PIDS+=("$(cat "$PID_DIR/server.pid")")
  STARTED_PIDFILES+=("$PID_DIR/server.pid")
  ok=0
  for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:$SERVER_PORT/readyz" >/dev/null 2>&1; then ok=1; break; fi
    sleep 0.5
  done
  if [ "$ok" -ne 1 ]; then
    echo "ERROR: Server on :$SERVER_PORT did not become ready, recent logs:" >&2
    tail -20 server.log >&2
    exit 1
  fi
  echo "Server :$SERVER_PORT ✓"
fi

# ── Daemon ─────────────────────────────────────────────────────────────────
if is_running "$PID_DIR/daemon.pid"; then
  echo "Daemon already running"
else
  require_free_port "Daemon" "$DAEMON_PORT"
  if [ ! -f "$PID_DIR/daemon" ]; then
    echo "Building daemon..."
    go build -o "$PID_DIR/daemon" ./cmd/daemon/
  fi
  if [ ! -f "$PID_DIR/solo" ]; then
    echo "Building solo CLI..."
    go build -o "$PID_DIR/solo" ./cmd/solo/
  fi
  nohup env DAEMON_PORT="$DAEMON_PORT" DAEMON_SERVER_URL="$SERVER_URL" "$PID_DIR/daemon" > daemon.log 2>&1 &
  echo $! > "$PID_DIR/daemon.pid"
  STARTED_PIDS+=("$(cat "$PID_DIR/daemon.pid")")
  STARTED_PIDFILES+=("$PID_DIR/daemon.pid")
  ok=0
  for i in $(seq 1 30); do
    if ! kill -0 "$(cat "$PID_DIR/daemon.pid")" 2>/dev/null; then
      break
    fi
    if curl -sf "http://127.0.0.1:$DAEMON_PORT/health" >/dev/null 2>&1 && \
       curl -sf "http://127.0.0.1:$SERVER_PORT/readyz" >/dev/null 2>&1; then
      ok=1
      break
    fi
    sleep 0.5
  done
  if [ "$ok" -ne 1 ]; then
    echo "ERROR: Daemon failed to start, recent logs:" >&2
    tail -20 daemon.log >&2
    exit 1
  fi
  echo "Daemon :$DAEMON_PORT ✓"
fi

# ── Frontend ───────────────────────────────────────────────────────────────
if is_running "$PID_DIR/frontend.pid"; then
  echo "Frontend already running"
else
  require_free_port "Frontend" "$FRONTEND_PORT"
  FRONTEND_DEV_ARGS=()
  if [ -n "$NEXT_DEV_ARGS" ]; then
    read -r -a FRONTEND_DEV_ARGS <<< "$NEXT_DEV_ARGS"
  fi
  cd frontend
  nohup env PORT="$FRONTEND_PORT" \
    NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-http://127.0.0.1:$SERVER_PORT}" \
    npm run dev -- "${FRONTEND_DEV_ARGS[@]}" > "$REPO_ROOT/frontend.log" 2>&1 &
  FRONTEND_PID=$!
  cd "$REPO_ROOT"
  echo "$FRONTEND_PID" > "$PID_DIR/frontend.pid"
  STARTED_PIDS+=("$FRONTEND_PID")
  STARTED_PIDFILES+=("$PID_DIR/frontend.pid")
  ok=0
  for i in $(seq 1 60); do
    if ! kill -0 "$FRONTEND_PID" 2>/dev/null; then
      break
    fi
    if curl -sf "$FRONTEND_URL" >/dev/null 2>&1; then ok=1; break; fi
    sleep 0.5
  done
  if [ "$ok" -ne 1 ]; then
    echo "ERROR: Frontend on :$FRONTEND_PORT did not become ready, recent logs:" >&2
    tail -20 frontend.log >&2
    exit 1
  fi
  echo "Frontend :$FRONTEND_PORT ✓"
fi

echo ""
echo "=== All services started ==="
echo "  http://localhost:$FRONTEND_PORT"
trap - EXIT
