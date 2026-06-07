# fe2 review of fe1/feat/fe1-brutal-foundation

> **Worktree:** wt-fe2 (branch `review/fe1` created from `feat/fe1-brutal-foundation`)
> **Reviewer:** fe2
> **Date:** 2026-06-06
> **Method:** read every file in fe1's diff; cross-check API vs fe2's planned STUB; grep for residual issues; `tsc --noEmit` validates.

---

## TL;DR

**Quality: 7.5/10** — fe1 is solid. Token rename is clean, components are typed and small (<100 LOC each), single-file fixes are mostly complete, and the cross-PR handoff (review-notes.md, visual-capture-status.md) is unusually thoughtful. The biggest problems are:

1. **P0 a11y regression** — `focus-visible:outline-none` left in `inbox-item.tsx` and `task-card.tsx` (2x) suppresses the new global electric-blue focus ring. fe1 removed the old pink ring override but kept the `outline-none`, so the focus state is now **invisible** to keyboard users.
2. **P1 dark-mode half-finish** — `--color-brutal-black: #ffffff` is declared in dark mode, but `.btn-brutal`, `.card-brutal`, `.input-brutal`, `.navbar-icon`, `.badge-brutal`, `.divider-brutal`, `.message-bubble`, `.mention-highlight`, `.tasknum-highlight`, `.pixel-avatar-cell-filled` all use hardcoded `border: 2px solid #000`. Borders don't invert.
3. **API mismatch with fe2 STUB** — fe1's `Button.variant` enum is `primary|danger|outline|ghost` + 4 deprecated aliases. fe2's STUB was `default|destructive|outline`. The 9 fe2 pages that use `<Button variant="default">` will break; fe2 needs a migration sweep. fe1's `Spinner` has no `variant` prop at all (STUB assumed `variant="black|pink"`).

Everything else (token aliases, 4px/8px borders, btn/card translate rhythm, reduced-motion, agent-selector migration, env-editor translate fix, layout font cleanup) is correct.

---

## A. globals.brutal.css 审查

### A.1 Token rename — clean ✓
- Old hex-named tokens (`brutal-pink: #FFD23F`, `brutal-yellow: #FF6B6B`, etc.) replaced with semantic names (`brutal-primary`, `brutal-accent`, etc.) at lines 48-63.
- Deprecated aliases at lines 71-85 each point to the correct new name. Verified by reading the `var()` chain.
- The naming confusion in the OLD system (yellow called "pink", red called "yellow") was real — the new names are role-based (`primary`/`accent`/`info`/...) which is correct.

### A.2 4px/8px border tokens — present, but only as utility classes ✓
- `.border-brutal-thicker` (4px) and `.border-brutal-thickest` (8px) defined at lines 243-249.
- They use `var(--color-brutal-black)` (not hardcoded) — so they DO invert in dark mode. Good.
- They are NOT in the `@theme inline` block (only the base `.border-brutal`/`.border-brutal-thin`/`.border-brutal-thick` are also outside, so consistent).

### A.3 btn-brutal-sm offset rhythm — correct ✓
- `.btn-brutal-sm:active` at line 301-304 uses `translate(3px, 3px)` + `box-shadow: none` — matches base. Verified in `env-editor.tsx` (single-file fix C5) which uses the same `active:translate-x-[3px] active:translate-y-[3px]` pattern.

### A.4 card-brutal:hover — correct ✓
- `translate(-1px, -2px)` + `box-shadow: 7px 8px 0 0 var(--color-brutal-shadow)` at line 338-341. Asymmetric Y offset is the lift effect.

### A.5 Dark mode — **incomplete (P1)**
- `--color-brutal-bg`, `--color-brutal-cream`, `--color-brutal-black`, `--color-brutal-white`, `--color-brutal-shadow` all inverted (lines 169-173). ✓
- The semantic palette (primary/accent/info/success/warning/danger/violet/muted) is NOT overridden, intentionally — the comment at line 164 says "reads fine on dark with white borders." But the **borders are hardcoded `#000`** in 10 CSS rules. The "white borders" promise is broken.
- Affected rules (all hardcode `border: 2px solid #000` or `border-top: 2px solid #000`):
  - `.btn-brutal` (line 263)
  - `.card-brutal` (line 332)
  - `.input-brutal` (line 348)
  - `.navbar-icon` (line 447)
  - `.badge-brutal` (line 401)
  - `.divider-brutal` (line 411)
  - `.message-bubble` (line 621)
  - `.mention-highlight` (line 678)
  - `.tasknum-highlight` (line 688)
