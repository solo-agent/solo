# Changelog

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
