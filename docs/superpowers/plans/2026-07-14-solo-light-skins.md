# Solo Light Skins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add nine fixed light skins to Solo, selectable from Settings and persisted in the current browser.

**Architecture:** Reuse the existing semantic CSS variables. A `data-skin` attribute on `<html>` selects one complete palette, a tiny `theme.ts` validates and persists the selected ID, and a pre-hydration script restores it before first paint. The settings page renders the nine options directly without a provider, backend work, or new dependency.

**Tech Stack:** Next.js 16, React 19, TypeScript, Tailwind CSS v4 semantic variables, Node source-check scripts, Playwright CLI.

## Global Constraints

- Exactly nine fixed light skins: classic, blueprint, ultraviolet, seasalt, tomato, matcha, bubblegum, lavender, sky.
- Persist only in the current browser under `solo.skin`.
- Keep black borders, white cards, hard shadows, zero radius, and existing typography.
- No dark skins, custom color editor, database/API sync, Theme Provider, or new dependency.
- Unknown or inaccessible storage falls back to `classic`.
- Theme selection must apply before first paint without a hydration warning.

---

## File Structure

- Create `frontend/lib/theme.ts`: IDs, translation keys, validation, storage read/write, DOM application.
- Create `frontend/scripts/assert-theme-skins.mjs`: executable red/green check for runtime behavior and source coverage.
- Modify `frontend/app/globals.brutal.css`: palette selectors shared by the root and preview cards.
- Modify `frontend/app/layout.tsx`: default `data-skin`, hydration suppression, pre-paint storage restoration.
- Modify `frontend/app/settings/page.tsx`: responsive 3×3 skin buttons.
- Modify `frontend/lib/i18n.ts`: English and Chinese labels/hints.

---

### Task 1: Theme Runtime Contract

**Files:**
- Create: `frontend/scripts/assert-theme-skins.mjs`
- Create: `frontend/lib/theme.ts`

**Interfaces:**
- Produces: `themeOptions`, `ThemeId`, `defaultThemeId`, `themeStorageKey`, `resolveThemeId`, `getStoredTheme`, `setTheme`.
- Consumes: browser `document.documentElement.dataset`, `window.localStorage`, and `solo:theme-change` events.

- [ ] **Step 1: Write the failing executable check**

Create a Node script that transpiles `lib/theme.ts` with the installed TypeScript package, executes it in a VM with real Map-backed storage, and asserts:

```js
if (themeOptions.length !== 9) throw new Error('Expected exactly nine themes');
if (new Set(themeOptions.map(({ id }) => id)).size !== 9) throw new Error('Theme IDs must be unique');
if (resolveThemeId('unknown') !== 'classic') throw new Error('Unknown theme must fall back');
if (getStoredTheme() !== 'classic') throw new Error('Missing storage must fall back');
if (setTheme('seasalt') !== 'seasalt') throw new Error('Valid theme should apply');
if (document.documentElement.dataset.skin !== 'seasalt') throw new Error('Theme must update the root');
if (storage.get('solo.skin') !== 'seasalt') throw new Error('Theme must persist');
```

The script must also read the CSS, layout, and settings sources and assert that every theme ID has a CSS selector, the layout contains the storage bootstrap, and the settings page maps `themeOptions` into `aria-pressed` buttons.

- [ ] **Step 2: Run the check and confirm RED**

Run: `node scripts/assert-theme-skins.mjs`

Expected: FAIL because `lib/theme.ts` does not exist.

- [ ] **Step 3: Implement the minimal runtime**

Create `lib/theme.ts` with this public shape:

```ts
import type { TranslationKey } from '@/lib/i18n';

export const themeStorageKey = 'solo.skin';
export const defaultThemeId = 'classic';
export const themeOptions = [
  { id: 'classic', labelKey: 'themeClassic' },
  { id: 'blueprint', labelKey: 'themeBlueprint' },
  { id: 'ultraviolet', labelKey: 'themeUltraviolet' },
  { id: 'seasalt', labelKey: 'themeSeasalt' },
  { id: 'tomato', labelKey: 'themeTomato' },
  { id: 'matcha', labelKey: 'themeMatcha' },
  { id: 'bubblegum', labelKey: 'themeBubblegum' },
  { id: 'lavender', labelKey: 'themeLavender' },
  { id: 'sky', labelKey: 'themeSky' },
] as const satisfies ReadonlyArray<{ id: string; labelKey: TranslationKey }>;

export type ThemeId = (typeof themeOptions)[number]['id'];

export function resolveThemeId(value: string | null | undefined): ThemeId;
export function getStoredTheme(): ThemeId;
export function setTheme(value: string): ThemeId;
```