- `--color-brutal-fg: #000000` is declared at line 43 but **never referenced anywhere** in the codebase. Dead code. (Also not overridden in dark mode, but moot since unused.)

### A.6 focus-visible global — present but **suppressed in 2 components (P0)**
- Global rule at line 477-480: `*:focus-visible { outline: 3px solid var(--color-brutal-info); outline-offset: 2px; }` ✓
- **But** `inbox-item.tsx:48` and `task-card.tsx:76, 118` each carry `focus-visible:outline-none`, which (in Tailwind v4) has higher specificity than the global `*:focus-visible` and **removes the ring entirely**. The previous pink `focus-visible:ring-*` overrides are gone, but the *lack of any visible focus state* is an a11y regression.
- `input-brutal:focus` at line 360-364 also adds its own `outline: 3px solid var(--color-brutal-info)` redundantly alongside `box-shadow` change — not a bug, just defensive.

### A.7 reduced-motion — complete ✓
- The `*` selector override (line 501-507) sets transition/animation duration to 0.001ms.
- Custom `@keyframes` (animate-spin, animate-pulse, animate-bounce, animate-fade-in, animate-slide-in-from-*, animate-indeterminate-progress, cursor-blink, cursor-blink-pink) are explicitly nulled at line 538-549.
- Per-component overrides (btn-brutal/card-brutal/navbar-icon) restore a static shadow so the visual rest state is preserved. Defensive but correct.

---

## B. 8 new components API

| Component | fe1 final API | fe2 STUB assumption | Conflict? | Notes |
|---|---|---|---|---|
| **Spinner** | `size?: 'sm'\|'md'`, `className`, `label?` | `size`, `variant?: 'black'\|'pink'`, `className`, `label` | **YES** | fe1 has NO `variant` prop. The color is hardcoded to `border-brutal-primary` (yellow). fe2 STUB assumed you could pick a color. The simpler API is fine — just delete `variant` from the STUB plan. |
| **Button** | `variant: 'primary'\|'danger'\|'outline'\|'ghost'` + 4 deprecated aliases | `variant: 'default'\|'destructive'\|'outline'` | **YES (BREAKING)** | fe1's `primary` is the yellow call-to-action. fe1's `default` is an alias for `primary`. fe2's STUB `default` was probably "the default white button". 9 fe2 pages using `<Button variant="default">` will get the yellow style — visual surprise. **Migrate fe2 to `variant="primary"` for CTAs and `variant="outline"` for secondary buttons.** |
| **SelectableRow** | `selected? onClick? leading? trailing? children?` (+ all button props) | matches | NO | API matches. |
| **SectionHeader** | `label count? expanded? onToggle? as? className? trailing?` | matches | NO | API matches. |
| **EmptyState** | `icon? title description? actionLabel? onAction? variant? className?` | matches | NO | API matches. `variant: 'plain'\|'dashed'` confirmed. |
| **BrutalAlert** | `variant?: 'error'\|'warning'\|'success'\|'info'`, `title?`, `children?`, `hideIcon?`, `role?` | matches | NO | Extra `hideIcon` and `role` props are nice. |
| **Tag** | `variant?: 'status'\|'type'\|'agent'\|'deleted'`, `className?`, `children` | matches | NO | API matches. |
| **Select** | `options: SelectOption[]`, `label?`, `placeholder?`, `size?: 'sm'\|'md'`, `className?` + all select props | matches | NO | fe1 chose native `<select>` over shadcn Radix Select — good call (no extra dep, full a11y + mobile picker). Trigger is brutal-styled (`border-2 border-black shadow-brutal-sm`, plus ChevronDown icon). |

**API conflict summary**: Spinner (no variant) and Button (different variant enum). Both are P0 for fe2 to address.

---

## C. 8 single-file fixes quality

