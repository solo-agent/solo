# fe1 review of fe2 / `feat/fe2-brutal-pages`

> **Worktree:** `wt-fe1` (branch `feat/fe1-brutal-foundation`)
> **Review branch:** `review/fe2` (created from `feat/fe2-brutal-pages` tip `130839b`)
> **Base for diff:** `6fca3c5` (the common ancestor of fe1 and fe2)
> **Date:** 2026-06-06

## TL;DR

fe2 delivered 9 pages, 7 STUB components, an audit script, an e2e spec, and a PR template — 16 commits total. The pages themselves are well structured (one-issue-per-commit, all 9 use the new `<Spinner>`/`<Button>`/`<BrutalAlert>`/`<Select>` STUBs). However, **all STUB component APIs diverge from fe1's final APIs in breaking ways**, so the merge will produce a stack of TypeScript errors that block CI.

**Top 3 blockers:**
1. **P0** — `<Spinner size="lg" />` / `size="default"` / `size="sm" square={false}` are used in 6+ pages but fe1's final Spinner only exposes `sm` and `md`, and the `square` prop does not exist. This produces ~12 TypeScript errors at merge time.
2. **P0** — `<Select>{<option>…</option>}</Select>` (children pattern) is used in 2 pages (`tasks/page.tsx`, `tasks/new/page.tsx`). fe1's final Select uses an `options` prop, not children. 4 sites need to be rewritten.
3. **P0** — `<BrutalAlert variant="…" action={<Button/>}>` is used in 6 pages. fe1's final BrutalAlert has no `action` prop; it has `title` + children, with role override. 6 sites need restructuring.

**Plus 1 high-value debt item:** fe2's own commit `ae56467` introduced `bg-brutal-pink/20` opacity on hover states (in `create-dm-modal.tsx:176`, `member-list.tsx:39`, `streaming-message.tsx:42,133`) — these violate the brutal canon "no alpha on backgrounds" that fe2 just wrote into the audit script.

## A. STUB component API compatibility

fe2 created 7 STUB components in `components/ui/`. fe1's final versions are on `feat/fe1-brutal-foundation`. Below is the API delta and how fe2's pages will break.

### A1. `Spinner` — **P0 BLOCKER** (6 pages affected)

| Aspect | fe2 STUB | fe1 final |
| --- | --- | --- |
| Sizes | `sm` (h-4) / `default` (h-6) / `lg` (h-8) | `sm` (h-3.5) / `md` (h-5) only |
| `borderWidth` prop | Yes (2 or 4) | No — fixed per size |
| `square` prop | Yes (true→div, false→Loader2) | No — always div |
| `label` prop | No (hardcoded) | Yes (customizable) |
| Color | `border-black` | `border-brutal-primary` |

**Call sites that will break (size prop invalid):**

| File:line | Current | Broken because |
| --- | --- | --- |
| `app/auth/login/page.tsx:65` | `<Spinner size="lg" />` | `lg` not in final API |
| `app/auth/register/page.tsx:76` | `<Spinner size="lg" />` | same |
| `app/dashboard/page.tsx:25` | `<Spinner size="default" />` | `default` not in final API |
| `app/dashboard/page.tsx:36` | `<Spinner size="default" />` | same |
| `app/dashboard/page.tsx:51` | `<Spinner size="lg" />` | same |
| `app/dashboard/page.tsx:296` | `<Spinner size="lg" />` | same |
| `app/dashboard/page.tsx:310` | `<Spinner size="default" />` | same |
| `app/dashboard/page.tsx:329` | `<Spinner size="default" />` | same |
| `app/tasks/page.tsx:263` | `<Spinner size="lg" />` | same |
| `app/tasks/page.tsx:409` | `<Spinner size="sm" square={false} />` | `square` not in final API |
| `app/tasks/new/page.tsx:79` | `<Spinner size="lg" />` | same |
| `app/tasks/new/page.tsx:150` | `<Spinner size="lg" />` | same |
| `app/teams/page.tsx:100` | `<Spinner size="lg" />` | same |
| `app/computers/page.tsx:208` | `<Spinner size="lg" />` | same |
| `app/computers/page.tsx:659` | `<Spinner size="sm" square={false} />` | same |
| `app/settings/page.tsx:60` | `<Spinner size="lg" />` | same |

**16 broken call sites across 7 pages.** Migration map: `lg` → `md`, `default` → `md`, drop `square={false}` (the final is always a div).

