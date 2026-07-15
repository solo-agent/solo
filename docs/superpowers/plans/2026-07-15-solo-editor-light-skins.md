# SOLO Editor-Inspired Light Skins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace SOLO's eight non-classic light palettes with the approved editor-inspired colors while preserving all existing theme behavior.

**Architecture:** Keep the nine existing internal theme IDs and browser persistence unchanged. Extend the current source assertion first, then replace only CSS variables and translated display names.

**Tech Stack:** Next.js, TypeScript, Tailwind CSS variables, Node assertion script, Playwright CLI

## Global Constraints

- Keep `classic` unchanged and default.
- Keep all nine skins light-only and stored under `solo.skin` in the current browser.
- Keep black borders, hard shadows, white cards, existing selectors, and responsive layout.
- Add no dependency, backend setting, migration, custom picker, or dark mode.
- Do not modify the user's untracked `AGENTS.md`.

---

### Task 1: Refresh the eight non-classic palettes

**Files:**
- Modify: `frontend/scripts/assert-theme-skins.mjs`
- Modify: `frontend/app/globals.brutal.css`
- Modify: `frontend/lib/i18n.ts`

**Interfaces:**
- Consumes: existing `themeOptions`, `data-skin`, `data-skin-preview`, and `solo.skin` behavior
- Produces: the same nine IDs with new names and CSS tokens for the eight non-classic IDs

- [ ] **Step 1: Add the failing palette contract**

Add assertions for these exact ID/name/core-color pairs:

```js
const expectedRefresh = {
  blueprint: ['Light Modern', '明亮现代', '#74b4ee', '#c88bdd'],
  ultraviolet: ['GitHub Light', 'GitHub 浅色', '#54aeff', '#d2a8ff'],
  seasalt: ['Quiet Light', '静谧浅色', '#c4b7d7', '#91b3e0'],
  tomato: ['Solarized Light', 'Solarized 浅色', '#e8aa63', '#75c1bc'],
  matcha: ['Ayu Light', 'Ayu 浅色', '#f5ad66', '#c6a1e4'],
  bubblegum: ['Catppuccin Latte', 'Catppuccin 拿铁', '#cba6f7', '#89b4fa'],
  lavender: ['Rosé Pine Dawn', 'Rosé Pine 黎明', '#ebbcba', '#c4a7e7'],
  sky: ['Gruvbox Light', 'Gruvbox 浅色', '#b8bb26', '#d3869b'],
};
```

For each entry, assert that the relevant CSS selector contains both core colors and that `i18n.ts` contains both display names.

- [ ] **Step 2: Run the contract and verify RED**

Run: `cd frontend && node scripts/assert-theme-skins.mjs`
Expected: FAIL because the old palette values and names are still present.

- [ ] **Step 3: Apply the approved palette tokens and names**

Keep the selectors and IDs unchanged. Replace the eight non-classic variable blocks using the spec's exact canvas, primary, accent, success, and danger values. Use the same source palette for info, warning, violet, muted, and light variants. Replace the corresponding English and Chinese display strings with the names listed in Step 1.

- [ ] **Step 4: Run focused verification and verify GREEN**

Run:

```bash
cd frontend
node scripts/assert-theme-skins.mjs
node scripts/assert-i18n-language.mjs
npm run build
```

Expected: both assertions print `passed`; production build exits 0.

- [ ] **Step 5: Verify visual behavior**

Use Chromium to confirm all nine previews are distinct, each button applies instantly, reload preserves the selected ID, an invalid stored ID falls back to `classic`, the settings grid remains responsive, and no new console error occurs.

- [ ] **Step 6: Refresh graph and review the surgical diff**

Run:

```bash
graphify update .
git diff --check
git status --short
```

Expected: graph rebuild succeeds; diff check exits 0; only the three task files plus ignored documentation are changed, while `AGENTS.md` remains untracked.

- [ ] **Step 7: Commit**

```bash
git add frontend/app/globals.brutal.css frontend/lib/i18n.ts frontend/scripts/assert-theme-skins.mjs
git commit -m "feat: refresh Solo skins with editor palettes"
```
