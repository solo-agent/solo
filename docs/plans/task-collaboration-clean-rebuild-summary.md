# Task collaboration clean rebuild summary

Branch: `codex/task-collab-clean`

Base: `master` at `901e2d7`

## Size

Compared with `master`:

```text
38 files changed, 2743 insertions(+), 261 deletions(-)
```

By commit:

| Commit | Purpose | Size |
|---|---|---:|
| `a54926a` | Fix master baseline verification | 7 files, +124/-6 |
| `0d6e26b` | Add minimal agent relationships | 7 files, +447/-4 |
| `741be2f` | Add task submit/accept/reject lifecycle | 8 files, +374/-107 |
| `b233169` | Fold subtasks under parent task cards | 2 files, +59/-5 |
| `35d463d` | Route wakes to mentioned agents or coordinator | 2 files, +207/-136 |
| `9c5ebc0` | Restore team templates and workspace relationship docs | 15 files, +785/-13 |
| latest | Add slim relationship graph editor | 5 files, +608/-4 |

Note: the final wake-routing commit includes `gofmt` cleanup in `internal/server/service/agent.go`, so the raw line count is larger than the semantic change.

## Added vs master

### Relationships

- Added `agent_relationships` migration.
- Added minimal relationship API:
  - create
  - list
  - list by agent
  - update
  - delete
- Kept only two relationship types:
  - `assigns_to`
  - `collaborates_with`
- `collaborates_with` is unique regardless of direction.
- `assigns_to` is directional.
- Relationships affect runtime in two ways:
  - server wake routing uses `assigns_to` to find the coordinator
  - relationship/template changes generate `RELATIONSHIPS.md` in the agent workspace

The agent prompt tells agents to read `RELATIONSHIPS.md` before deciding whether to coordinate, delegate, claim, or collaborate. There is no second direct `TaskContext` relationship injection path.

### Relationship graph UI

- Added `/relationships` as a slim relationship graph editor.
- Added left-nav entry for Relationships.
- The graph supports:
  - auto layout from `assigns_to`
  - drag nodes and persist positions in localStorage
  - create relationships
  - edit relationship instruction
  - delete relationships
- It intentionally avoids new graph dependencies (`@xyflow/react`, `dagre`) and uses a small local SVG/absolute-node implementation.

### Team templates

- Added `agent_templates` migration with starter team templates.
- Added template API:
  - `GET /api/v1/templates`
  - `POST /api/v1/templates/{templateID}/apply`
- Applying a template creates:
  - agents
  - `assigns_to` relationships from the leader to members
  - channel membership in the user's `all-*` channel when available
- Added `/teams` create flow:
  - Single Agent
  - From Template
- Runtime must be selected before applying a template.

### Task lifecycle

- Added business lifecycle actions:
  - `submit`: claimer moves task from `in_progress` to `in_review`
  - `accept`: creator moves task from `in_review` to `done`
  - `reject`: creator moves task from `in_review` back to `in_progress`
- Added HTTP routes:
  - `POST /api/v1/channels/{channelID}/tasks/{taskID}/submit`
  - `POST /api/v1/channels/{channelID}/tasks/{taskID}/accept`
  - `POST /api/v1/channels/{channelID}/tasks/{taskID}/reject`
- Added CLI commands:
  - `solo task submit`
  - `solo task accept`
  - `solo task reject`
- Updated agent prompt so agents use lifecycle commands instead of `solo task update -s` for review flow.
- Raw status update compatibility now enforces:
  - only the creator can move a task to `done`
  - `close` and `reopen` are human-only actions

### Task UI

- Parent tasks now own visible child tasks in the board.
- Child tasks no longer appear as separate cards when their parent is present in the loaded task list.
- Parent cards show a collapsible subtask list with:
  - child task status color
  - child task number
  - child task title
- Orphan child tasks still appear normally so filtering or partial data does not hide tasks.

### Wake routing

- Explicit agent mention wakes only mentioned agents.
- A message with unresolved `@...` patterns wakes no agents.
- Unmentioned channel messages wake one coordinator.
- Unmentioned thread messages wake:
  1. coordinator among existing thread agent participants
  2. root message agent if present
  3. channel coordinator fallback