### A2. `Select` — **P0 BLOCKER** (2 pages affected)

| Aspect | fe2 STUB | fe1 final |
| --- | --- | --- |
| Options source | `<option>` children | `options: SelectOption[]` prop |
| `error` prop | Yes | No (use className) |
| `label` prop | No | Yes (visible label) |
| `placeholder` prop | No | Yes (renders disabled option) |
| `size` prop | No | Yes (`sm`/`md`) |
| Wrapper | Bare `<select>` | Wrapped in `<div class="relative">` with chevron icon |

**Call sites that will break (children → options):**

| File:line | Current pattern |
| --- | --- |
| `app/tasks/page.tsx:302-314` | `<Select><option value="">认领人: 全部</option>{assigneeOptions.map(a => <option …>{a.name}</option>)}</Select>` |
| `app/tasks/page.tsx:317-329` | same pattern for creator |
| `app/tasks/new/page.tsx:115-127` | channel select with `<option value="">选择频道...</option>` |

**3 broken call sites** in 2 pages. Migration: convert inline `<option>` children to `options={[{value, label}]}` prop, and convert `value=""` placeholder to `placeholder="…"`.

### A3. `BrutalAlert` — **P0 BLOCKER** (6 pages affected)

| Aspect | fe2 STUB | fe1 final |
| --- | --- | --- |
| `title` prop | No (children only) | Yes (title + body) |
| `action` prop | Yes (slot) | **No** (use external layout) |
| `icon` prop | Yes (override) | No (uses variant default) |
| Variants | `info` / `warning` / `error` / `success` | Same names, semantic colors |
| `hideIcon` prop | No | Yes |
| `role` prop | No (hardcoded "alert") | Yes (alert/status) |
| Shadow | `shadow-brutal-sm` | None (intentional — alerts are inline) |
| Colors | `brutal-cyan/orange/red/lime` (legacy names) | `brutal-info/warning/danger/success` (semantic) |

**Call sites that will break (action prop, shadow, no-shadow mismatch):**

| File:line | Current pattern | Issue |
| --- | --- | --- |
| `app/computers/page.tsx:240-252` | `<BrutalAlert variant="warning" action={<Button>重试</Button>}>` | `action` doesn't exist in final |
| `app/computers/page.tsx:366-394` | same | same |
| `app/teams/page.tsx:123-136` | same | same |
| `app/teams/page.tsx:137-150` | same | same |
| `app/settings/page.tsx:100-112` | same | same |
| `app/workspace/page.tsx:112-114` | `<BrutalAlert variant="error">` (no action) | OK; `variant="error"` maps to `danger` in final |
| `app/auth/login/page.tsx:85` | `<BrutalAlert variant="error">{error}</BrutalAlert>` | OK semantically (no `action`) |
| `app/auth/register/page.tsx:96` | same | same |

**5 sites use `action` prop** — these need restructuring (move the Button out of the alert, render a flex row above the alert, or convert to a BrutalAlert with the Button as a sibling).

### A4. `EmptyState` — **Not used by fe2 pages, no breakage**

Per fe2's own review notes, EmptyState was "carried for fe1, not yet adopted in fe2 pages". fe2 did not adopt it. **No call sites to fix.**

API diff: fe2 STUB `icon: ReactNode` (required), `iconBgClass: string`, `iconSize: 'sm' | 'md' | 'lg'`, no `variant`. fe1 final `icon?: ReactNode` (optional), `variant: 'plain' | 'dashed'`, `actionLabel/onAction` slot. Both render a centered column with bordered icon. Different but no consumers to break.

### A5. `SelectableRow` — **Not used by fe2 pages, no breakage**

Same story as EmptyState — fe2 created the STUB and didn't adopt it. The two call sites that would have used it (channel-list / dm-list) use a hand-rolled `<div role="button">` pattern instead.

### A6. `SectionHeader` — **Not used by fe2 pages, no breakage**

fe2 created the STUB but didn't use it. fe2's section headers in `tasks-left-column.tsx` and `channel-list.tsx` are inlined button + chevron patterns.

### A7. `Tag` — **Not used by fe2 pages, no breakage**

Same — fe2 created the STUB, didn't adopt it. fe2's tags in pages are inline `<span className="badge-brutal bg-brutal-…">`.

### A8. STUB components summary

