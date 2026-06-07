# fe1 Review Notes — UI inconsistencies in fe2's region

> **Worktree:** `wt-fe1` (branch `feat/fe1-brutal-foundation`)
> **Scope:** fe1 only documents. All fixes below belong to fe2 (their worktree).
> **Date:** 2026-06-06

This file lists every UI inconsistency I observed while building the
v3.0 token layer + shared component library. Each entry is concrete and
reproducible. fe2 can execute them as a sweep.

For each item I include: the file:line, the current state, the
recommended fix, and a short rationale.

---

## Severity 1 — broken/contrary-to-system

### 1.1 Four files still use the `font-mono` token for prose text, not code/IDs

The v3.0 token rules say `--font-mono` is for code/IDs/tabular numerals
only. Prose, metadata labels, and badges should use `font-body` /
`font-heading`.

| File:line | Current | Recommended |
| --- | --- | --- |
| `components/tasks/task-card.tsx:158` | `<span className="font-mono">#{task.channel_name}</span>` | `<span className="font-body text-muted-foreground">#{task.channel_name}</span>` (channels are prose, not IDs) |
| `components/dashboard/message-list.tsx:412` (search around the channel span) | mixed `font-mono` on timestamps | `text-xs text-muted-foreground` |
| `components/dashboard/agent-message.tsx` | `font-mono` on agent name + time block | use `font-heading` for name, `text-xs text-muted-foreground` for time |
| `components/dashboard/message-input.tsx:486,650` (after my edit) | already migrated in the drag overlay + help text | no action |

**Why:** Mono is heavy and noisy in chat rows. We use mono only for IDs
(channel number, task number, file size, timestamps with sub-second
precision). The audit showed ~14 files using mono on prose.

---

### 1.2 Two `focus-visible:ring-brutal-pink` overrides remain

I removed these from `inbox-item.tsx`, `task-card.tsx`,
`message-input.tsx`. Two more still exist in fe2's region:

| File:line | Pattern |
| --- | --- |
| `components/tasks/task-column.tsx:211` | `focus-visible:ring-2 focus-visible:ring-brutal-pink focus-visible:ring-offset-2` |
| `components/tasks/task-column.tsx:244` | same (parent task badge) |

**Fix:** delete both `focus-visible:ring-*` clauses. The v3.0 global
`* :focus-visible { outline: 3px solid var(--color-brutal-info); }` in
`globals.brutal.css:280-283` already provides the canonical electric
blue ring.

**Why:** pink focus ring on a 2px black border looks noisy and breaks
visual coherence (pink = call-to-action, not selection state).

---

### 1.3 At least 7 inline `<Loader2 className="… animate-spin" />` patterns remain

I replaced only the ones in fe1's files. fe2's region has 7+ more:

| File:line | Notes |
| --- | --- |
| `components/dashboard/channel-view.tsx:547` | full-page loading state, `h-5 w-5` → use `<Spinner size="md" />` |
| `components/dashboard/create-dm-modal.tsx:242` | inline `h-3 w-3` → use `<Spinner size="sm" />` |
| `components/dashboard/message-list.tsx:321, 599` | two spinners, both inline |
| `components/dashboard/typing-indicator.tsx:45` | inside an icon slot, can keep the prop-driven config |
| `components/dashboard/channel-search.tsx:169, 185` | two spinners |
| `components/dashboard/message-attachments.tsx:95, 156` | two ad-hoc div spinners (not even Loader2) — direct candidates for `<Spinner />` |
| `components/tasks/task-form.tsx` (search) | one inline submit spinner |

**Why:** every ad-hoc spinner has a slightly different `border-2 / h-X w-X / border-t-transparent` recipe. `<Spinner />` standardizes the color to `brutal-primary` and the aria-label, which also helps screen readers (the inline versions have no `aria-label`).

---

## Severity 2 — visual inconsistency

### 2.1 `border-l-brutal-pink` (old alias) used in 5 places

After my Phase A rename, `border-l-brutal-pink` resolves to the
deprecated alias for `border-l-brutal-accent`. v4.0 will remove the
alias, so fe2 should sweep these now:

| File:line | Recommended |
| --- | --- |
| `components/dashboard/streaming-message.tsx:113` | `border-l-brutal-accent` |
| `components/dashboard/message-list.tsx:238` | same |
| `components/dashboard/agent-message.tsx:72` | same |
| `components/dashboard/thread-panel.tsx:62, 186` | same |

> The CSS class is named `border-l-brutal-pink` in `globals.brutal.css:519`
> — the *class name* itself is the alias. If we want to retire it in v4.0,
> fe2 should rename to `border-l-brutal-accent` so the class deletion is a
> single PR.

