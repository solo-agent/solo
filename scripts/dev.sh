#!/usr/bin/env bash
# Bootstrap a fresh checkout and start every service in one shot.
# Idempotent: safe to re-run; skips work that's already done.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ── 1. Prerequisites ───────────────────────────────────────────────────────
missing=()
command -v go      >/dev/null 2>&1 || missing+=("go")
command -v node    >/dev/null 2>&1 || missing+=("node")
command -v npm     >/dev/null 2>&1 || missing+=("npm")
command -v docker  >/dev/null 2>&1 || missing+=("docker")
if [ "${#missing[@]}" -gt 0 ]; then
  echo "✗ Missing prerequisites: ${missing[*]}" >&2
  echo "  Please install: Go 1.22+, Node.js 20+, Docker" >&2
  exit 1
fi

# ── 2. Env file ────────────────────────────────────────────────────────────
if [ ! -f .env ]; then
  echo "==> Creating .env from .env.example..."
  cp .env.example .env
fi

# Load .env so DATABASE_URL etc. are visible to child processes (e.g. migrate).
set -a
# shellcheck disable=SC1091
. .env
set +a

# ── 3. Frontend dependencies ───────────────────────────────────────────────
if [ ! -d frontend/node_modules ]; then
  echo "==> Installing frontend dependencies..."
  (cd frontend && npm install)
fi

# ── 4. Database ────────────────────────────────────────────────────────────
bash scripts/ensure-postgres.sh

# ── 5. Migrations ──────────────────────────────────────────────────────────
echo "==> Running migrations..."
go run ./cmd/migrate up

# ── 6. Start services ──────────────────────────────────────────────────────
echo ""
bash scripts/start-services.sh