| Component | Used by fe2 pages? | API delta severity |
| --- | --- | --- |
| Spinner | Yes (heavily) | **P0** |
| Select | Yes | **P0** |
| BrutalAlert | Yes (heavily) | **P0** |
| EmptyState | No | (None — fe1 wins silently) |
| SelectableRow | No | (None) |
| SectionHeader | No | (None) |
| Tag | No | (None) |

## B. Token naming consistency

Measured on `review/fe2` branch (which is fe2's state):

| Token family | Count | Notes |
| --- | --- | --- |
| Old color names (`brutal-pink/yellow/cyan/lime/orange/red/lavender/stone`) | **331** | All in fe2-touched files |
| New semantic names (`brutal-primary/accent/info/success/warning/danger/muted`) | **0** | fe2 didn't adopt the rename |
| Old light tints (`brutal-pink-light/cyan-light/lime-light/red-light/…`) | **37** | fe2 actually INTRODUCED new uses (see §C) |

**Conclusion:** fe2 explicitly chose not to do the rename pass — they listed the mapping table in their review notes §36-55 and said "until fe1 renames, the fe2 worktree continues to reference the legacy names." This is a sound decision (avoid a half-renamed state), and the fe1 alias layer in `globals.brutal.css:519-560` makes the old names resolve correctly.

**Post-merge action:** fe1 should sweep these 331 + 37 sites to the new names. This is a separate PR — out of scope for the merge of fe2's branch.

**Conflict risk at merge time:** None. Aliases work. The only "conflict" is a single class name `border-l-brutal-pink` (CSS class, not the underlying token) — fe1's `globals.brutal.css:519` keeps it as an alias, so the 5 sites in `streaming-message.tsx`, `message-list.tsx`, `agent-message.tsx`, `thread-panel.tsx` are fine.

## C. Anti-pattern cleanup verification

`bash scripts/audit-brutal.sh` on `review/fe2` returns **7 rule violations**. Breakdown by ownership:

| Violation | File:line | In fe2's region? |
| --- | --- | --- |
| `rounded-lg` + `shadow-lg` | `components/dashboard/mention-dropdown.tsx:55` | No (fe1 owns this; fe1 already fixed in `7286986`) |
| `bg-green-500` / `bg-gray-400` | `components/computers/computers-left-column.tsx:130` | No (fe1 region) |
| `text-gray-400/300` | `components/agents/agent-card.tsx:107` | No |
| `text-yellow-600/500/10` etc. | `components/dashboard/typing-indicator.tsx:41-54` | No |
| `text-green-500/600`, `text-blue-500` | `components/dashboard/mention-dropdown.tsx:91,93,99` | No |
| `border-blue-500/red-500/blue-50/50` | `components/dashboard/agent-chunk.tsx:99-110` | No |
| `border-black/N` (alpha borders) | 12 sites, mostly fe1's region | No (mostly) |
| `bg-black/50 backdrop-blur-sm` | `components/tasks/create-task-modal.tsx:123` | No |
| `bg-brutal-pink-light/60 backdrop-blur-sm` | `components/dashboard/message-input.tsx:478` | No |
| `backdrop-blur` on dialog | `components/ui/dialog.tsx` | **Yes — fe2 already fixed in `130839b`** |
| Raw `<button class="btn-brutal">` | 4 sites in `agent-*-tab.tsx`, `teams-human-profile.tsx` | No |

**So fe2's commit message claim "After this commit, audit-brutal.sh → 'audit clean (9 paths scanned)'"** (in `ae56467`) **is false.** That commit only fixed 4 specific files and the audit still finds 7 rule violations. fe2 may have run the audit before the latest commit landed, or against a different state, or simply didn't run it at all. Either way: the audit is NOT clean on fe2's branch.

**Anti-patterns fe2 introduced (or left in their own files):**

| File:line | Pattern | Severity | Notes |
| --- | --- | --- | --- |
| `app/teams/page.tsx:177` | Raw `<button className="btn-brutal btn-brutal-sm bg-brutal-pink text-black">` (multiline) | P1 | Multiline → audit Rule 8 misses it. fe2 should use `<Button size="sm" variant="primary">`. |
| `components/tasks/tasks-left-column.tsx:127,194` | Raw `<button className="btn-brutal btn-brutal-sm">` (multiline) | P1 | fe2 wrote this file. Should use `<Button size="sm" variant="outline">`. |
| `components/dashboard/create-dm-modal.tsx:176` | `hover:bg-brutal-pink/20` (alpha on brutal token) | P1 | fe2 introduced this in `ae56467`. Violates the brutal canon. fe1 review notes §1.3 also flagged this. Should use `hover:bg-brutal-pink` (no opacity) or `hover:bg-brutal-pink-light` (no opacity). |
| `components/dashboard/create-dm-modal.tsx:202-203` | `bg-brutal-lime-light` / `bg-brutal-cyan-light` (old `-light` tints) | P2 | fe2 introduced in `ae56467`. Should migrate to `bg-brutal-success-light` / `bg-brutal-info-light` after fe1's rename. For now the aliases work. |
| `components/dashboard/create-dm-modal.tsx:215-217` | `fill-brutal-stone` / `fill-brutal-lime` (old color names) | P2 | fe2 introduced. Same. |
| `components/dashboard/create-dm-modal.tsx:242` | Inline `<Loader2 className="mr-1 h-3 w-3 animate-spin" />` | P1 | fe2's own notes §1.3 listed this for replacement with `<Spinner size="sm" />`. fe2 didn't fix. **The audit Rule 9 has a `grep -vE 'Loader2|…'` filter that SILENCES this.** |
| `components/dashboard/member-list.tsx:39` | `hover:bg-brutal-pink/20` (alpha) | P1 | fe2 introduced. Same. |
| `components/dashboard/member-list.tsx:24-29` | `fill-brutal-lime/stone/yellow/cyan` (old names) | P2 | fe2 introduced. |
| `components/connection-banner.tsx:78-81` | `bg-brutal-yellow/red/lime` (old names) | P3 | fe2 modified this file in `1d48f60` (drop Tailwind defaults) but kept the old color names. Not in audit but visible debt. |
| `components/network-status.tsx:49` | `bg-brutal-lime/red` (old names) | P3 | Same. |
| `app/teams/page.tsx:190,204` | `hover:bg-brutal-pink/40` (alpha) | P1 | fe2's `c34ee13 refactor(teams)` introduced this. The hover should be `hover:bg-brutal-pink` (no opacity) per the brutal canon. |

## D. Visual consistency

I can't run the dev server in this sandbox, so this section is based on static className review of the 9 pages and the 5 shared components fe2 touched.

**9 pages — consistent on:** layout shell (NavBar + 220px left column + main area), cream background (`bg-brutal-cream`), auth-guard spinner pattern. Good.

**9 pages — inconsistent on:**

1. **Spinner sizing** — every page uses a different size for the same auth-guard loading state. login/register use `size="lg"`, dashboard uses `size="lg"` for the outer guard but `size="default"` for in-page loaders. This is a STUB-API artifact (not the designers' choice); after migration to fe1's `sm`/`md`, this collapses to two consistent sizes. **After fix: OK.**

2. **BrutalAlert + Button(action) pattern** — 5 sites render `<BrutalAlert action={<Button>重试</Button>}>`. The pattern works in fe2's STUB, but fe1's final has no `action` slot. After migration, these 5 sites will render a stacked alert + button instead of an inline button. **The visual after-fix is acceptable but the spacing/alignment will be off.** Migrate to either:
   - `<BrutalAlert>` + a separate `<Button>` below it
   - A new `<AlertWithAction>` wrapper (fe1's call, not fe2's)
   For minimum churn, the stacked layout is fine.

3. **Tab strip in `app/teams/page.tsx:184-211`** — fe2's commit `c34ee13` replaced emojis with `<User>` / `<FolderOpen>` icons (good, matches the audit's emoji rule), but kept `hover:bg-brutal-pink/40` — this alpha pink doesn't have enough contrast on white to look intentional. Replace with `hover:bg-brutal-pink-light` or `hover:bg-brutal-pink` (solid).

4. **`dialog.tsx` backdrop** — fe2's `130839b` switched from `bg-black/50 backdrop-blur-sm` to `bg-black/60`. The spec says "重压" (heavy/pressing) feel. 60% is borderline — for true brutalist impact, `bg-black/80` or `bg-black/90` would feel more like a "sticker slammed on the page". 60% is acceptable but a bit light.

## E. Cross-file coordination

fe1's review notes (which fe2 received) listed 5 PRs worth of cleanup in fe1's region:
1. Two `focus-visible:ring-brutal-pink` overrides in `task-column.tsx:211, 244`
2. 7 inline `Loader2` spinners
3. 8 list-row `border-b border-black/10` files
4. 4 `rounded-md` files
5. Old `border-l-brutal-pink` alias renames

fe2's response:
- PR 1 (focus rings): **Not done** — these are in fe1's region (fe2's commit boundary excludes them).
- PR 2 (inline spinners): **Partially done** — fe2's commit `7f388be refactor(dashboard): replace 4 inline spinners` only fixed 4 of the 7 listed. The remaining 3 are still in fe1's region.
- PR 3 (border bottom weight): **Not done** — these are all in fe1's region.
- PR 4 (rounded-md): **Done for the 4 files listed** — fe2's `ae56467` fixed `member-list.tsx:39`, `dm-list.tsx:78`, `channel-list.tsx:46`, `create-dm-modal.tsx:176`.
- PR 5 (border-l-brutal-pink alias): **Not done** — fe1's region.

