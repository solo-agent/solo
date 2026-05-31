#!/usr/bin/env bash
set -euo pipefail

# Database migration script for Solo
# Uses golang-migrate CLI.
# Usage: ./scripts/migrate.sh [up|down|drop|force|version]

DIR="$(cd "$(dirname "$0")/.." && pwd)"
MIGRATIONS_DIR="${DIR}/migrations"
DATABASE_URL="${DATABASE_URL:-postgres://solo:solo@localhost:5432/solo?sslmode=disable}"

# Detect golang-migrate binary
MIGRATE=""
if command -v migrate &>/dev/null; then
    MIGRATE="migrate"
elif [ -f "${DIR}/bin/migrate" ]; then
    MIGRATE="${DIR}/bin/migrate"
else
    echo "Installing golang-migrate..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
    MIGRATE="migrate"
fi

CMD="${1:-up}"

case "$CMD" in
    up|down|drop|force|version)
        echo "Running migrate $CMD..."
        "$MIGRATE" -database "$DATABASE_URL" -path "$MIGRATIONS_DIR" "$CMD" "${2:-}"
        ;;
    create)
        if [ -z "${2:-}" ]; then
            echo "Usage: $0 create <name>"
            exit 1
        fi
        echo "Creating migration: $2"
        "$MIGRATE" create -ext sql -dir "$MIGRATIONS_DIR" -seq "$2"
        ;;
    *)
        echo "Usage: $0 [up|down|drop|force|version|create]"
        exit 1
        ;;
esac
