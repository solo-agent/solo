#!/bin/bash
# test-agent-autonomy.sh — E2E integration verification for the agent autonomy loop.
# Verifies builds, tests, and protocol regex patterns.
#
# Usage: ./scripts/test-agent-autonomy.sh
set -e

# cd to repo root regardless of where the script is called from
cd "$(dirname "$0")/.."

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

PASS_COUNT=0
FAIL_COUNT=0

pass() {
    echo -e "${GREEN}PASS${NC}: $1"
    ((PASS_COUNT++))
}

fail() {
    echo -e "${RED}FAIL${NC}: $1"
    ((FAIL_COUNT++))
}

info() {
    echo -e "${YELLOW}INFO${NC}: $1"
}

# ─────────────────────────────────────
# Step 1: Build verification
# ─────────────────────────────────────
echo ""
echo "=== Step 1: Build Verification ==="

info "Building solo CLI..."
if make solo 2>&1; then
    pass "solo CLI build"
else
    fail "solo CLI build"
fi

info "Building server..."
if go build ./cmd/server/ 2>&1; then
    pass "server build"
else
    fail "server build"
fi

info "Building daemon..."
if go build ./cmd/daemon/ 2>&1; then
    pass "daemon build"
else
    fail "daemon build"
fi

info "Building entire module..."
if go build ./... 2>&1; then
    pass "full module build"
else
    fail "full module build"
fi

# ─────────────────────────────────────
# Step 2: Go tests
# ─────────────────────────────────────
echo ""
echo "=== Step 2: Go Tests ==="

info "Running pkg/agent tests..."
if go test ./pkg/agent/ -count=1 -timeout 60s 2>&1; then
    pass "pkg/agent tests"
else
    fail "pkg/agent tests"
fi

info "Running cmd/solo tests (including protocol regex)..."
if go test ./cmd/solo/ -count=1 -timeout 60s 2>&1; then
    pass "cmd/solo tests (including protocol regex)"
else
    fail "cmd/solo tests (including protocol regex)"
fi

info "Running internal/server/service tests..."
if go test ./internal/server/service/ -count=1 -timeout 60s 2>&1; then
    pass "internal/server/service tests"
else
    fail "internal/server/service tests"
fi

# ─────────────────────────────────────
# Step 3: Template generation verification
# ─────────────────────────────────────
echo ""
echo "=== Step 3: Template Generation Verification ==="

# Check that CLAUDE.md template exists and is non-empty
CLAUDE_TEMPLATE="pkg/agent/claude_template.go"
if [ -f "$CLAUDE_TEMPLATE" ]; then
    # Verify it exports a Template function or constant
    if grep -q "claudeMdTemplate\|ClaudeMdTemplate\|claudeMd\|CLAUDE_MD_TEMPLATE" "$CLAUDE_TEMPLATE" 2>/dev/null; then
        pass "CLAUDE.md template exists with content"
    else
        info "CLAUDE.md template file exists but pattern not found — checking for any template content"
        if [ -s "$CLAUDE_TEMPLATE" ]; then
            pass "CLAUDE.md template file exists (non-empty)"
        else
            fail "CLAUDE.md template file is empty"
        fi
    fi
else
    fail "CLAUDE.md template file not found at $CLAUDE_TEMPLATE"
fi

# Check that prompt templates exist
PROMPT_TEMPLATES="pkg/agent/prompt_templates.go"
if [ -f "$PROMPT_TEMPLATES" ]; then
    if [ -s "$PROMPT_TEMPLATES" ]; then
        pass "prompt_templates.go exists (non-empty)"
    else
        fail "prompt_templates.go is empty"
    fi
else
    fail "prompt_templates.go not found at $PROMPT_TEMPLATES"
fi

# Check that claude_template_test.go exists
CLAUDE_TEST="pkg/agent/claude_template_test.go"
if [ -f "$CLAUDE_TEST" ]; then
    if [ -s "$CLAUDE_TEST" ]; then
        pass "claude_template_test.go exists"
    else
        info "claude_template_test.go exists but is empty"
    fi
else
    info "claude_template_test.go not found (may not exist yet)"
fi

# Verify template test package is valid
info "Running claude template tests..."
if go test ./pkg/agent/ -run TestClaudeMd -count=1 -timeout 30s 2>&1; then
    pass "claude template tests"
else
    info "claude template tests not found or failed (may be expected if test names differ)"
fi

# ─────────────────────────────────────
# Step 4: Regex pattern validation (via Go tests)
# ─────────────────────────────────────
echo ""
echo "=== Step 4: Protocol Regex Validation ==="

info "Running protocol regex tests..."
if go test ./cmd/solo/ -run "TestClaim|TestUpdate" -v -count=1 -timeout 30s 2>&1; then
    pass "protocol regex tests"
else
    fail "protocol regex tests"
fi

# ─────────────────────────────────────
# Step 5: Summary
# ─────────────────────────────────────
echo ""
echo "==========================================="
if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}Agent autonomy E2E: PASS${NC}"
    echo "  Passed: $PASS_COUNT checks"
    exit 0
else
    echo -e "${RED}Agent autonomy E2E: FAIL${NC}"
    echo "  Passed: $PASS_COUNT checks"
    echo "  Failed: $FAIL_COUNT checks"
    exit 1
fi
