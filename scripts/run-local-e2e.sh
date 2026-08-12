#!/usr/bin/env bash
# Run a real local E2E suite with a disposable Daemon identity, then restore
# the ordinary make-managed local stack even when the suite fails.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <scenario> <command> [args...]" >&2
  exit 2
fi

SCENARIO="$(printf '%s' "$1" | tr -cd 'a-zA-Z0-9_-')"
shift
if [ -z "$SCENARIO" ]; then
  echo "ERROR: E2E scenario must contain a letter, number, underscore, or dash" >&2
  exit 2
fi

E2E_TMP_ROOT="${TMPDIR:-/tmp}"
E2E_TMP_ROOT="${E2E_TMP_ROOT%/}"
E2E_STATE_DIR="$(mktemp -d "$E2E_TMP_ROOT/solo-e2e-daemon.XXXXXX")"
E2E_DAEMON_ID="daemon-e2e-${SCENARIO}-$(date +%s)-$$"
ORDINARY_DAEMON_ID="$(sed -n 's/^DAEMON_ID=//p' "$REPO_ROOT/.env" 2>/dev/null | tail -1 | tr -d '"'"'"'[:space:]')"
ORDINARY_DAEMON_ID="${ORDINARY_DAEMON_ID:-daemon-01}"
case "$ORDINARY_DAEMON_ID" in
  *[!a-zA-Z0-9_-]*)
    echo "ERROR: ordinary DAEMON_ID contains unsupported characters" >&2
    exit 2
    ;;
esac
RESTORED=0

restore_local_stack() {
  local test_status=$?
  local cleanup_status=0
  local restore_status
  trap - EXIT INT TERM HUP

  if [ "$RESTORED" -eq 0 ]; then
    RESTORED=1
    echo "=== Restoring ordinary local Daemon ==="
    set +e
    (cd "$REPO_ROOT" && make stop)
    cleanup_status=$?
    docker exec "${SOLO_POSTGRES_CONTAINER:-solo-postgres}" \
      psql -U "${POSTGRES_USER:-solo}" -d "${POSTGRES_DB:-solo}" \
      -v ON_ERROR_STOP=1 \
      -c "UPDATE agents SET is_active = false, updated_at = now() WHERE runtime_id = (SELECT id::text FROM computers WHERE daemon_id = '$E2E_DAEMON_ID' AND daemon_id LIKE 'daemon-e2e-%'); DELETE FROM computers WHERE daemon_id = '$E2E_DAEMON_ID' AND daemon_id LIKE 'daemon-e2e-%';"
    if [ "$?" -ne 0 ]; then
      cleanup_status=1
      echo "ERROR: could not remove the isolated E2E Computer record" >&2
    fi
    (
      unset \
        DAEMON_ID \
        DAEMON_SERVER_URL \
        SOLO_DAEMON_CREDENTIAL_FILE \
        SOLO_E2E_DAEMON_ID \
        INTERNAL_TOKEN_SECRET \
        AGENT_SESSION_IDLE_TTL \
        THINKING_SESSION_IDLE_TTL \
        SESSION_IDLE_SWEEP_INTERVAL \
        AGENT_SEND_RATE_LIMIT \
        AGENT_SEND_RATE_WINDOW \
        AGENT_CASCADE_THRESHOLD \
        AGENT_CASCADE_WINDOW \
        AGENT_CASCADE_COOLDOWN
      cd "$REPO_ROOT" && make rebuild
    )
    restore_status=$?
    if [ "$restore_status" -eq 0 ]; then
      ordinary_health="$(curl -sf "http://127.0.0.1:${DAEMON_PORT:-8081}/health" 2>/dev/null || true)"
      ordinary_online="$(docker exec "${SOLO_POSTGRES_CONTAINER:-solo-postgres}" \
        psql -U "${POSTGRES_USER:-solo}" -d "${POSTGRES_DB:-solo}" -tA \
        -c "SELECT count(*) FROM computers WHERE daemon_id = '$ORDINARY_DAEMON_ID' AND status = 'online';" 2>/dev/null | tr -d '[:space:]')"
      isolated_remaining="$(docker exec "${SOLO_POSTGRES_CONTAINER:-solo-postgres}" \
        psql -U "${POSTGRES_USER:-solo}" -d "${POSTGRES_DB:-solo}" -tA \
        -c "SELECT count(*) FROM computers WHERE daemon_id = '$E2E_DAEMON_ID';" 2>/dev/null | tr -d '[:space:]')"
      if [[ "$ordinary_health" != *'"status":"ok"'* ]] || \
         [[ "$ordinary_health" != *'"control_connected":true'* ]] || \
         [ "$ordinary_online" != "1" ] || \
         [ "$isolated_remaining" != "0" ]; then
        restore_status=1
        echo "ERROR: local Daemon restoration did not reach the expected database state" >&2
      fi
    fi
    set -e
    if [ "$restore_status" -ne 0 ]; then
      echo "ERROR: E2E finished, but the ordinary local Daemon could not be restored" >&2
      test_status="$restore_status"
    elif [ "$cleanup_status" -ne 0 ] && [ "$test_status" -eq 0 ]; then
      test_status="$cleanup_status"
    fi
  fi

  case "$E2E_STATE_DIR" in
    "$E2E_TMP_ROOT"/solo-e2e-daemon.*) rm -rf -- "$E2E_STATE_DIR" ;;
    *) echo "WARNING: refusing to remove unexpected E2E state directory: $E2E_STATE_DIR" >&2 ;;
  esac
  exit "$test_status"
}

trap restore_local_stack EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

echo "=== Starting isolated E2E Daemon: $E2E_DAEMON_ID ==="
(
  export DAEMON_ID="$E2E_DAEMON_ID"
  export DAEMON_SERVER_URL="http://127.0.0.1:8080"
  export SOLO_DAEMON_CREDENTIAL_FILE="$E2E_STATE_DIR/credentials.json"
  export SOLO_E2E_DAEMON_ID="$E2E_DAEMON_ID"
  unset SOLO_COMPUTER_ID SOLO_COMPUTER_CREDENTIAL SOLO_ENROLLMENT_TOKEN

  cd "$REPO_ROOT"
  MAKE_ARGS=(
    rebuild
    "DAEMON_ID=$DAEMON_ID"
    "DAEMON_SERVER_URL=$DAEMON_SERVER_URL"
    "SOLO_DAEMON_CREDENTIAL_FILE=$SOLO_DAEMON_CREDENTIAL_FILE"
    "SOLO_COMPUTER_ID="
    "SOLO_COMPUTER_CREDENTIAL="
    "SOLO_ENROLLMENT_TOKEN="
  )
  for key in \
    INTERNAL_TOKEN_SECRET \
    AGENT_SESSION_IDLE_TTL \
    THINKING_SESSION_IDLE_TTL \
    SESSION_IDLE_SWEEP_INTERVAL \
    AGENT_SEND_RATE_LIMIT \
    AGENT_SEND_RATE_WINDOW \
    AGENT_CASCADE_THRESHOLD \
    AGENT_CASCADE_WINDOW \
    AGENT_CASCADE_COOLDOWN; do
    if [ -n "${!key+x}" ]; then
      MAKE_ARGS+=("$key=${!key}")
    fi
  done
  make "${MAKE_ARGS[@]}"
  cd frontend
  "$@"
)