`getStoredTheme` catches storage access errors. `setTheme` resolves the ID first, always updates the DOM when available, catches only the storage write, dispatches `solo:theme-change`, and returns the applied ID.

- [ ] **Step 4: Run the check**

Run: `node scripts/assert-theme-skins.mjs`

Expected: advance beyond the missing runtime and fail on missing CSS/layout/settings integration.

---

### Task 2: Palette CSS And First Paint

**Files:**
- Modify: `frontend/app/globals.brutal.css`
- Modify: `frontend/app/layout.tsx`

**Interfaces:**
- Consumes: the nine stable IDs from Task 1.
- Produces: selectors matching both `:root[data-skin="ID"]` and `[data-skin-preview="ID"]`.

- [ ] **Step 1: Add exact palette selectors**

Add one shared default selector and eight named overrides. Use the exact base colors in the design spec and these corresponding light fills:

| ID | primary-light | accent-light | info-light | success-light | warning-light | danger-light | violet-light | muted-light |
|---|---|---|---|---|---|---|---|---|
| classic | `#FFF9E0` | `#FFD4D4` | `#E0F0FF` | `#E0F5E0` | `#FFF0E0` | `#FFE0DC` | `#F0ECFF` | `#ECE6DF` |
| blueprint | `#DCEAFF` | `#FFD9E5` | `#DDF8FC` | `#E3F7E7` | `#FFF0D5` | `#FFE0E3` | `#EEE9FF` | `#EDF1F5` |
| ultraviolet | `#EEE0FF` | `#DDF8F2` | `#E2EEFF` | `#E4F6E8` | `#FFF2D8` | `#FFE3E5` | `#ECE7FA` | `#F0EAF5` |
| seasalt | `#D7F8EC` | `#FFE2DB` | `#E0E9FF` | `#E3F4E0` | `#FFF3D4` | `#FFDFE3` | `#ECE9FF` | `#E7F2EE` |
| tomato | `#FFE2DB` | `#DDEBFF` | `#DDF8F3` | `#E3F4E0` | `#FFF0D1` | `#FFDBE1` | `#EFE8FA` | `#F0E9E5` |
| matcha | `#ECF8C9` | `#E0E6FF` | `#DDF6F9` | `#DFF3E3` | `#FFF2D8` | `#FFE0DF` | `#EDE9FA` | `#EAF0DC` |
| bubblegum | `#FFDDED` | `#DCF7F3` | `#E3ECFF` | `#E3F6E7` | `#FFF3D9` | `#FFDFE5` | `#EEE7FF` | `#F3E8EE` |
| lavender | `#E8E0FF` | `#FFF5C7` | `#DFF3FB` | `#E5F4E7` | `#FFEBD5` | `#FFE1E4` | `#F0EBFF` | `#ECE8F3` |
| sky | `#D7F3FF` | `#FFE4D6` | `#E3E9FF` | `#E1F3E7` | `#FFF4D7` | `#FFDEDF` | `#EDEAFF` | `#E6F0F4` |

Each selector sets the `brutal-*` palette and mirrors canvas, primary, accent, destructive, ring, muted, and sidebar compatibility variables. Keep structural black/white tokens unchanged.

- [ ] **Step 2: Add pre-paint restoration**

In `app/layout.tsx`, render `data-skin="classic"` and `suppressHydrationWarning` on `<html>`. Add a synchronous `<head><script>` whose body is exactly scoped to reading `solo.skin` and assigning `document.documentElement.dataset.skin`; storage exceptions leave the server default unchanged.

- [ ] **Step 3: Run the theme check**

Run: `node scripts/assert-theme-skins.mjs`

Expected: fail only because the settings UI and i18n labels are missing.

