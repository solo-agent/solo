#!/usr/bin/env bash
# ============================================================================
# audit-brutal.sh — neo-brutalism style linter
# Scans frontend/ for tokens and patterns that violate the design system.
# Exits 0 if clean, 1 if any violations found.
# ============================================================================
set -u

# Resolve the directory of this script so the audit can run from anywhere
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$FRONTEND_DIR" || exit 2

# Paths to audit (the fe2 worktree owns app/ + the few components/ files
# it was instructed to refactor; fe1 boundary files are excluded because
# they are owned by a parallel worktree and would block the gate)
SCAN_PATHS=(
  "app"
  "components/connection-banner.tsx"
  "components/network-status.tsx"
  "components/inbox/inbox-badge.tsx"
  "components/dashboard/channel-list.tsx"
  "components/dashboard/dm-list.tsx"
  "components/dashboard/create-dm-modal.tsx"
  "components/dashboard/member-list.tsx"
  "components/tasks/tasks-left-column.tsx"
)

# Compose a single find expression that respects all scan paths
FIND_EXPR=()
FIND_TRAILING_O=0
for p in "${SCAN_PATHS[@]}"; do
  if [[ $FIND_TRAILING_O -eq 1 ]]; then
    FIND_EXPR+=( -o )
  fi
  FIND_EXPR+=( -path "$p" )
  FIND_TRAILING_O=1
done

# Use grep -E to find offending tokens, then count + show context.
# Each rule is grep -nE pattern | grep -v <ignore patterns>
violations=0
report=""
# Bash 3.2 (macOS default) does not support `unset arr[-1]`. Track trailing -o
# separately so we can build the find expression portably.

# Helper: run a rule. Arg1 = description, rest = grep args (already chained)
run_rule() {
  local desc="$1"
  shift
  # Use a subshell so the pipeline does not leak; capture stdout
  local out
  out="$("$@" 2>/dev/null || true)"
  if [[ -n "$out" ]]; then
    violations=$((violations + 1))
    report+="\n--- $desc ---\n$out\n"
  fi
}

# 1. Rounded corners — anything between none and full is forbidden
grep_rounded() {
  grep -rnE '\brounded-(md|lg|sm|xl|2xl|3xl)\b' "${SCAN_PATHS[@]}" \
    --include='*.tsx' --include='*.ts' --include='*.css' 2>/dev/null || true
}
run_rule "rounded-{md,lg,sm,xl,2xl,3xl}" grep_rounded

# 2. Tailwind default color scales (we only use brutal-* tokens)
grep_default_colors() {
  grep -rnE '\b(bg|text|border|ring|fill|stroke)-(green|red|amber|yellow|gray|blue|orange|pink|cyan|lime|lavender|stone|emerald|teal|indigo|purple|fuchsia|rose|sky|violet|slate|zinc|neutral)-(50|100|200|300|400|500|600|700|800|900|950)\b' \
    "${SCAN_PATHS[@]}" --include='*.tsx' --include='*.ts' --include='*.css' 2>/dev/null \
    | grep -vE 'globals\.brutal\.css' || true
}
run_rule "default Tailwind color scale (use brutal-*)" grep_default_colors

# 3. text-gray-N (specific text util)
grep_text_gray() {
  grep -rnE '\btext-gray-[0-9]+\b' "${SCAN_PATHS[@]}" \
    --include='*.tsx' --include='*.ts' --include='*.css' 2>/dev/null || true
}
run_rule "text-gray-N (use text-muted-foreground or text-brutal-stone)" grep_text_gray

# 4. Border with alpha — borders must be solid
grep_alpha_border() {
  grep -rnE '\bborder-black\/[0-9]+\b' "${SCAN_PATHS[@]}" \
    --include='*.tsx' --include='*.ts' --include='*.css' 2>/dev/null || true
}
run_rule "border-black/N (alpha) (use solid border-black)" grep_alpha_border

# 5. Soft shadows — anything not shadow-brutal-*
grep_soft_shadow() {
  grep -rnE '\bshadow-(lg|md|2xl|xl|sm|inner)\b' "${SCAN_PATHS[@]}" \
    --include='*.tsx' --include='*.ts' --include='*.css' 2>/dev/null \
    | grep -vE 'globals\.brutal\.css' || true
}
run_rule "soft shadow (lg|md|2xl|xl|sm|inner)" grep_soft_shadow

# 6. Backdrop blur — forbidden in neo-brutalism
grep_backdrop_blur() {
  grep -rnE '\bbackdrop-blur(-[a-z0-9]+)?\b' "${SCAN_PATHS[@]}" \
    --include='*.tsx' --include='*.ts' --include='*.css' 2>/dev/null \
    | grep -vE 'globals\.brutal\.css' || true
}
run_rule "backdrop-blur (forbidden in neo-brutalism)" grep_backdrop_blur

# 7. Emoji as icon — informal emoji in JSX
grep_emoji_icon() {
  grep -rnE '[👤📁⚙️🚀💡✨🎉🔔🛠️🎨📊📈📉🤖🧠🔍📌⭐🔥💬❤️👍👎🎯📎🔗]' \
    "${SCAN_PATHS[@]}" --include='*.tsx' --include='*.ts' 2>/dev/null || true
}
run_rule "emoji as icon (use lucide-react)" grep_emoji_icon

# 8. Raw <button> with btn-brutal class (should use Button component)
grep_raw_brutal_button() {
  grep -rnE '<button[^>]*className="[^"]*btn-brutal' "${SCAN_PATHS[@]}" \
    --include='*.tsx' 2>/dev/null || true
}
run_rule "raw <button> with btn-brutal class (use <Button> component)" grep_raw_brutal_button

# 9. Animate-spin without proper class (raw spinner)
grep_raw_spinner() {
  # Catches cases where animate-spin is applied to a non-Spinner / non-Loader2
  # element. Loaders are an allowed case (Loader2 from lucide-react).
  grep -rnE 'animate-spin[^"]*"' "${SCAN_PATHS[@]}" --include='*.tsx' 2>/dev/null \
    | grep -vE 'Loader2|loading\.|loader|<Loader2|animate-spin rounded' || true
}
run_rule "raw animate-spin (use Spinner or Loader2)" grep_raw_spinner

# Output
if [[ $violations -gt 0 ]]; then
  echo -e "❌ audit found $violations rule violation(s):\n$report" >&2
  exit 1
fi

echo "✅ audit clean (${#SCAN_PATHS[@]} paths scanned)"
exit 0
