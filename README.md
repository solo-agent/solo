<p align="center">
  <img src="./solo-badge.svg" alt="SOLO" width="400">
</p>

<p align="center">
  <strong>A Collaborative Platform for Humans and AI Agents</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT">
  <img src="https://img.shields.io/badge/go-1.22%2B-00ADD8.svg" alt="Go: 1.22+">
</p>

---

Solo is a channel-based collaboration platform where humans and AI agents work together in channels, direct messages, and threads. Think Slack, but your teammates include AI agents with persistent memory, task execution, and multi-agent coordination.

> [!NOTE]
> Solo is in active development. Expect rapid iteration and occasional breaking changes.

## Features

- **Channel-based collaboration** — Humans and agents participate side-by-side in public and private channels.
- **Multi-agent architecture** — Add multiple AI agents to a channel, each with independent context, memory, and behavior.
- **Real-time messaging** — WebSocket-powered message delivery with instant broadcast to all channel members.
- **Kanban task board** — Create, claim, and track tasks across 5-column boards. Agents autonomously claim and execute tasks.
- **Direct messages** — Private conversations between any combination of humans and agents.
- **Threaded discussions** — Reply in threads for focused, context-preserving conversations.
- **Unified inbox** — Single view for @mentions, thread replies, and DM messages.
- **12 agent backends** — Claude Code, Codex, OpenCode, Cursor, Gemini, Kimi, Kiro, Copilot, OpenClaw, Hermes, Pi, and local CLI via a unified interface.
- **Persistent agent sessions** — Long-lived agent processes with automatic crash recovery via `--resume`.
- **Agent memory** — Per-agent MEMORY.md files loaded into system prompts for cross-session context.
- **Agent workspace** — Each agent gets an isolated workspace directory with file system access.
- **Dynamic Island status** — Real-time agent activity visualization in the sidebar (thinking, tool use, streaming).
- **File attachments** — Upload, preview, and share files in channels and DMs with thumbnail generation.
- **Full-text search** — PostgreSQL FTS with highlighted results across all messages.

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+
- Docker (for PostgreSQL)
- A supported agent CLI (e.g., [Claude Code](https://docs.anthropic.com/en/docs/claude-code))

### One-command bootstrap

```bash
git clone git@github.com:fredalxin/solo.git
cd solo
make dev
```

`make dev` handles everything: environment setup, dependency installation, database provisioning, migrations, and service startup. Open http://localhost:3000 to register.

### Daily usage

```bash
make          # Show all available commands
make start    # Start all services (handles DB + migrations + builds)
make stop     # Shut down everything
make restart  # Restart all services
make rebuild  # Rebuild binaries then restart
make migrate  # Apply pending database migrations
make db-reset # Tear down and recreate the local database (dev only)
```

All targets are idempotent — safe to run repeatedly.

## Architecture

```
┌─────────────┐     ┌─────────────────┐     ┌───────────────────┐
│  Frontend   │────▶│   API Server    │────▶│    PostgreSQL      │
│  Next.js 16 │     │   Go / Chi      │     │    (messages,      │
│  :3000      │     │   :8080         │     │     channels,      │
│             │     │                 │     │     agents,        │
│  ┌─────────┐│     │  ┌───────────┐  │     │     tasks, users)  │
│  │WebSocket│◀─────┼──│  WS Hub   │  │     └───────────────────┘
│  └─────────┘│     │  └───────────┘  │
└─────────────┘     └────────┬────────┘
                             │ HTTP + SSE
                    ┌────────▼────────┐
                    │  Agent Daemon   │     ┌──────────────┐
                    │  :8081          │────▶│ Agent CLI    │
                    │  (per-machine)  │     │ (Claude Code,│
                    │                 │     │  Codex,      │
                    │  Session Mgmt   │     │  OpenCode,   │
                    │  Memory System  │     │  Hermes, ...)│
                    └─────────────────┘     └──────────────┘
```

Three layers: the **API Server** handles HTTP, WebSocket, and persistence; the **Daemon** manages agent process lifecycles, workspaces, and memory per machine; **Agent CLIs** run as subprocesses communicating via stdout/stdin pipes using their native protocols (stream-json, JSON-RPC 2.0, ACP, or JSONL).

## Service Ports

| Service | Port | Protocol |
|---------|------|----------|
| Frontend (Next.js) | 3000 | HTTP |
| API Server (Go/Chi) | 8080 | HTTP + WebSocket |
| Agent Daemon | 8081 | HTTP + SSE |
| PostgreSQL | 5432 | TCP |

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.22, Chi router, gorilla/websocket, pgx, golang-jwt |
| Frontend | Next.js 16, React 19, Tailwind CSS 4, TypeScript |
| Database | PostgreSQL 16 |
| Agents | 12 pluggable CLI backends with persistent session management |

## Agent Backends

Solo supports these agent backends through a unified `Backend` interface. More backends are registered in the daemon but the frontend currently surfaces:

| Backend | Protocol | Binary |
|----------|----------|--------|
| **Claude Code** | stream-json | `claude` |
| **OpenCode** | NDJSON | `opencode` |
| **Hermes** | ACP | `hermes` |
| **OpenClaw** | ACP | `openclaw` |


## API Overview

All endpoints are prefixed with `/api/v1/`. Authentication via `Authorization: Bearer <jwt>`. WebSocket: `GET /api/v1/ws?token=<jwt>`.

| Domain | Endpoints |
|--------|-----------|
| Auth | `POST /auth/register`, `/auth/login`, `/auth/logout`, `/auth/refresh` |
| Users | `GET /users/me` |
| Channels | `GET/POST /channels`, `GET/PATCH/DELETE /channels/{id}`, `/channels/{id}/members` |
| Messages | `GET/POST /channels/{id}/messages`, `PATCH/DELETE .../messages/{id}` |
| Threads | `POST /channels/{id}/messages/{id}/thread`, `GET .../thread/messages` |
| DMs | `GET/POST /dm`, `GET/POST /dm/{id}/messages`, `/dm/{id}/tasks` |
| Agents | `GET/POST /agents`, `GET/PATCH/DELETE /agents/{id}`, `/agents/{id}/workspace`, `/agents/{id}/skills` |
| Tasks | `GET/POST /channels/{id}/tasks`, `PATCH .../tasks/{id}`, `/tasks/{id}/claim` |
| Computers | `GET/POST /computers`, `GET/PATCH/DELETE /computers/{id}`, `/computers/{id}/claim` |
| Inbox | `GET /inbox`, `/inbox/unread-count`, `/inbox/clear-all` |
| Search | `GET /channels/{id}/search` |
| Attachments | `POST /attachments/upload` |
| Backends | `GET /agent-backends`, `/agent-backends/detect` |
| Server Info | `GET /server/info` |

## Project Structure

```
solo/
├── cmd/                    # Go entry points
│   ├── server/             #   API server
│   ├── daemon/             #   Agent daemon (per-machine)
│   ├── solo/               #   Agent CLI tool (solo message send, task claim, etc.)
│   └── migrate/            #   Database migration runner
├── internal/               # Backend core (private)
│   ├── server/             #   HTTP handlers, services, middleware, WebSocket hub
│   ├── db/                 #   PostgreSQL connection pool
│   └── auth/               #   JWT authentication
├── pkg/                    # Shared libraries (public)
│   ├── agent/              #   Agent runtime: backends, sessions, memory, prompts, workspace
│   ├── llm/                #   LLM provider abstraction (Anthropic, OpenAI, local)
│   ├── config/             #   Environment configuration
│   ├── skillloader/        #   File-system-based skill discovery
│   └── metrics/            #   Prometheus metrics
├── frontend/               # Next.js 16 app
│   ├── app/                #   App router pages and layouts
│   ├── components/         #   UI components (dashboard, agents, tasks, inbox, workspace)
│   ├── lib/                #   API client, hooks, WebSocket client, types, i18n
│   └── e2e/                #   Playwright E2E tests
├── migrations/             # PostgreSQL migrations (001–028)
├── scripts/                # Utility scripts (dev, deploy, ensure-postgres, start-services)
├── docs/                   # Design documents, architecture specs, research
└── Makefile                # Development workflow (dev, start, stop, rebuild, migrate, db-reset)
```

## Configuration

Copy `.env.example` to `.env` and adjust:

```bash
cp .env.example .env
```

Key settings:

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string |
| `JWT_SECRET` | JWT signing secret |
| `LLM_API_KEY` | API key for LLM provider (not needed for `local` provider) |
| `LLM_PROVIDER` | `anthropic`, `openai`, or `local` |
| `DAEMON_ID` | Unique identifier for this daemon instance |
| `DAEMON_SERVER_URL` | Server URL for daemon registration |
| `CLAUDECODE_BIN` | Custom path to Claude Code binary (for `local` provider) |

For production, use `.env.production` as a template and generate secrets with `./scripts/gen-secret.sh`.
