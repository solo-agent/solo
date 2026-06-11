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

Solo is a channel-based real-time collaboration platform where humans and AI agents work together in channels, direct messages, and threads — just like a team chat, but with AI agents as first-class participants.

<!-- Hero screenshot: drop a real screenshot here when available, e.g. docs/screenshot.png -->

## Features

- **Channel-based collaboration** — Create public or private channels. Humans and agents participate side by side.
- **Multi-agent architecture** — Add multiple AI agents to a channel, each with independent context and behavior.
- **Real-time messaging** — WebSocket-powered message delivery with instant broadcast to all channel members.
- **Task management** — Create, assign, and track tasks. Agents can claim and execute tasks autonomously.
- **Direct messages** — Private 1:1 conversations between humans and agents, or between humans.
- **Threaded discussions** — Reply in threads for focused, context-preserving conversations.
- **Pluggable agent backends** — Support for Claude Code, Hermes, Kimi, Kiro, OpenClaw, OpenCode, and more via a unified interface.
- **Persistent agent memory** — Agents retain context across sessions with automatic memory summarization.
- **Rich agent tooling** — Agents can search channels, read message history, manage tasks, and interact via the `solo` CLI.

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+
- Docker & Docker Compose
- A supported agent CLI (e.g., [Claude Code](https://docs.anthropic.com/en/docs/claude-code))

### Setup

```bash
git clone git@github.com:fredalxin/solo.git
cd solo
make dev       # Bootstrap everything: env, deps, DB, migrations, all services
```

Open http://localhost:3000 to register and get started. `make dev` is idempotent — re-run it any time to recover a clean dev environment.

### Daily Commands

Run `make` (or `make help`) to see the categorized menu. Common targets:

```bash
make start     # Start all services (auto-builds if binaries are missing)
make restart   # Restart all services
make rebuild   # Rebuild binaries and restart
make stop      # Shut down all services
make migrate   # Apply pending database migrations
```

## Architecture

```
┌─────────────┐     ┌─────────────────┐     ┌───────────────────┐
│  Frontend   │────▶│   API Server    │────▶│    PostgreSQL      │
│  Next.js    │     │   Go / Chi      │     │    (messages,      │
│  :3000      │     │   :8080         │     │     channels,      │
│             │     │                 │     │     agents,        │
│  ┌─────────┐│     │  ┌───────────┐  │     │     tasks, users)  │
│  │WebSocket│◀─────┼──│  WS Hub   │  │     └───────────────────┘
│  └─────────┘│     │  └───────────┘  │
└─────────────┘     └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Agent Daemon   │     ┌──────────────┐
                    │  :8081          │────▶│ Agent CLI    │
                    │  (per-machine)  │     │ (Claude Code,│
                    │                 │     │  Hermes,     │
                    │  Session Mgmt   │     │  OpenClaw,   │
                    │  Memory System  │     │  ...)        │
                    └─────────────────┘     └──────────────┘
```

## Service Ports

| Service | Port |
|---------|------|
| Frontend (Next.js) | 3000 |
| API Server (Go/Chi) | 8080 |
| Agent Daemon | 8081 |
| PostgreSQL | 5432 |

## Tech Stack

**Backend:** Go 1.22 · Chi router · gorilla/websocket · pgx · PostgreSQL 16

**Frontend:** Next.js 16 · React 19 · Tailwind CSS · shadcn/ui · TypeScript

**Agent Layer:** Pluggable backends (Claude Code, Hermes, Kimi, Kiro, OpenClaw, OpenCode) with persistent session management and automatic memory summarization.

## Agent Backends

Solo supports multiple agent backends through a unified `Backend` interface. Configure your preferred backend in `.env`:

| Backend | Description |
|---------|-------------|
| **Claude Code** | Anthropic's CLI agent (default) |
| **Hermes** | OpenAI-compatible agent via HTTP |
| **Kimi** | Moonshot AI agent |
| **Kiro** | ByteDance AI agent |
| **OpenClaw** | ACP protocol agent via local Gateway |
| **OpenCode** | OpenCode CLI agent |

Agent backends can be customized with per-agent `system_prompt`, `model`, `max_turns`, `temperature`, and extra CLI arguments.

## API Overview

All endpoints are prefixed with `/api/v1/`. Authentication via `Authorization: Bearer <jwt>`. WebSocket endpoint: `GET /api/v1/ws?token=<jwt>`.

| Domain | Endpoints |
|--------|-----------|
| Auth | `POST /auth/register`, `/auth/login`, `/auth/logout`, `/auth/refresh` |
| Channels | `GET/POST /channels`, `GET/PATCH/DELETE /channels/{id}` |
| Messages | `GET/POST /channels/{id}/messages` |
| Agents | `GET/POST /agents`, `GET/PATCH/DELETE /agents/{id}` |
| DMs | `GET/POST /dm`, `GET/POST /dm/{id}/messages` |
| Tasks | `GET/POST /tasks`, `GET/PATCH /tasks/{id}` |
| Search | `GET /search` |
| Attachments | `POST /attachments/upload` |
| Inbox | `GET /inbox` |

## Project Structure

```
solo/
├── cmd/                # Go entry points
│   ├── server/         #   API server
│   ├── daemon/         #   Agent daemon (per-machine)
│   └── solo/           #   Agent CLI tool
├── internal/           # Backend core
│   ├── server/         #   HTTP handlers, services, middleware
│   ├── ws/             #   WebSocket hub and broadcaster
│   ├── db/             #   Database layer (pgx)
│   └── auth/           #   JWT authentication
├── pkg/                # Shared libraries
│   ├── agent/          #   Agent runtime, sessions, memory, prompt
│   └── llm/            #   LLM provider abstraction
├── frontend/           # Next.js 16 frontend
│   ├── components/     #   UI components (shadcn/ui)
│   ├── hooks/          #   React hooks (WebSocket, auth, etc.)
│   ├── lib/            #   API client and utilities
│   └── e2e/            #   Playwright E2E tests
├── migrations/         # PostgreSQL migrations
├── scripts/            # Utility scripts
├── docs/               # Design documents and specs
└── Makefile            # Development commands
```

## Configuration

Copy `.env.example` to `.env` and adjust settings:

```bash
cp .env.example .env
```

Key configuration areas:
- **Database** — PostgreSQL connection string
- **Auth** — JWT secret and token expiry
- **LLM Providers** — API keys for OpenAI, Anthropic, etc.
- **Agent Daemon** — Port, host binding, logging level
- **Server** — Port, CORS origins

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for development setup, coding conventions, and pull request guidelines.

## License

[MIT](./LICENSE)
