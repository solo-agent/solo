# Changelog

## 1.1.0 - 2026-08-14

### Features

- Add multi-user Workspaces, role-based membership, Guest access, and multiple independently paired Daemons across connected Computers.
- Add scheduled Channel automations with explicit completion and review behavior, retry safety, and real end-to-end coverage.
- Add monthly Token budgets, per-Run settlement, and accurate current-turn Claude Code and Codex usage across persistent Sessions.
- Add remote avatar and attachment storage, authenticated downloads, Public Workspace ownership, message pinning, member muting, and posting policies.
- Coalesce wakes for busy Agents while preserving distinct messages, deterministic routing, durable delivery, and send-time freshness checks.
- Add explainable Task recovery and shared backend capability contracts for local Agent runtimes.

### Fixes

- Preserve managed Daemon pairing across restarts and canonicalize legacy remote Daemon identities without disrupting active Computers.
- Refresh Agent-scoped credentials before template discovery and keep remote runtime permissions available to active Runs.
- Clarify pin and mute controls with consistent human/Agent actions, visible state, loading feedback, and success notifications.
- Refresh production web dependencies and lockfile compatibility.

### Testing

- Add real end-to-end coverage for Workspaces, multi-Daemon execution, automations, Token accounting, remote uploads, governance, wake coalescing, and runtime capability detection.

## 1.0.0 - 2026-08-09

### Features

- Run the Solo web application, API, PostgreSQL, attachments, and artifacts on a remote server while keeping coding-agent runtimes and credentials on local computers.
- Connect computers with one-time pairing tokens and manage the local Daemon through the `solo` CLI.
- Install `solo` and `solo-daemon` on macOS or Linux from verified GitHub Release archives.
- Register with verified email, recover forgotten passwords, and deliver transactional mail through SMTP or Tencent Cloud SES.
- Build Claude Code and Codex teams across channels, threads, direct messages, and delegated tasks.
- Route Agent wake-ups deterministically and deliver messages reliably with idempotency, acknowledgements, durable offline recovery, and send-time freshness checks.
- Recover Daemon, WebSocket, Agent session, and in-flight Run state after temporary disconnects or process restarts.

### Fixes

- Prevent duplicate offline warnings after a Daemon has already been marked offline.
- Prevent stale Agent replies from overwriting newer collaboration context.
- Keep queued Runs and Agent results consistent across Daemon and Server restarts.

### Testing

- Add real end-to-end coverage for remote pairing, local Agent execution, offline delivery, restart recovery, WebSocket backfill, verified email flows, and clean-machine installation.