- New task with mentions wakes only mentioned agents.
- New task without mentions wakes one coordinator.
- Coordinator selection:
  - prefer the root of the `assigns_to` relationship graph
  - fallback to first active channel agent by stable join/create order

### Baseline fixes

- Added `local` backend alias for Claude Code.
- Fixed memory tests to match current workspace memory path.
- Fixed one CLI test using unsupported legacy args.
- Added missing frontend type/i18n fields.
- Wrapped `/tasks` page in `Suspense` for `useSearchParams`.

## Removed or intentionally not ported

- No block/unblock task dependency model.
- No task dependency migration/table.
- No `solo task block`, `unblock`, or `blocked`.
- No dependency popover or block DAG UI.
- No old full relationship graph editor.
- No relationship MiniMap.
- No relationship WebSocket live-sync.
- No separate `/relationships/manage` table page.
- No relationship SVG export or undo/redo.
- No relationship event publisher.
- No old full `RELATIONSHIPS.md` report with recent activity/task-count sections.
- No channel-scoped relationship model.
- No extra relationship types.
- No full old template gallery component; the template picker is integrated into the existing `/teams` create dialog.
- No large old template catalog beyond starter team templates.
- No automatic execution-driven transition to review.

## Differences from the original plan

### PR0

Unchanged. We kept baseline verification as its own commit.

### PR1

Changed after audit: templates were restored after the initial clean rebuild missed them.

Reason: the user explicitly asked to keep templates and their UI. The final version includes template backend/API, seed migration, `/teams` UI entry, workspace `RELATIONSHIPS.md` generation, and a slim `/relationships` graph editor. It intentionally avoids a second direct prompt-injection path for the same relationship data.

### PR2

Changed: old `PATCH status` remains available.

Reason: the current server does not cleanly distinguish human UI actions from agent CLI actions at this layer. Instead, agents are guided to use `submit/accept/reject`, while the compatibility endpoint remains for existing UI.

### PR3

Unchanged in spirit, but intentionally minimal.

We implemented nested subtasks in the board without adding a full graph view or dependency visualization.

### PR4

Changed: “leader” is derived from `assigns_to`, not from a separate role column or template key.

Reason: we kept the architecture smaller. The coordinator is the agent with no parent in the assignment graph; without relationships, the first active channel agent responds.

## Possible surprises

- `internal/server/service/agent.go` has a larger diff because `gofmt` fixed existing indentation while touching the file.
- `solo task update -s` still exists for compatibility, but the agent prompt no longer teaches it as the lifecycle path.
- Relationship routing assumes `assigns_to` means `leader assigns_to worker`.
- Relationships write a lean `RELATIONSHIPS.md`; runtime sees relationships by reading that workspace file, plus server wake routing uses the same relationship graph.
- `/relationships` keeps Auto Layout and drag position persistence, but does not include MiniMap, WebSocket live-sync, SVG export, undo/redo, or the old manager table page.
- Wake routing now reduces fan-out. This is intended, but it means a channel with no relationships will pick one stable fallback agent instead of all agents.
- Subtasks are folded only when parent and child are both present in the loaded task list.

## Explicit requirement audit

| User requirement | Current status |
|---|---|
| Keep colleague relationships | Kept |
| Keep templates and template UI | Restored after audit |
| Keep relationship graph Auto Layout | Kept in slim `/relationships` editor |
| Keep relationship graph drag position persistence | Kept via localStorage |
| Keep only useful relationship types | Kept: `assigns_to`, `collaborates_with` |
| Remove block/unblock | Removed / not ported |
| Keep task parent/child DAG UI | Kept as folded subtasks under parent cards |
| Keep wake behavior responsive when no mention | Kept: coordinator fallback |
| Explicit mention should be precise | Kept: only mentioned agents wake |
| Claim should not be absolutely first if coordinator should delegate | Kept in prompt: claim before executing, delegate first when coordinating |
| Agent can mark done when agent is creator | Kept: `accept` moves creator-owned reviewed work to `done`; raw `done` is also creator-only |
| Close/reopen human-only | Kept: agent actors are rejected for `close` and `reopen`; humans keep old status compatibility |

## Verification

Passed:

```bash
go test ./... -count=1
cd frontend && npm run build
```
