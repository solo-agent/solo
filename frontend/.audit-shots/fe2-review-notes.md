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
told NOT to touch. They should be on fe1's radar:

1. `components/dashboard/member-list.tsx:39` — `rounded-md` on
   `member item` row. Should be sharp or use the new `SelectableRow`.
2. `components/dashboard/create-dm-modal.tsx:176` — same `rounded-md`
   on the participant option row.
3. `components/dashboard/dm-list.tsx:78,259` and
   `components/dashboard/channel-list.tsx:46,150` — `rounded-md` and
   `rounded-sm` on the create-channel / create-dm "+" buttons.
4. `components/dashboard/mention-dropdown.tsx:55` — `rounded-lg` +
   `shadow-lg` on a popover. Violates zero-radius + hard-shadow rules.
5. `components/dashboard/member-list.tsx:25-29` — `fill-gray-300` and
   `text-gray-300` (and friends) for member status dots. Should be
   brutal-stone (offline) and brutal-lime (online).
6. `components/dashboard/agent-chunk.tsx:33,99-110` — `border-black/10`,
   `bg-blue-50/50`, `text-blue-600`, `text-red-600` etc. — multiple
   alpha-border + default-color violations in a single file.
7. `components/dashboard/message-input.tsx:478` — `backdrop-blur-sm`
   on the file drop overlay.
8. `components/tasks/create-task-modal.tsx:123` — `backdrop-blur-sm` on
   the modal overlay.
9. `components/inbox/inbox-item.tsx:46` — `border-black/10` between
   message rows.
10. `components/dashboard/message-list.tsx:235` — same `border-black/10`.
11. `components/dashboard/agent-message.tsx:72` and
    `components/dashboard/thread-panel.tsx:62,186` — same.
12. `components/dashboard/agent-track.tsx:32` — same.
13. `components/agents/agent-card.tsx:107` — `text-gray-400`,
    `text-gray-300`.
14. `components/agents/history-tab.tsx:77` — `border-black/10`.
15. `components/teams/teams-graph-view.tsx:108` — `border-black/10`.
16. `components/search/global-search.tsx:258,284` — `border-black/10`
    and `border-black/20`.
17. `components/connection-banner.tsx:78-81` — `bg-amber-500` and
    `bg-green-500` (should be brutal-warning / brutal-success).
18. `components/network-status.tsx:50` — `bg-green-500`.
19. `components/dashboard/typing-indicator.tsx:42-54` — `bg-yellow-500/10`,
    `bg-blue-500/10`, `bg-green-500/10`.
20. `components/agents/agent-skills-tab.tsx:124`,
    `agent-runtime-tab.tsx:84`, `agent-profile-tab.tsx:271` — raw
    `<button>` with `btn-brutal-sm` class.
21. `components/teams/teams-human-profile.tsx:69` — same raw button.
22. `components/ui/dialog.tsx:43` — `backdrop-blur-sm` on the modal
    backdrop (intentionally kept by fe2 to land in a later PR; out of
    scope for this round).
23. `app/teams/page.tsx:184,197` — emoji (👤, 📁) as tab icons. Should
    use `lucide-react` `User`, `FolderOpen`.

These are listed so fe1 has a clear backlog of design-debt fixes to
prioritise. The audit-brutal.sh script will flag each of them on a
fresh run.

## fe2 commit log

This worktree is structured as 10+ focused commits (see `git log
feat/fe2-brutal-pages` from master base). The commit-level review is
intentionally granular so fe1 can review one concern per commit.