---

### 2.2 `border-b border-black/10` used in 9 list-row patterns

I migrated `inbox-item.tsx` from `border-b border-black/10` to
`border-b-2 border-black` for visual consistency with the rest of the
sidebar. fe2 has the same anti-pattern in 8 more files:

| File:line | Notes |
| --- | --- |
| `components/dashboard/message-list.tsx:235` | message row |
| `components/dashboard/agent-message.tsx:72` | agent message row |
| `components/dashboard/thread-panel.tsx:62, 186` | thread header + reply row |
| `components/dashboard/agent-track.tsx:32` | agent track header |
| `components/agents/history-tab.tsx:77` | history item row |
| `components/search/global-search.tsx:284` | search result row |
| `components/teams/teams-graph-view.tsx:108` | section divider |

**Recommended fix:** all `border-b border-black/10` → `border-b-2 border-black`
(visible separator, matches `inbox-item.tsx`).

The narrower `border-black/20` and `border-black/5` in
`task-card.tsx:134,138` and `task-column.tsx:296,300` are inside cards
and intentionally subtle — leave those.

---

### 2.3 `rounded-md` still in 4 list-row components

Brutal canon: `rounded-none` or `rounded-full`, no in-between. The
following files use `rounded-md`:

| File:line | Element | Fix |
| --- | --- | --- |
| `components/dashboard/member-list.tsx:39` | member row | `rounded-none` |
| `components/dashboard/dm-list.tsx:78` | DM "add DM" pill button | `rounded-none` + add 2px border for visual weight |
| `components/dashboard/channel-list.tsx:46` | channel "browse" pill button | same as above |
| `components/dashboard/create-dm-modal.tsx:176` | DM candidate row | `rounded-none` |

> `search/global-search.tsx:258` has a `<kbd>` element with `rounded border`
> — that's the only acceptable exception (keyboard chips are a known
> 2px-rounded pattern). Leave it.

---

### 2.4 Two `btn-brutal btn-brutal-sm btn-brutal-pink` chains

The `btn-brutal btn-brutal-sm` chains exist everywhere; v3.0 of
`<Button size="sm" variant="primary">` is the replacement.

| File:line | Recommended |
| --- | --- |
| `components/dashboard/message-list.tsx:339` | `<Button size="sm" variant="primary">保存</Button>` |
| `components/tasks/create-task-modal.tsx:210` | already replaced in my Phase C; the file also has line 202 (`bg-white` only) — could become `<Button size="sm" variant="outline">` |

> The chain `btn-brutal btn-brutal-sm bg-white` (no pink) appears in
> `create-task-modal.tsx:202` and is essentially the same as
> `variant="outline" size="sm"`.

---

## Severity 3 — minor polish

### 3.1 `border-l-brutal-gray` still in `globals.brutal.css:524`

After the rename, this is the only `border-l-brutal-` utility still
using an old alias. It points to `var(--color-brutal-muted)` (correct)
but the class name should be `border-l-brutal-muted` for v4.0.

> Not urgent — alias works. But while fe2 is sweeping the list-row
> files, they can add the rename.

---

### 3.2 20 inline `border-2 border-black shadow-brutal-sm` patterns

I left these alone (they're the correct recipe). However, this pattern
repeats 20 times across `dashboard/`, `tasks/`, and `workspace/`
components. A `<BrutalBox>` primitive (a div with the brutal shadow +
border baked in) would be a natural follow-up — but it's outside fe1's
scope; flag for fe2's call.

---

### 3.3 `font-mono text-[10px]` still ubiquitous

The audit shows ~20 files using `font-mono` + tiny text (10-11px) for
metadata. After my Phase C in `inbox-item.tsx`, the pattern is
`text-xs text-muted-foreground` (system font, semantic color).
The same swap applies broadly — but this is a sweep task, not a
single-file fix.

---

## Recommended fe2 sweep order (1 PR per severity)

1. **PR 1 — focus rings:** delete the two remaining
   `focus-visible:ring-brutal-pink` clauses in `task-column.tsx`.
2. **PR 2 — spinners:** replace 7+ inline `Loader2` / ad-hoc div
   spinners with `<Spinner>`.
3. **PR 3 — border bottom weight:** swap `border-b border-black/10`
   → `border-b-2 border-black` across 8 list-row files.
4. **PR 4 — rounded corners:** drop `rounded-md` in 4 list-row
   components (the canonical brutal pattern is square + border).
5. **PR 5 — alias renames:** sweep `border-l-brutal-pink` and any
   other remaining hex-named alias to its semantic name.

Each PR is small, mechanical, and non-breaking thanks to the v3.0 alias
layer.