---

### Task 3: Settings Skin Selector

**Files:**
- Modify: `frontend/app/settings/page.tsx`
- Modify: `frontend/lib/i18n.ts`

**Interfaces:**
- Consumes: `themeOptions`, `getStoredTheme`, `setTheme` and existing `t`.
- Produces: nine native buttons with `data-skin-preview`, translated names, and `aria-pressed` selection state.

- [ ] **Step 1: Add translations**

Add these English/Chinese keys to both dictionaries:

```ts
settingsTheme: 'Skin' / '皮肤'
settingsThemeHint: 'Applies instantly and is saved in this browser' / '点击后立即生效，并保存在当前浏览器中'
themeClassic: 'Classic Sunlight' / '经典日光'
themeBlueprint: 'Blueprint Soda' / '蓝图汽水'
themeUltraviolet: 'Ultraviolet Studio' / '紫外线工作室'
themeSeasalt: 'Sea Salt Orange' / '海盐橙'
themeTomato: 'Tomato Radio' / '番茄电台'
themeMatcha: 'Matcha Blue' / '抹茶蓝'
themeBubblegum: 'Bubblegum' / '泡泡糖'
themeLavender: 'Lavender Lemon' / '薰衣草柠檬'
themeSky: 'Clear Sky Orange' / '晴空橘'
```

- [ ] **Step 2: Add the selector**

Import `Palette` and `Check`, plus the theme helpers. Initialize component state to `classic`, synchronize it from `getStoredTheme()` in the existing mount effect, and render after Language:

```tsx
<div className="border-t-2 border-black px-6 py-5">
  <Label>{t('settingsTheme')}</Label>
  <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
    {themeOptions.map((option) => (
      <button
        key={option.id}
        type="button"
        data-skin-preview={option.id}
        aria-pressed={theme === option.id}
        onClick={() => setThemeState(setTheme(option.id))}
      >
        <span className="grid grid-cols-4 border-b-2 border-black" aria-hidden="true">
          <span className="h-8 bg-brutal-primary" />
          <span className="h-8 bg-brutal-accent" />
          <span className="h-8 bg-brutal-info" />
          <span className="h-8 bg-brutal-success" />
        </span>
        <span className="flex items-center justify-between gap-2 bg-white px-3 py-2">
          <span>{t(option.labelKey)}</span>
          {theme === option.id && <Check className="h-4 w-4" aria-hidden="true" />}
        </span>
      </button>
    ))}
  </div>
</div>
```

Use existing brutalist border/shadow classes. Do not extract a one-use component. Increase the page width only enough for the 3-column grid and stack to one column on narrow screens.

- [ ] **Step 3: Run the theme check and existing i18n check**

Run:

```bash
node scripts/assert-theme-skins.mjs
node scripts/assert-i18n-language.mjs
```

Expected: both print their pass messages and exit 0.

---

### Task 4: Build, Browser E2E, Graph And Review

**Files:**
- Verify all changed files; no new production file unless a failing verification requires a scoped fix.

- [ ] **Step 1: Run production build**

Run: `npm run build`

Expected: Next.js compilation, TypeScript, and static page generation exit 0.

- [ ] **Step 2: Run browser end-to-end verification**

Start the existing Solo services and use Playwright CLI in a fresh named session. Authenticate through the real UI/API, open `/settings`, and assert:

```text
9 skin buttons are visible
each click changes html[data-skin]
each click writes solo.skin
the computed --color-brutal-primary matches the selected palette
reload preserves a non-default selection
dashboard/tasks/teams keep readable text and brutal borders/shadows
console contains no hydration, React, or uncaught runtime error
```

Capture a settings screenshot for visual review, then close the browser session.

- [ ] **Step 3: Update graphify**

Run: `graphify update .`

Expected: incremental graph update exits 0.

- [ ] **Step 4: Self-review and fresh verification**

Review `git diff --check`, `git diff --stat`, and the full diff. Confirm every changed production line maps to the approved nine-skin scope, no user-owned `AGENTS.md` is staged, and no dark/custom/backend code was added. Fix any issue, then rerun the theme check, i18n check, production build, and targeted browser flow before completion.
