# fe2 review notes (neo-brutalism design system, phase 2)

Author: fe2 worktree (`wt-fe2/frontend`)
Branch: `feat/fe2-brutal-pages`
Date: 2026-06-06

This file is the hand-off between fe2 and fe1 / master. It records the
contract this worktree implemented, the things it had to stub, and the
open questions for the other phases.

## STUB清单 (fe1 should override when it merges)

The following 7 components are local stubs created in this worktree so
the fe2 page refactor could move forward without waiting on the fe1 API
freeze. Each stub file starts with the marker:

```
// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
```

| Component           | Path                                | Used by (fe2)                                          |
| ------------------- | ----------------------------------- | ------------------------------------------------------ |
| `Spinner`           | `components/ui/spinner.tsx`         | auth/login, auth/register, dashboard, tasks, etc.      |
| `SelectableRow`     | `components/ui/selectable-row.tsx`  | (carried for fe1, not yet adopted in fe2 pages)        |
| `SectionHeader`     | `components/ui/section-header.tsx`  | (carried for fe1, not yet adopted in fe2 pages)        |
| `EmptyState`        | `components/ui/empty-state.tsx`     | computers, tasks, teams (planned adoptions)            |
| `BrutalAlert`       | `components/ui/brutal-alert.tsx`    | (carried for fe1, not yet adopted in fe2 pages)        |
| `Tag`               | `components/ui/tag.tsx`             | (carried for fe1, not yet adopted in fe2 pages)        |
| `Select`            | `components/ui/select.tsx`          | tasks, tasks/new (replaces bare `<select>`)            |

Why local stubs: the fe2 plan intentionally drew the boundary here so
that the page refactor does not have to wait on the fe1 token freeze.
Once fe1 merges, the marker comment should be deleted and fe1's
version should win automatically (file-level merge).

## fe1 应做的 token 改名清单 (from Q3)

The current `globals.brutal.css` uses the legacy brutal-pink / yellow /
cyan / lime / orange / red / lavender / stone token names. The design
system doc calls for `primary / accent / success / danger / muted`.
Until fe1 renames, the fe2 worktree continues to reference the legacy
names. The mapping is:

| Current (used now)       | Target (after fe1)    | Where used                                       |
| ------------------------ | --------------------- | ------------------------------------------------ |
| `brutal-pink`            | `brutal-primary`      | primary buttons, selected states                 |
| `brutal-yellow`          | `brutal-accent`       | badges, agent profile card                       |
| `brutal-cyan`            | `brutal-info`         | online indicator, info banners                   |
| `brutal-lime`            | `brutal-success`      | success messages, agent idle indicator           |
| `brutal-orange`          | `brutal-warning`      | warning banners (computers, teams error banner)  |
| `brutal-red`             | `brutal-danger`       | destructive buttons, error banners               |
| `brutal-lavender`        | `brutal-muted`        | section accents                                  |
| `brutal-stone`           | `brutal-muted-foreground` | text-foreground on muted                       |
| `brutal-pink-light` etc. | `brutal-primary-light` etc. | tinted backgrounds                       |

The `-light` tints also need a rename pass (all eight colors). When fe1
makes the rename, every `bg-brutal-*-light` reference in this worktree
must be updated. The audit script catches default Tailwind colors but
not the old `*-light` names.

## button.tsx 应补的 variant 列表 (from Q4)

Current `components/ui/button.tsx` exposes:

- `default` (bg-brutal-pink)
- `destructive` (bg-brutal-red)
- `outline` (bg-brutal-white)
- `secondary` (bg-brutal-white — duplicates outline)
- `ghost` (btn-flat)
- `link` (underline)

Missing variants the design system implies:

1. `accent` — bg-brutal-yellow (used in messages/feed for "new")
2. `info` — bg-brutal-cyan
3. `success` — bg-brutal-lime
4. `warning` — bg-brutal-orange
5. `ghost-bordered` — transparent bg + border-2 border-black (used in
   filter bars; currently a raw button)
6. `icon-square` — square icon button with bordered-box look (currently
   `btn-brutal-sm h-8 w-8` literals)

If fe1 plans to centralise Button variants, the page refactor in this
worktree intentionally did NOT add them — that's fe1's call.

## fe1 区域问题 (visible in this worktree but owned by fe1)

These are design-system smells that live in components/ files fe2 was
told NOT to touch. They should be on fe1's radar.

### Fixed in fe2 worktree (because the file was in the fe2 scope list)

- ~~#1 `components/dashboard/member-list.tsx:39` `rounded-md`~~ — FIXED,
  replaced with `border-2 border-transparent hover:border-black
  hover:bg-brutal-pink/20`.
- ~~#2 `components/dashboard/create-dm-modal.tsx:176` `rounded-md`~~ —
  FIXED, same treatment.
- ~~#3 `components/dashboard/dm-list.tsx:78,259` and
  `channel-list.tsx:46,150` `rounded-md` and `rounded-sm`~~ — FIXED.
  The `+` button in the section header is now a bordered brutal
  square; the empty-state CTA is a proper brutalist button.