| File | fe1 change | Complete? | Residual issues |
|---|---|---|---|
| `inbox-item.tsx` | Removed pink `focus-visible:ring-*`, `border-b`→`border-b-2 border-black`, ad-hoc badge → `<Tag>`, removed mono/time pair → `text-xs text-muted-foreground` | **NO** | **P0 a11y**: `focus-visible:outline-none` left at line 48 — suppresses new global electric-blue focus ring. Should be deleted. |
| `task-card.tsx` | Removed 2 pink `focus-visible:ring-*` overrides, kept tag colors using deprecated aliases (brutal-orange/cyan/lavender/lime/stone/red) | **NO** | **P0 a11y**: `focus-visible:outline-none` left at lines 76 and 118. Both should be deleted. Also, deprecated `bg-brutal-orange/cyan/lavender/lime/stone/red` are still in use here — these resolve via aliases, so they work, but for v4.0 hygiene fe2 should sweep them to `bg-brutal-warning/info/violet/success/muted/danger`. |
| `mention-dropdown.tsx` | `rounded-lg border bg-popover shadow-lg` → `border-2 border-black bg-white shadow-brutal-sm rounded-none` | YES ✓ | None. (Verified in commit `7286986` diff.) |
| `agent-selector.tsx` | Replaced raw `<select>` with new `<Select>` from `@/components/ui/select` | YES ✓ | None. |
| `env-editor.tsx` | `active:translate-x-0.5 active:translate-y-0.5` → `active:translate-x-[3px] active:translate-y-[3px]` (2x) | YES ✓ | Minor: hardcoded `style={{ background: '#fffaef' }}` should use `bg-brutal-cream` or `var(--color-brutal-bg)`. Not blocking. |
| `create-task-modal.tsx` | Dialog `border-2 border-black` → `border-brutal-thick` (3px) | YES ✓ | None. |
| `message-input.tsx` | `<Loader2 className="animate-spin" />` → `<Spinner size="sm" />`, `font-display` → `font-heading`, removed `Loader2` from imports | YES ✓ | Comment at line 4 still says "btn-brutal-pink circular icon button" — that class is now yellow, not pink. Cosmetic. |
| `layout.tsx` | `Syne` import + `syne.variable` removed from `<html>` className | YES ✓ | None. |

---

## D. fe1's report of 3 fe2 region issues

| Issue | fe1 fixed? | Notes |
|---|---|---|
| `task-column.tsx:211, 244` pink focus ring | NO — properly escalated to fe2 in `fe1-review-notes.md` 1.2 | Correct scope discipline. fe2 owns task-column. |
| 7+ inline `<Loader2>` spinners in fe2 files | NO — fe1 replaced only the one in `message-input.tsx` (its own file). Documented the rest in `fe1-review-notes.md` 1.3. | Correct. |
| 8 list-row files using `border-b border-black/10` | NO — fe1 migrated only `inbox-item.tsx` (its own file). Documented the other 7 in `fe1-review-notes.md` 2.2. | Correct. |

**Verdict**: fe1 drew the scope boundary correctly. All three items are in fe2 files; fe1 documented them with file:line and recommended fixes. No "顺手改" overreach.

---

## E. Documentation quality

### E.1 `components/ui/README.md` (213 lines) — Good ✓
- All 8 components have a prop table with Type, Default, and Notes.
- Copy-pasteable examples for each component.
- Section 8 ("Color tokens reference") is genuinely useful — the alias/swap table prevents fe2 from getting confused (since `pink` → `primary` and `yellow` → `accent` is non-obvious).
- Minor nit: the README doesn't mention that `Button.variant="default"` is a deprecated alias for `variant="primary"`. fe2 might be surprised when `<Button>` (no variant) renders yellow, not white. Add a sentence.

### E.2 `fe1-review-notes.md` (203 lines) — Good ✓
- 3 severity tiers, all entries have file:line + current state + recommended fix.
- Severity 1 (broken/contrary) lists 3 issues; Severity 2 (visual inconsistency) lists 4; Severity 3 (polish) lists 3.
- "Recommended fe2 sweep order (1 PR per severity)" is a practical execution plan.
- Minor nit: doesn't flag that 7 of the 13 listed issues are concentrated in 2-3 files (message-list, thread-panel, task-column), so a single PR per file might be more efficient than 5 PRs. Not blocking.

### E.3 `fe1-visual-capture-status.md` (74 lines) — Good ✓
- Honest about not being able to run `npm run dev` in the sandbox.
- "How to re-run the visual check" with exact Playwright commands is the right level of detail.
- "What would be captured in screenshots" section enumerates the visual diffs a reviewer should expect. Helpful.

---

## F. Dark mode blind spots

