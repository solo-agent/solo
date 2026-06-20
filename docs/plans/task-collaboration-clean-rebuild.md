# Task collaboration clean rebuild

Goal: rebuild task + relationship changes from `master` with the smallest useful surface.

## Rules

- Keep templates and template UI.
- Keep colleague relationships, but only the types needed for coordination.
- Keep task parent/child DAG UI: subtasks fold inside parent tasks.
- Remove task block/unblock for now: no dependency table, CLI, prompt, or UI.
- Human-only: close and reopen.
- Agent lifecycle: claim, submit, accept, reject; done is allowed only when the agent created the task.

## PRs

### PR0: verifiable master baseline

Fix only test/build failures already present on `master`.

Verify:

- `go test ./... -count=1`
- `cd frontend && npm run build`

### PR1: relationships + templates foundation

Port the minimal relationship model and template UI.

Keep:

- templates and template UI
- colleague relationship needed for coordination

Skip:

- extra relationship types
- team/gallery expansion not needed by templates

Verify:

- backend tests for relationship/template APIs
- frontend build

### PR2: task lifecycle core

Implement the minimal lifecycle contract:

- claim
- submit
- accept
- reject
- done only by creator
- close/reopen only by human

Also align CLI and prompts with the same contract.

Skip:

- block/unblock
- automatic execution-driven status jumps
- deprecated command prose

Verify:

- lifecycle permission tests
- CLI tests
- prompt snapshot/review

### PR3: parent/child task UI

Show subtasks folded inside parent tasks and keep child progress visible.

Skip:

- block/dependency visualization
- graph UI beyond parent/child nesting

Verify:

- frontend build
- focused manual UI review

### PR4: wake routing + collaboration behavior

Route responses with the least server-side rule set:

- explicit mention wakes only mentioned agent
- unmentioned channel message prefers channel leader
- unmentioned thread message prefers thread leader, then thread members

Prompt agents to prefer delegation when a coordinator/leader role exists.

Verify:

- wake routing tests
- manual multi-agent conversation smoke test