**Cross-file coordination within fe2's region:**
- `app/teams/page.tsx:177` (raw btn-brutal button) — fe2 introduced this. Audit Rule 8 misses it (multiline).
- `components/tasks/tasks-left-column.tsx:127, 194` (raw btn-brutal buttons) — fe2 wrote this file. Audit misses both (multiline).

These two sites are **in fe2's region but not in any of fe1's review notes** — fe2 should sweep them as part of the page work.

## F. E2E test coverage strength

`frontend/e2e/brutal-consistency.spec.ts` has 5 tests. Strength assessment:

| Test | Catches? | Misses / false positive risk |
| --- | --- | --- |
| `no mid-range rounded corners on any visited page` | Yes (rounded-{sm,md,lg,xl,2xl,3xl}) | Doesn't visit `/workspace`, `/tasks/new`, or `/auth/*` — misses login/register spinners. Doesn't catch `rounded-md` on the `kbd` element (not present here). |
| `no default Tailwind color scale on visible interactive elements` | Tailwind colors in `bg-/text-/border-` classes | Regex `green\|red\|blue\|gray\|amber\|yellow` misses `purple`, `pink`, `cyan`, `lime`, `orange`, `stone`, `slate`, `zinc`, `neutral`, `indigo`, `violet`, `fuchsia`, `rose`, `sky`, `emerald`, `teal`. The e2e test would miss `text-green-500` on `mention-dropdown.tsx:91` because it's inside a portal, but the test only checks `[class*="bg-"], [class*="text-"], [class*="border-"]` selectors so it would catch the class string. The regex is narrower than the static audit's regex. **Gap: tailwind colors in className strings are caught; opacity modifiers (e.g., `bg-green-500/10`) are NOT — the regex has no `/\d+` clause.** |
| `spinners all share the same className shape` | `border-[24]\b` on every `animate-spin` element | The `border-[24]` regex matches `border-2`, `border-4` — the final Spinner uses `border-[3px]` (not in the regex). **Will FAIL when fe1's Spinner lands.** This test will be a false positive. **P0 — needs updating to `border-\d` or `border-\[?\d+` to handle fe1's `border-[3px]`.** |
| `focus ring is consistent (cyan outline color)` | Tab to first 3 elements, check `outlineColor` | Hardcodes `rgb(116, 185, 255)` — fe1's globals.brutal.css:280 sets `--color-brutal-info: #74b9ff` which is `rgb(116, 185, 255)`. **Test will pass only if the active element uses the global rule.** `* :focus-visible { outline: 3px solid var(--color-brutal-info); }` should apply universally. Fragile: if the active element has its own focus-visible rule, this fails. |
| `dialog overlay has no backdrop-blur` | `.fixed.inset-0.z-50` first match | Has `if (await dialog.isVisible())` — silently passes if the dialog isn't visible (i.e., if the user hasn't opened the create-channel modal). **False negative risk: low value.** |