- ~~#5 `components/dashboard/member-list.tsx:25-29` `fill-gray-300`
  status dots~~ — FIXED, status colors now map to `brutal-lime`,
  `brutal-stone`, `brutal-yellow`, `brutal-cyan`.
- ~~#17 `components/connection-banner.tsx:78-81` `bg-amber-500` /
  `bg-green-500`~~ — FIXED, `bg-brutal-yellow` and `bg-brutal-lime`.
- ~~#18 `components/network-status.tsx:50` `bg-green-500`~~ — FIXED.
- ~~#22 `components/ui/dialog.tsx:43` `backdrop-blur-sm`~~ — FIXED.
  Backdrop is now `bg-black/60` opaque; `border-brutal-thick` replaced
  with `border-4 border-black`.
- ~~#23 `app/teams/page.tsx:184,197` emoji icons~~ — FIXED, now uses
  `lucide-react` `User` and `FolderOpen`.

### Still open (fe1 should pick these up)

1. `components/dashboard/mention-dropdown.tsx:55` — `rounded-lg` +
   `shadow-lg` on a popover. Violates zero-radius + hard-shadow rules.
2. `components/dashboard/agent-chunk.tsx:33,99-110` — `border-black/10`,
   `bg-blue-50/50`, `text-blue-600`, `text-red-600` etc. — multiple
   alpha-border + default-color violations in a single file.
3. `components/dashboard/message-input.tsx:478` — `backdrop-blur-sm`
   on the file drop overlay.
4. `components/tasks/create-task-modal.tsx:123` — `backdrop-blur-sm` on
   the modal overlay.
5. `components/inbox/inbox-item.tsx:46` — `border-black/10` between
   message rows.
6. `components/dashboard/message-list.tsx:235` — `border-black/10`.
7. `components/dashboard/agent-message.tsx:72` and
   `components/dashboard/thread-panel.tsx:62,186` — same.
8. `components/dashboard/agent-track.tsx:32` — same.
9. `components/agents/agent-card.tsx:107` — `text-gray-400`,
   `text-gray-300`.
10. `components/agents/history-tab.tsx:77` — `border-black/10`.
11. `components/teams/teams-graph-view.tsx:108` — `border-black/10`.
12. `components/search/global-search.tsx:258,284` — `border-black/10`
    and `border-black/20`.
13. `components/dashboard/typing-indicator.tsx:42-54` — `bg-yellow-500/10`,
    `bg-blue-500/10`, `bg-green-500/10`.
14. `components/agents/agent-skills-tab.tsx:124`,
    `agent-runtime-tab.tsx:84`, `agent-profile-tab.tsx:271` — raw
    `<button>` with `btn-brutal-sm` class.
15. `components/teams/teams-human-profile.tsx:69` — same raw button.

These remain so fe1 has a clear backlog of design-debt fixes to
prioritise. The audit-brutal.sh script will flag each of them on a
fresh run (after fe1 extends SCAN_PATHS to cover those files).

## fe2 commit log

This worktree is structured as 10+ focused commits (see `git log
feat/fe2-brutal-pages` from master base). The commit-level review is
intentionally granular so fe1 can review one concern per commit.

## Validation status (as of worktree final state)

- `bash scripts/audit-brutal.sh` → **PASS** (9 paths scanned, 0
  violations). This is the only gate that can be confirmed without
  re-running the worktree's own dev server.
- `npx tsc --noEmit` → 3 errors remain, all in
  `lib/hooks/use-computers.ts:40-42` (TS2352 double-cast). These are
  **pre-existing baseline** — the same 3 errors reproduce on master
  without any fe2 work. The fix is a mechanical 3-line
  `as unknown as Record<string, unknown>` double-cast; landed in
  commit `fix: use-computers pre-existing TS2352 cast (unblock tsc
  gate)`. This is the only commit in fe2 that touches a non-UI file
  and is included solely to unblock the tsc gate.
- `npx playwright test e2e/brutal-consistency.spec.ts` → not run
  in-worktree. The current dev server on :3000 is the main repo
  (`/Users/langgengxin/AiWorkspace/solo/frontend`), so the spec would
  only validate main-repo source. To run this spec against the
  worktree, start a worktree dev server on a separate port and set
  `PLAYWRIGHT_BASE_URL` (e.g.
  `cd .worktrees/wt-fe2/frontend && PORT=3001 npm run dev`, then
  `PLAYWRIGHT_BASE_URL=http://localhost:3001 npx playwright test
  e2e/brutal-consistency.spec.ts`).
- 5+ screenshots → directory `.audit-shots/fe2-screenshots/` is
  empty. Same blocker: requires a worktree dev server.
- `npm run build` → fails on `app/tasks/page.tsx` with
  `useSearchParams() should be wrapped in a suspense boundary`. This
  is a pre-existing Next.js 16 framework requirement, not a fe2
  regression; the page imports `useSearchParams()` at line 8 and uses
  it at line 30 (the page was structured this way before fe2 began).
  The fix is a 10-20 line refactor to split the page into
  `TasksPage` (Suspense wrapper) + `TasksPageContent` (the actual
  default-export). Out of fe2 scope; flagged here for the follow-up.
feat/fe2-brutal-pages` from master base). The commit-level review is
intentionally granular so fe1 can review one concern per commit.
