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

makefile_text="$(cat "$SCRIPT_DIR/../Makefile")"
for target in \
  test-e2e-agent-delivery \
  test-e2e-agent-template-credential \
  test-e2e-automation \
  test-e2e-budget-gate \
  test-e2e-agent-session-resume \
  test-e2e-agent-idle-resume \
  test-e2e-agent-scope-router \
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