**Overall:** The e2e spec is a good start but has 2 real issues:
- Spinner test will break at fe1 merge (regex too narrow).
- Dialog test silently passes (no assertion path).

**5 unimplemented anti-patterns the e2e spec doesn't catch:**
- `bg-brutal-pink/20` opacity (brutal canon violation, not in any rule)
- `bg-brutal-pink-light` legacy tint (will silently break after fe1's rename)
- `border-l-brutal-pink` legacy alias (same)
- Inline `Loader2` use (audit Rule 9 silently filters it)
- `rounded` defaults on shadcn primitives (e.g., `bg-popover`, `border`)

## G. `audit-brutal.sh` false positive / false negative analysis

| Rule | False positive risk | False negative risk | Recommendation |
| --- | --- | --- | --- |
| 1. `rounded-(md\|lg\|sm\|xl\|2xl\|3xl)` | **Low.** Will match the `rounded border` on a `<kbd>` element (search/global-search.tsx:258), but fe1's notes §2.3 already said this is the only acceptable exception. | None for brutalist canon. | Add `\| grep -v '<kbd'` exception. |
| 2. Default Tailwind color scale | **Low.** Excludes `globals.brutal.css` correctly. | **High.** Doesn't catch opacity modifiers (`/10`, `/20`, etc. on `bg-green-500`). Doesn't catch the "modern" Tailwind palette (`bg-zinc-900`, `bg-slate-800`, `bg-rose-500`, etc. — wait, actually `slate`/`rose` are in the regex, but `fuchsia` etc. are too — let me re-check the regex: `(green\|red\|amber\|yellow\|gray\|blue\|orange\|pink\|cyan\|lime\|lavender\|stone\|emerald\|teal\|indigo\|purple\|fuchsia\|rose\|sky\|violet\|slate\|zinc\|neutral)` — looks comprehensive). Doesn't catch `dark:` variants. | Add opacity modifier pattern: `\b(bg\|text\|border)-(green\|red\|…)-(50\|…\|950)\/[0-9]+`. |
| 3. `text-gray-N` | **Low.** | None. | OK. |
| 4. `border-black/N` (alpha) | **Low.** Excludes nothing. | Doesn't catch `border-b-black/10` (border-side alpha) — wait, the regex `\bborder-black\/[0-9]+\b` matches `border-black/10` regardless of preceding tokens. So this is fine. **Caveat:** doesn't catch `border-t-b-black/10` Tailwind 4 syntax (if any). | OK for current codebase. |
| 5. `shadow-(lg\|md\|2xl\|xl\|sm\|inner)` | **Low.** | None — `shadow-brutal-*` is excluded by `-` token. | OK. |
| 6. `backdrop-blur` | **Low.** | None. | OK. |
| 7. Emoji as icon | **Low.** | **Medium.** The character class is curated (24 emoji) — will miss any emoji not in the set. | Add `\p{Emoji_Presentation}` Unicode property to be future-proof, but the curated set is intentional. |
| 8. Raw `<button>` with `btn-brutal` class | **None.** | **HIGH.** The regex `'<button[^>]*className="[^"]*btn-brutal'` is single-line only. In multi-line JSX (`<button\n  type="button"\n  className="btn-brutal …"`), `<button` and `className=` are on different lines and the regex doesn't match. **20+ files in the codebase have multi-line raw buttons that the audit misses** (verified with a Python multiline search). | Switch to `grep -Pzo '(?s)<button[^>]*className="[^"]*btn-brutal'` (Perl-compatible + multi-line). |
| 9. `animate-spin` not on Spinner/Loader2 | **Low.** | The `grep -vE 'Loader2\|…'` filter SILENCES inline `Loader2` use, even though the design canon says use `<Spinner>` instead. So inline `Loader2` is invisible to this rule. | Remove the `Loader2` exemption OR add a separate rule "inline `Loader2` is forbidden, use `<Spinner>`". |

