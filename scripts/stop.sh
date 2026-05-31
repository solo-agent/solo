#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$DIR/.pids"

GREEN='\033[0;32m'
NC='\033[0m'

log() { echo -e "${GREEN}[solo]${NC} $1"; }

stop_by_pid() {
  local pid_file="$PID_DIR/$1"
  local name="$2"
  if [ -f "$pid_file" ]; then
    local pid=$(cat "$pid_file")
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      log "Stopped $name (pid=$pid)"
    fi
    rm -f "$pid_file"
  fi
}

stop_by_pid "server.pid" "server"
stop_by_pid "daemon.pid" "daemon"

# Fallback: kill by port
for port in 8080 8081; do
  pids=$(lsof -ti :"$port" 2>/dev/null || true)
  if [ -n "$pids" ]; then
    echo "$pids" | xargs kill 2>/dev/null || true
    log "Killed process on port $port"
  fi
done

log "All services stopped"
