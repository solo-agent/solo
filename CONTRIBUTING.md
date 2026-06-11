# Contributing to Solo

Thanks for your interest in Solo. This document covers the day-to-day mechanics
of contributing. For the project's behavior expectations, see
[CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

## Quick start

Solo is a Go + Next.js project with a local PostgreSQL. The Makefile drives
almost everything.

```bash
# 1. Clone
git clone git@github.com:fredalxin/solo.git
cd solo

# 2. Bootstrap and start everything in one shot
make dev
```

Open <http://localhost:3000> to register an account and try it out. `make dev`
is idempotent — re-run it any time to get back to a clean dev environment.

Daily commands (run `make help` for the full menu):

| Command        | What it does                                          |
| -------------- | ----------------------------------------------------- |
| `make dev`     | Bootstrap from scratch (env, deps, DB, migrations, services) |
| `make start`   | Start all services (auto-builds if binaries missing)  |
| `make restart` | Restart all services                                  |
| `make rebuild` | Rebuild binaries and restart                          |
| `make stop`    | Shut everything down                                  |
| `make migrate` | Apply pending database migrations                     |

## Project layout

See the [Project Structure](./README.md#project-structure) section in README
for the full tree. The short version:

- `cmd/server` — Go API server (Chi router, `:8080`)
- `cmd/daemon` — Agent daemon (per-machine, `:8081`)
- `cmd/solo` — Agent CLI tool
- `internal/` — HTTP handlers, services, WebSocket hub, DB layer, auth
- `pkg/agent/` — Agent runtime, sessions, memory, prompt
- `pkg/llm/` — LLM provider abstraction
- `frontend/` — Next.js 16 app (`:3000`)
- `migrations/` — PostgreSQL migrations (applied automatically by `make`)
- `e2e/` — Playwright end-to-end tests
- `docs/` — Working design docs (not part of the public artifact)

## Running tests

The CI pipeline (`.github/workflows/ci.yml`) runs three checks locally too:

```bash
# Lint (Go + frontend)
golangci-lint run --timeout=5m ./...
(cd frontend && npm run lint)

# Tests (Go requires PostgreSQL; frontend just needs node_modules)
go test ./... -v -count=1
(cd frontend && npx tsc --noEmit)
```

For a real end-to-end run, the dev environment from `make start` is enough —
register an account, create a channel, add an agent backend (see
[Configuration](./README.md#configuration)), and start a conversation.

## Commit style

Solo uses a lightweight conventional-commits style. Look at `git log` for the
shape; the most common types are:

- `fix:` — bug fix
- `feat:` — new user-facing feature
- `refactor:` — internal change with no behavior difference
- `polish:` — visual / UX tweak with no behavior change
- `cleanup:` — dead code, dependency, or naming removal
- `simplify:` — drop complexity without changing behavior
- `docs:` — documentation only

A good subject line is short (under ~70 chars) and describes the *what*, not
the *why*. Put rationale in the commit body if it's not obvious.

## Pull requests

PRs use the template at
[.github/PULL_REQUEST_TEMPLATE.md](./.github/PULL_REQUEST_TEMPLATE.md). It
covers the type checklist, the design-system checks (if you touched UI), and
E2E expectations. Fill it in even for small PRs — it helps reviewers skim.

Before opening a PR:

1. `make start` and exercise the changed area in the browser.
2. Run the lint and test commands above.
3. If you changed anything under `frontend/app/`, the design-system checklist in
   the PR template applies.
4. Keep the diff focused. One concern per PR. Don't mix a refactor with a
   feature.

## Reporting bugs

Open a GitHub issue. Include:

- What you did (steps to reproduce)
- What you expected
- What actually happened
- Logs from `server.log` / `daemon.log` if relevant
- Your environment: OS, Go version, Node version, Docker version

## Security

Please **do not** file public issues for suspected vulnerabilities. See
[SECURITY.md](./SECURITY.md) for the disclosure process.

## Questions

If you're stuck or want to discuss an approach before writing code, open a
GitHub issue with the `question` label. For quick clarifications, the issue
tracker is fine — there's no Discord/Slack to gatekeep on.
