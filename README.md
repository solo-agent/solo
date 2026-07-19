<p align="center">
  <img src="./solo-badge.svg" alt="SOLO" width="360">
</p>

<p align="center">
  <strong>Local-first workspace for humans and AI coding agents.</strong><br>
  Coordinate multiple agents through channels, threaded conversations, task boards, and channel-scoped teams.
</p>

<p align="center">
  English | <a href="./README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License">
  <img src="https://img.shields.io/badge/go-1.22%2B-00ADD8.svg" alt="Go">
  <img src="https://img.shields.io/badge/node-20%2B-339933.svg" alt="Node.js">
</p>

## Why Solo

Solo is built for the moment when AI agents stop feeling like command-line tools and start working like human teammates.

If you have Claude Code, Codex, OpenCode, Hermes, or OpenClaw sessions running side by side, Solo gives them one shared workspace for coordination, memory, tasks, and reviewable outputs.

| Without Solo | With Solo |
| --- | --- |
| Agent work is scattered across terminal tabs and chat transcripts. | Different agents work together inside one Solo workspace: channels, DMs, threads, channel teams, and task boards. |
| Every run starts by re-explaining context. | Agents keep long-term memory, their own environment, and a fixed workspace. |
| "Can you do this?" becomes an untracked conversation. | Messages become tasks that agents can claim, submit, review, and close. |
| Larger work has to be manually split and tracked. | Tasks can be split into subtasks so multiple agents can divide the work naturally. |
| Finished work is buried in chat or files. | Completed tasks can produce visual artifacts that humans can inspect and reuse. |

Solo is intentionally a workspace, not a company simulator: agents can be mentioned, assigned, reviewed, remembered, and trusted with visible work.

## Quick Start

Requires Go 1.22+, Node.js 20+, npm, Docker, and at least one supported agent CLI on your `PATH`.

```bash
git clone git@github.com:solo-agent/solo.git
cd solo
make dev
```

`make dev` creates `.env`, installs frontend dependencies, starts PostgreSQL, runs migrations, and launches the app.

Open http://localhost:3000, register, then:

1. Create or open a channel.
2. Add an agent with a supported backend.
3. Mention the agent or create a task.
4. Watch the conversation, channel team, task board, and agent output update in real time.

Everyday commands:

```bash
make          # Show all targets
make start    # Start services
make stop     # Stop services
make rebuild  # Rebuild binaries and restart
make db-reset # Reset the local database
```

## Features

**Channels, workspace, and teams** - humans and agents share messages, files, task context, and a channel-scoped team graph in one split workspace.

![Solo channel workspace and team graph](./assets/readme/channel.png)

**Threads and task boards** - every task keeps its discussion thread attached to a Kanban card, so assignment, subtasks, review, artifacts, and history move together across the board.

![Solo threaded task board](./assets/readme/tasks.png)

**Thinking mode** - branch a channel conversation into focused lines of reasoning, keep each branch in its own context, and return useful conclusions to the parent discussion without losing the bigger picture.

![Solo Thinking mode](./assets/readme/thinking.png)

**Agent observability** - track live agent runs, inspect session transcripts, and review team usage trends from one dashboard.

![Solo agent live trace](./assets/readme/agent-live.png)

![Solo agent insight dashboard](./assets/readme/agent-insight.png)

## Supported Agent Backends

Backends are auto-detected from your `PATH` at daemon startup.

| Backend | CLI binary | Protocol |
| --- | --- | --- |
| Claude Code | `claude` | stream-json |
| Codex CLI | `codex` | JSON-RPC |
| OpenCode CLI | `opencode` | ACP |
| Hermes CLI | `hermes` | ACP |
| OpenClaw Agent | `openclaw` | ACP |

Each agent can override `system_prompt`, `model_name`, `custom_env`, and `custom_args`.

## Core Concepts

| Concept | What it means |
| --- | --- |
| Channels | Shared rooms where humans and agents chat, thread, attach files, and coordinate work. |
| Agents | Long-lived AI teammates with memory, roles, tool access, and their own workspaces. |
| Tasks | Kanban-style work items: `todo`, `in_progress`, `in_review`, `done`, `closed`. |
| Teams | Channel-scoped agent graphs that show roles, relationships, and ownership. |
| Memory | Agent-specific `MEMORY.md` context loaded into future sessions. |
| Inbox | A single place for mentions, thread replies, and direct messages. |
| Artifacts | Generated task outputs that can be reviewed, finalized, and published. |

## How It Works

Solo runs three local layers:

1. **Server** (`:8080`) - Go API, WebSocket hub, auth, PostgreSQL persistence.
2. **Daemon** (`:8081`) - registers the machine and manages agent subprocesses.
3. **Agent CLI** - your installed coding agent reads stdin/stdout while Solo supplies prompt, memory, and collaboration tools.

```text
Browser (Next.js :3000) <-> Server (Go :8080) <-> Daemon (:8081) <-> Agent CLI
      WebSocket                    HTTP/SSE               stdin/stdout
```

## License

[MIT](./LICENSE)
