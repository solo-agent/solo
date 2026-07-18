# Project testing rules

- Before implementation, produce a complete architecture design covering the domain model, ownership and lifecycle, data flow, persistence, APIs, frontend state, failure recovery, migration, and compatibility with existing Solo behavior.
- Review every change from the whole-project perspective. Trace the existing frontend, server, daemon, local agent runtime, session, and database paths before changing one feature path.
- All new product-flow validation must use real components and dependencies; do not use mocks.
- End-to-end tests must use the real frontend, API server, and PostgreSQL database.
- Do not mock HTTP routes, backend services, or database behavior.
- Validation must cover frontend behavior and the complete backend path, including the real local agent runtime when the feature invokes agents; UI-only tests are not sufficient.
- Report E2E success only after verifying the user-visible result and persisted database state.

# Project service lifecycle rules

- An agent may start or restart the Solo frontend, API server, and daemon when requested, but only by running `make rebuild` from the repository root. Tests must reuse that make-managed stack.
- Never create `launchctl` jobs or manually start these services with direct binaries, `go run`, `npm run dev`, `nohup`, or custom background commands.
- When the user asks to stop the project, use `make stop` and verify that ports 3000, 8080, and 8081 are no longer listening. Do not restart services unless the user explicitly asks.