**The audit script is sound in concept but has 2 real bugs:** Rule 8 (multiline) and Rule 9 (Loader2 exemption). fe2 should fix these.

## H. mention-dropdown.tsx conflict

**The user's hypothesis is correct — fe1's branch HAS fixed this, but fe2's branch has not.**

| Branch | `mention-dropdown.tsx:55` |
| --- | --- |
| `6fca3c5` (master base) | `className="fixed z-[100] rounded-lg border bg-popover shadow-lg"` |
| `feat/fe2-brutal-pages` (130839b tip) | `className="fixed z-[100] rounded-lg border bg-popover shadow-lg"` ← unchanged |
| `feat/fe1-brutal-foundation` (033e141 tip) | `className="fixed z-[100] border-2 border-black bg-white shadow-brutal-sm rounded-none"` ← fixed in `7286986` |

**Resolution:** When the two branches merge, `mention-dropdown.tsx` will be fe1's fixed version. fe2's review notes §4 are correct (this was fe1's debt), and fe1 has paid it. **No action needed for this file.**

The audit script flags it on fe2's branch because the script runs on whatever HEAD is checked out. fe2 should add a comment to the audit workflow that says "run after merge, not on source branch" — or accept that the audit will report pre-existing issues from the other branch.

## Fix list (priority order)

### P0 — must fix (merge blockers)

1. **`<Spinner size="lg" />` and `size="default"`** (16 sites, 7 pages) → replace with `size="md"`. Drop `square={false}` prop.
2. **`<Select>{<option>…</option>}</Select>`** (3 sites, 2 pages) → convert to `<Select options={…} placeholder="…">`.
3. **`<BrutalAlert action={…}>`** (5 sites, 4 pages) → restructure to `<BrutalAlert>…</BrutalAlert>` + a sibling `<Button>`.

### P1 — important (debt in fe2's region)

4. `app/teams/page.tsx:177` — replace raw `<button className="btn-brutal …">` with `<Button size="sm" variant="primary">`.
5. `components/tasks/tasks-left-column.tsx:127, 194` — same, replace raw buttons with `<Button size="sm" variant="outline">`.
6. `components/dashboard/create-dm-modal.tsx:176, 202-204, 215-217, 242` — `bg-brutal-pink/20` → solid `bg-brutal-pink-light` or `bg-brutal-pink`; inline `Loader2` → `<Spinner size="sm" />`.
7. `components/dashboard/member-list.tsx:39` — same opacity fix.
8. `app/teams/page.tsx:190, 204` — `hover:bg-brutal-pink/40` → `hover:bg-brutal-pink-light`.
9. **audit-brutal.sh Rule 8** — fix multiline regex so it catches multi-line `<button>` patterns.
10. **e2e test "spinner className shape"** — relax `border-[24]\b` to handle fe1's `border-[3px]`.

### P2 — edge / future cleanup

11. `bg-brutal-pink-light/30`, `bg-brutal-red-light/30` opacity uses (in `inbox-item.tsx:48`, `message-list.tsx:237, 642`, `add-agent-modal.tsx:96`, `message-input.tsx:478`) — out of fe2's scope, but the audit should add a rule for them.
12. Audit Rule 2 should also catch `bg-…/[0-9]+` opacity modifiers on default Tailwind colors.
13. Dialog `bg-black/60` — bump to `bg-black/80` for true brutalist weight.
14. Token rename sweep (331 sites) — separate post-merge PR.

## Fixes applied in `review/fe2` branch

This review ships the P0 fixes + the most impactful P1 fixes in commit `fix(review): fe1 review findings on fe2 branch`. The commit:

- P0: Migrates all `<Spinner size="lg" / "default">` to `size="md"` and drops `square={false}`.
- P0: Migrates `<Select>` to fe1's `options`/`placeholder` API in `app/tasks/page.tsx` (2 sites) and `app/tasks/new/page.tsx` (1 site).
- P0: Restructures `<BrutalAlert action={…}>` to alert + sibling Button in 5 sites.
- P1: Replaces 3 raw `<button className="btn-brutal">` patterns (in `app/teams/page.tsx` and `components/tasks/tasks-left-column.tsx`) with the `<Button>` component.
- P1: Drops `bg-brutal-pink/20` and `hover:bg-brutal-pink/40` opacity in fe2's modified files.
- P1: Replaces inline `<Loader2>` in `create-dm-modal.tsx:242` with `<Spinner size="sm" />`.

**P1 items deferred** (out of scope of this review commit):
- e2e test spinner regex update (lives in test files; better as a separate fix when fe1's Spinner lands).
- audit-brutal.sh Rule 8 multiline fix (the script's logic is sound; the bug is regex-only; better as a separate fix).
- Token rename sweep (331 sites, separate PR).

## Quality score

**6/10.** fe2 executed the page work cleanly — 16 commits, one concern per commit, all 9 pages compile, all 9 use the new STUB components. The audit script and e2e spec are real value-adds. But the STUB components are *stub-shaped* — the props diverge from fe1's final in ways that will produce ~24 TypeScript errors at merge time, and fe2's review notes correctly identify the API gap but the merge won't be smooth without adaptation. The `bg-brutal-pink/20` opacity violation that fe2 introduced in `ae56467` (despite writing the audit script that would have caught it) is the most ironic miss.

**fe2 did the right things:** wrote the audit script, wrote the e2e spec, wrote the PR template, adopted the STUB components, gave fe1 a clear list of debt to clean up. **fe2's gaps are mechanical:** size prop migration (16 sites), Select API migration (3 sites), BrutalAlert action restructure (5 sites), and the one-internal-violation that fe1's own audit would catch (`bg-brutal-pink/20`).
