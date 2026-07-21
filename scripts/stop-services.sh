#!/usr/bin/env bash
# Stop only processes proven to belong to this checkout. Refuse to claim a
# configured port when an unrelated process is listening on it.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$REPO_ROOT/.pids"
SERVER_PORT="${SERVER_PORT:-8080}"
DAEMON_PORT="${DAEMON_PORT:-8081}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"

echo "=== Stopping all services ==="

descendants_of() {
  local parent="$1"
  local child
  for child in $(pgrep -P "$parent" 2>/dev/null || true); do
    descendants_of "$child"
    printf '%s\n' "$child"
  done
}

owns_process() {
  local service="$1"
  local pid="$2"
  local command cwd

  command="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  case "$service" in
    server|daemon)
      [[ "$command" == *"$PID_DIR/$service"* ]]
      ;;
    frontend)
      cwd="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)"
      [[ "$cwd" == "$REPO_ROOT/frontend" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

stop_service() {
  local service="$1"
  local pidfile="$PID_DIR/$service.pid"
  local pid descendants

  if [ ! -f "$pidfile" ]; then
    echo "$service not running (no pid file)"
    return
  fi

  pid="$(sed -n '1p' "$pidfile")"
  if [[ ! "$pid" =~ ^[0-9]+$ ]] || ! kill -0 "$pid" 2>/dev/null; then
    echo "$service not running (stale pid file)"
    rm -f "$pidfile"
    return
  fi

  if ! owns_process "$service" "$pid"; then
    echo "ERROR: refusing to stop PID $pid from $pidfile: process is not owned by this checkout" >&2
    return 1
  fi

  descendants="$(descendants_of "$pid")"
  if [ -n "$descendants" ]; then
    while IFS= read -r child; do
      kill "$child" 2>/dev/null || true
    done <<< "$descendants"
  fi
  kill "$pid" 2>/dev/null || true
  rm -f "$pidfile"
  echo "$service stopped"
}

stop_service frontend
stop_service daemon
stop_service server

for entry in "server:$SERVER_PORT" "daemon:$DAEMON_PORT" "frontend:$FRONTEND_PORT"; do
  service="${entry%%:*}"
  port="${entry##*:}"
  for _ in $(seq 1 30); do
    if ! lsof -nP -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
  if lsof -nP -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "ERROR: $service port $port is still owned by another process; refusing to stop it" >&2
    exit 1
  fi
done

echo "=== All services stopped ==="
