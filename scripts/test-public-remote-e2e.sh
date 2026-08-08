#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONTAINER="solo-mailpit-e2e-$$"

cleanup() {
  set +e
  docker rm -f "$CONTAINER" >/dev/null 2>&1
  cd "$ROOT"
  make rebuild >/dev/null
}
trap cleanup EXIT

docker run -d --name "$CONTAINER" \
  -p 127.0.0.1::1025 -p 127.0.0.1::8025 \
  axllent/mailpit:latest >/dev/null

SMTP_ADDR="$(docker port "$CONTAINER" 1025/tcp)"
HTTP_ADDR="$(docker port "$CONTAINER" 8025/tcp)"
SMTP_PORT="${SMTP_ADDR##*:}"
HTTP_PORT="${HTTP_ADDR##*:}"

cd "$ROOT"
SMTP_HOST=127.0.0.1 \
SMTP_PORT="$SMTP_PORT" \
SMTP_FROM='Solo <noreply@solo.local>' \
SMTP_TLS=starttls \
SOLO_DEV_AUTH_CODE=123456 \
make rebuild

cd "$ROOT/frontend"
CI=1 \
SOLO_E2E_PUBLIC_REMOTE=1 \
SOLO_E2E_AUTH_CODE=123456 \
SOLO_E2E_MAILPIT_URL="http://127.0.0.1:$HTTP_PORT" \
npx playwright test e2e/public-remote-onboarding.spec.ts --workers=1
