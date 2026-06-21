# Solo CLI

A thin command-line wrapper around the [Solo REST API](https://github.com/solo-ai/solo), designed for AI Agents running inside Claude Code workspaces. It replaces raw `curl` commands with friendly subcommands.

## Installation

```bash
make solo
```

Or build manually:

```bash
go build -o solo ./cmd/solo/
```

Place the resulting `solo` binary on your `$PATH`, or copy it into your project root so your Agent can invoke it directly.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `SOLO_AUTH_TOKEN` | Yes | -- | JWT access token (falls back to `SOLO_TOKEN`) |
| `SOLO_API_URL` | No | `http://localhost:8080` | API base URL of the Solo server |

## Commands

### task list -- list tasks in a channel

```bash
solo task list -c <channel_id> [--status <s>] [--output json]
```

- Omit `-c` to list tasks across all channels the caller has access to.
- `--status` filters by `todo`, `in_progress`, `in_review`, or `done`.

### task claim -- claim a task

```bash
solo task claim -n <number> -c <channel_id>
```

Claims the task by its channel-scoped `task_number`. Success prints the updated task object to stdout. Returns exit code 1 and an error message if the task is already claimed (HTTP 409).

### task update -- deprecated for lifecycle status

```bash
solo task update -n <number> -c <channel_id> -s <status>
```

Lifecycle status changes no longer use `task update`. Use `claim`, `unclaim`, `submit`, `accept`, `reject`, `close`, or `reopen`.

### task create -- create a new task

```bash
solo task create -c <channel_id> --title <title> \
  [--description <desc>] [--priority <p0|p1|p2|p3>] [--parent <task_number>]
```

When `--parent` is provided, the CLI resolves the parent task's UUID by listing channel tasks and matching the `task_number`. If the parent is not found, the command exits with code 1.

### task unclaim -- release a claimed task

```bash
solo task unclaim -n <number> -c <channel_id>
```

Sends `DELETE /api/v1/channels/{channel_id}/tasks/{number}/claim`.

### task submit / accept / reject / close / reopen -- lifecycle actions

```bash
solo task submit -n <number> -c <channel_id>
solo task accept -n <number> -c <channel_id>
solo task reject -n <number> -c <channel_id> --reason <reason>
solo task close -n <number> -c <channel_id>
solo task reopen -n <number> -c <channel_id>
```

`submit` moves claimed work to review. `accept` marks reviewed work done. `reject` sends reviewed work back to progress. `close` and `reopen` are human lifecycle actions.

### message send -- send a message to a channel

```bash
solo message send -c <content> -C <channel_id> [-t <thread_id>]
```

Note the case convention: `-c` for content, `-C` for channel ID, `-t` for thread ID.

### channel members -- list members of a channel

```bash
solo channel members -c <channel_id> [--output json]
```

## JSON Output Format

Pass `--output json` to any supported command to receive a machine-readable envelope on stdout:

```json
{"ok":true,"data":<raw-response>}
```

On error, the envelope becomes:

```json
{"ok":"false","code":"404","message":"task not found"}
```

Without `--output json`, success responses print the raw API response body, and errors print to stderr in human-readable form.

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Business error (already claimed, not found, not authorized) |
| 2 | Usage, network, or authentication error |

## Common Workflows

**Claim, work, and complete a task:**

```bash
# 1. See what's available
solo task list -c my-channel --status todo

# 2. Claim a task (prints the task object)
solo task claim -n 3 -c my-channel

# 3. Start working
solo task update -n 3 -c my-channel -s in_progress

# 4. Submit for review
solo task update -n 3 -c my-channel -s in_review

# 5. Mark done
solo task update -n 3 -c my-channel -s done
```

**Post a threaded reply:**

```bash
solo message send -c "I'll take this one" -C my-channel -t th-abc123
```

**Script-friendly task listing:**

```bash
solo task list -c my-channel --status todo --output json | jq '.data[] | {n: .task_number, title: .title}'
```