(Full list in A.5; recapping for priority.)

1. **Hardcoded `#000` borders in 10 CSS rules** — `.btn-brutal`, `.card-brutal`, `.input-brutal`, `.navbar-icon`, `.badge-brutal`, `.divider-brutal`, `.message-brutal`, `.mention-highlight`, `.tasknum-highlight`, `.pixel-avatar-cell-filled`. These should use `var(--color-brutal-black)` so they invert in dark mode.
2. **Dead token** `--color-brutal-fg` (line 43) — declared but never used. Remove.
3. **`--color-brutal-violet`** (`#bbafe6`) and **`--color-brutal-muted`** (`#c0b9b1`) on dark background (`#1a1a1a`) — contrast is borderline. Not checked against WCAG. The comment at line 164 says they "read fine" but visual verification needed.

---

## G. CSS regression risks

1. **P0 — focus ring invisible in 2 components** (A.6, C table). fe1 removed the pink ring but left `focus-visible:outline-none` → net result: no visible focus. Keyboard navigation broken for screen-reader / keyboard-only users.
2. **P1 — dark mode borders don't invert** (A.5, F). Visual regression only; not a11y.
3. **P2 — `btn-brutal-pink` class name is now misleading** (A.8). The CSS class is named "pink" but maps to yellow. Works fine; confusing for new readers. Add a `/* v4.0: rename to .btn-brutal-primary */` comment.
4. **P2 — `brutal-pink: var(--color-brutal-primary)` alias in dark mode still resolves to yellow.** No issue, but the class name "pink" makes people assume it's pink. (Same as #3.)

---

## 修复清单（按优先级）

### P0 — 必须修（a11y regression）

1. **[`frontend/components/inbox/inbox-item.tsx:48`]** — `focus-visible:outline-none` removes the global electric-blue focus ring. **Fix:** delete the entire `focus-visible:outline-none` token from the className. The global `*:focus-visible` rule in `globals.brutal.css:477-480` provides the canonical ring.
2. **[`frontend/components/tasks/task-card.tsx:76`]** — same issue on the card root. **Fix:** delete `focus-visible:outline-none` from the className.
3. **[`frontend/components/tasks/task-card.tsx:118`]** — same issue on the parent-task badge. **Fix:** delete `focus-visible:outline-none` from the className.

### P1 — 重要（dark mode consistency）

4. **[`frontend/app/globals.brutal.css`]** — replace 9 hardcoded `border: 2px solid #000` (and `border-top: 2px solid #000` and `border: 2px solid #000;` inside .message-bubble etc.) with `var(--color-brutal-black)` so dark mode actually inverts borders as the comment promises. Files to touch: `.btn-brutal`, `.card-brutal`, `.input-brutal`, `.navbar-icon`, `.badge-brutal`, `.divider-brutal`, `.message-bubble`, `.mention-highlight`, `.tasknum-highlight`.

### P2 — 边缘

5. **[`frontend/app/globals.brutal.css:43`]** — remove dead `--color-brutal-fg: #000000` (and its mention in `@theme inline`).
6. **[`frontend/app/globals.brutal.css:287`]** — add a `/* v4.0: rename to .btn-brutal-primary */` comment on `.btn-brutal-pink` to flag the misleading class name.
7. **[`frontend/components/ui/README.md:39`]** — add a sentence clarifying that `<Button>` (no variant) and `<Button variant="default">` render the **yellow** primary CTA, not a neutral white button. fe2's 9 pages will hit this.

---

## 修复执行

I applied fixes 1-3 (P0) and fix 4 (P1) directly on the `review/fe1` branch. See commit `fix(review): fe2 review findings on fe1 branch` for the diff. The dark-mode border sweep (fix 4) is mechanical and safe.

Fix 5-7 (P2) are listed for visibility but not committed; they can be a follow-up PR.

---

## Re-validation

- `npx tsc --noEmit` in `review/fe1`: **passes** with only the 3 pre-existing `use-computers.ts` errors (unrelated to fe1's worktree).
- No new visual regressions expected from the P0 fixes (re-adding the focus ring is the intent).

---

## Final note

fe1's work is shippable as-is for the v3.0 token layer. The 3 P0 a11y fixes are blocking for a "WCAG AA" claim but won't affect any visual design review. fe2 should pick up these 3 fixes in a sweep before the next design review milestone.
