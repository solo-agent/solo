#!/usr/bin/env bash
# =============================================================================
# Solo - Secret Key Generation Script
# =============================================================================
# Usage:
#   ./scripts/gen-secret.sh              # Generate all secrets
#   ./scripts/gen-secret.sh jwt           # Generate JWT secret only
#   ./scripts/gen-secret.sh internal      # Generate internal token secret only
#   ./scripts/gen-secret.sh db            # Generate DB password only
#
# Output is printed to stdout. Redirect to append to your .env file:
#   ./scripts/gen-secret.sh >> .env.production
# =============================================================================

set -euo pipefail

# Generate a 64-character random hex string
gen_hex() {
  openssl rand -hex 32
}

# Generate a 32-character random base64 string (URL-safe)
gen_base64() {
  openssl rand -base64 24 | tr '+/' '-_' | tr -d '='
}

# Generate a random PostgreSQL password
gen_db_password() {
  openssl rand -base64 18 | tr '+/' '-_'
}

CMD="${1:-all}"

case "$CMD" in
  jwt)
    echo "# JWT Secret (used for signing access and refresh tokens)"
    echo "JWT_SECRET=$(gen_hex)"
    echo ""
    ;;
  internal)
    echo "# Internal Token Secret (used for server-daemon communication)"
    echo "INTERNAL_TOKEN_SECRET=$(gen_base64)"
    echo ""
    ;;
  db)
    echo "# Database Password"
    echo "DB_PASSWORD=$(gen_db_password)"
    echo ""
    ;;
  all)
    echo "# ============================================"
    echo "# Solo - Production Secrets"
    echo "# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "# ============================================"
    echo ""
    echo "# JWT Secret (required)"
    echo "JWT_SECRET=$(gen_hex)"
    echo ""
    echo "# Internal Token Secret (required)"
    echo "INTERNAL_TOKEN_SECRET=$(gen_base64)"
    echo ""
    echo "# Database Password (set this in POSTGRES_PASSWORD too)"
    echo "DB_PASSWORD=$(gen_db_password)"
    echo ""
    echo "# Important: Run this script once and save the output securely."
    echo "# If you lose these secrets, all existing sessions will be invalidated."
    ;;
  *)
    echo "Usage: $0 [jwt|internal|db|all]"
    exit 1
    ;;
esac
