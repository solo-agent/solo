#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

fail() {
  echo "run-local-e2e-test: $*" >&2
  exit 1
}

source_text="$(cat "$SCRIPT_DIR/run-local-e2e.sh")"
grep -q 'daemon-e2e-' <<<"$source_text" || fail "missing isolated Daemon identity"
grep -q 'trap restore_local_stack EXIT' <<<"$source_text" || fail "missing EXIT recovery"
grep -q 'make stop' <<<"$source_text" || fail "cleanup must stop the E2E stack"
grep -q "daemon_id LIKE 'daemon-e2e-%'" <<<"$source_text" || fail "cleanup is not restricted to E2E Computers"
grep -q 'make rebuild' <<<"$source_text" || fail "cleanup must restore the ordinary stack"
grep -q 'ordinary_online' <<<"$source_text" || fail "cleanup does not verify the ordinary Daemon"
grep -q 'ordinary_health' <<<"$source_text" || fail "cleanup does not verify the ordinary Daemon health"
grep -q 'isolated_remaining' <<<"$source_text" || fail "cleanup does not verify removal of the E2E Daemon"
grep -Fq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$' <<<"$source_text" || fail "Computer ID is not validated as a hexadecimal UUID"
grep -q "UPDATE tasks SET status = 'todo', claimer_id = NULL" <<<"$source_text" || fail "cleanup does not release E2E Agent task claims"
grep -q "UPDATE agent_runs SET status = 'cancelled'" <<<"$source_text" || fail "cleanup does not cancel active E2E Agent Runs"
grep -q "UPDATE agent_sessions SET status = 'closed'" <<<"$source_text" || fail "cleanup does not close active E2E Agent Sessions"

tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/solo-e2e-wrapper-test.XXXXXX")"
trap 'rm -rf -- "$tmp_root"' EXIT
credential_file="$tmp_root/credentials.json"
printf '{"computer_id":"1234567g-1234-1234-1234-123456789abc"}\n' >"$credential_file"
set +e
validation_output="$(TMPDIR="$tmp_root/missing" SOLO_DAEMON_CREDENTIAL_FILE="$credential_file" bash "$SCRIPT_DIR/run-local-e2e.sh" self-test true 2>&1)"
validation_status=$?
set -e
[ "$validation_status" -eq 2 ] || fail "non-hex Computer ID was not rejected before startup"
grep -q 'valid UUID Computer ID' <<<"$validation_output" || fail "non-hex Computer ID rejection was not reported"

printf '{"computer_id":"ABCDEF01-2345-6789-aBcD-ef0123456789"}\n' >"$credential_file"
set +e
validation_output="$(TMPDIR="$tmp_root/missing" SOLO_DAEMON_CREDENTIAL_FILE="$credential_file" bash "$SCRIPT_DIR/run-local-e2e.sh" self-test true 2>&1)"
validation_status=$?
set -e
[ "$validation_status" -ne 2 ] || fail "valid hexadecimal UUID was rejected"
grep -q 'valid UUID Computer ID' <<<"$validation_output" && fail "valid hexadecimal UUID was rejected"

makefile_text="$(cat "$SCRIPT_DIR/../Makefile")"
for target in \
  test-e2e-agent-delivery \
  test-e2e-agent-template-credential \
  test-e2e-automation \
  test-e2e-budget-gate \
  test-e2e-agent-session-resume \
  test-e2e-agent-idle-resume \
  test-e2e-agent-scope-router \
  test-e2e-context-compaction \
  test-e2e-send-freshness \
  test-e2e-m8 \
  test-e2e-m9; do
  block="$(awk -v target="$target" '
    $0 ~ "^" target ":" { printing=1 }
    printing { print }
    printing && /^[^[:space:]#][^:]*:/ && $0 !~ "^" target ":" { exit }
  ' <<<"$makefile_text")"
  grep -q 'scripts/run-local-e2e.sh' <<<"$block" || fail "$target bypasses the E2E Daemon wrapper"
done

echo "run-local-e2e-test: ok"
