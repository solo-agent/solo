# Trae CLI Backend Design

## Goal

Add Trae CLI as a first-class Solo agent backend. A user with `traex` installed
and authenticated can select **Trae CLI** when creating or editing an agent and
receive the same persistent, streaming, tool-capable experience as Solo's
existing ACP backends.

The integration uses Trae CLI's native ACP stdio server:

```text
traex acp serve
```

It does not parse terminal UI output and does not use a new process for every
turn.

## Scope

The feature includes:

- backend registration and local CLI detection;
- one-shot and persistent execution;
- multi-turn session continuity and persisted provider session IDs;
- streaming text, thinking, tool activity, tool results, and usage;
- session-level automatic approval, matching Solo's current ACP behavior;
- model selection, custom environment, and safe custom arguments;
- readable startup, authentication, protocol, and recovery failures;
- frontend runtime selection through the existing metadata-driven UI;
- English and Chinese setup documentation;
- real full-stack validation with Trae CLI and PostgreSQL.

It does not add a Trae-specific settings page, store Trae credentials in Solo,
change the database schema, or introduce a generic user-configurable ACP
backend.

## Alternatives Considered

### Dedicated Trae backend over shared ACP transport — selected

Add `TraeBackend` and reuse Solo's ACP JSON-RPC transport and event conversion.
Trae-specific command construction, argument filtering, error handling, and
session behavior remain isolated in the adapter. This follows the existing
provider boundary while retaining mature shared protocol handling.

### Generic configurable ACP backend

A generic command-and-arguments adapter appears smaller initially, but existing
ACP CLIs differ in invocation, model switching, stderr errors, session resume,
and tool naming. A generic adapter would move provider-specific branching into
shared infrastructure and weaken its interface.

### `traex exec` JSON event mode

Launching `traex exec` for each turn could support basic tasks, but it would
give weaker interruption, latency, streaming, process ownership, and recovery
semantics than Trae's long-running ACP server. It does not satisfy the requested
complete integration.

## Domain Model

`trae` is a new agent provider value:

- registry type: `trae`;
- display name: `Trae CLI`;
- required binary: `traex`;
- binary override: `TRAEX_BIN`;
- protocol family: `acp`;
- process command: `traex acp serve`.

An existing Solo `Agent` owns configuration such as provider, model, custom
environment, and custom arguments. Existing channel-scoped and Thinking-node
session keys own runtime isolation. An existing `agent_sessions` row owns the
persisted Trae provider session ID and lifecycle status. No new entity is
introduced.

Trae authentication remains owned by Trae CLI under the user's home directory.
Solo neither reads nor stores credentials directly.

## Architecture and Ownership

### Registry and factory

The built-in registry adds Trae metadata and a factory. The factory resolves
the executable from `BackendConfig.ExecPath`, then `TRAEX_BIN`, then `traex`.
`NewBackend("trae", ...)` and `NewPersistentBackend("trae")` both produce a
Trae backend. Supported-provider error text and tests include `trae`.

### Trae adapter

A dedicated adapter owns:

- the fixed `acp serve` invocation;
- Trae-specific blocked arguments;
- process startup and stderr diagnostics;
- ACP initialization, new/resumed sessions, and model selection;
- one-shot and persistent lifecycle methods;
- provider-specific error wording.

Protocol framing, permission replies, event conversion, usage parsing, and
turn serialization remain in shared ACP code.

### Daemon and session managers

Daemon startup enumerates registered persistent backends, so registering Trae
causes a Trae `SessionManager` to be created without a parallel lifecycle
system. Existing keys preserve isolation among channels, DMs, and Thinking
nodes. Stop, idle sleep, force close, and process-exit paths use the current
session manager contracts.

### Server and API

The existing backend metadata and detection endpoints expose Trae. Agent CRUD
already accepts string provider identifiers and needs no Trae-specific route.
Run requests continue carrying `model_config.provider = "trae"`.

### Frontend state

The agent form consumes backend registry metadata and detection status. Trae
therefore appears as a runtime option with its installed/unavailable state
without a hard-coded form branch. Existing agent hooks persist `"trae"` through
normal create and update requests.

Activity rendering treats Trae as an ACP-family backend so tool names and
status text follow the same normalization as other ACP providers.

## Data Flow

1. The user selects **Trae CLI** in the agent form.
2. The server persists `agents.model_provider = "trae"` with the selected model,
   `custom_env`, and `custom_args`.
3. On the first run for a session key, the daemon creates a Trae backend and
   starts `traex acp serve` in the agent workspace.
4. The ACP client sends `initialize`, followed by `session/resume` when a
   persisted provider session ID is available, or `session/new` otherwise.
5. If a non-empty model is configured, the client sends `session/set_model`.
   An unsupported model is a visible failure; it is not silently replaced.
6. Solo composes system instructions, memory, channel context, attachments, and
   the user turn into ACP prompt blocks and sends `session/prompt`.
7. ACP notifications become Solo output chunks for text, thinking, tool calls,
   tool results, status, and usage. Existing WebSocket/SSE and observability
   paths publish them to the frontend.
8. The provider session ID is stored in `agent_sessions.external_session_id`.
   Subsequent turns reuse the live process and session.

## Permissions and Argument Safety

ACP `session/request_permission` receives Solo's existing
`approve_for_session` response. The adapter does not add
`--dangerously-bypass-approvals-and-sandbox`.

The adapter owns the protocol command and blocks custom arguments that could
replace, duplicate, or disrupt `acp serve`. Other supported Trae flags remain
available through `custom_args`. Agent `custom_env` values merge through the
existing environment builder and are never logged by value.

## Persistence and Migration

No database migration is required:

- `agents.model_provider` is a string field that can store `trae`;
- `agents.model_name`, `custom_env`, and `custom_args` already cover runtime
  configuration;
- `agent_sessions.provider` and `external_session_id` already cover provider
  lifecycle persistence;
- existing run and transcript metadata remain applicable.

Old rows and every existing provider retain their current behavior.

## Failure Recovery

- **Binary missing:** detection marks Trae unavailable; direct execution reports
  that `traex` was not found and mentions `TRAEX_BIN`.
- **Not authenticated:** startup/provider stderr is converted to a concise
  instruction to run `traex login --sso`; credentials and raw tokens are not
  exposed.
- **ACP handshake failure:** the task fails, the child process is closed, and no
  live session is recorded.
- **Invalid model:** the turn fails with the requested model name and Trae's
  diagnostic. Solo does not fall back silently.
- **Process exit during a turn:** the active run is marked failed and all
  waiters are closed exactly once. The next turn may start a new process.
- **Idle sleep or daemon restart:** the persisted provider session ID is offered
  to `session/resume`.
- **Resume rejected or session missing:** the old runtime is marked ended and a
  new provider session is created. The run continues with Solo's persisted
  conversation context rather than remaining stuck.
- **Timeout or cancellation:** Solo interrupts the active turn, closes stdin,
  and force-kills the process if graceful shutdown does not complete.
- **Late ACP events:** the existing per-turn controller ignores events after a
  terminal result, preventing sends to closed channels.

stderr is retained for backend diagnostics but never appended to the agent's
answer.

## Compatibility

The feature is additive. It does not change API shapes, database constraints,
provider defaults, session keys, or semantics for existing agents. Trae uses
the established ACP family behavior for event and activity normalization.

An unavailable Trae binary is represented like any other unavailable registered
CLI. An installed but logged-out CLI is detectable as installed; authentication
failure is reported when a run starts because detection must not inspect or
export credentials.

English and Chinese READMEs document the `traex` binary, ACP protocol, install
command, and `traex login --sso` prerequisite.

## Validation

### Automated Go coverage

Tests cover:

- registry metadata, factory creation, persistent-backend creation, and
  `TRAEX_BIN`;
- fixed arguments and filtering of protocol-conflicting custom arguments;
- missing binary and authentication/provider error promotion;
- ACP initialize, session creation, model selection, prompt streaming, usage,
  and session ID propagation;
- two sequential persistent turns;
- graceful close, force close, process exit, late events, and resume fallback;
- ACP-family activity and tool-name normalization.

The shared persistent turn contract test includes Trae.

### Real full-stack E2E

Validation uses:

- the real frontend and API server;
- the real daemon and make-managed service lifecycle;
- real PostgreSQL and migrations;
- the installed, authenticated `traex` binary;
- no mocked HTTP route, backend, database, or agent process.

The stack is rebuilt only with `make rebuild`. The test creates a Trae agent
from the UI, asks it to create a uniquely named file in its workspace, observes
streaming output and tool activity, then sends a follow-up that depends on the
first turn.

Success requires all of:

- the UI shows the Trae runtime and completed replies;
- the requested workspace file contains the expected content;
- the second turn demonstrates context continuity;
- `agents.model_provider` is `trae`;
- `agent_sessions` contains a Trae provider session ID and valid lifecycle
  status;
- run records reach their terminal completed state;
- stop and subsequent resume/restart behavior is observed through the real
  provider path.

If authentication, service availability, account quota, or Trae server behavior
blocks the real run, the blocker is reported explicitly and partial checks are
not described as full E2E success.

## Documentation and Operational Notes

The supported-backend tables list Trae CLI with command `traex` and protocol
ACP. Setup instructions include:

```bash
curl -fsSL https://code.byted.org/api/tos-proxy/download/traex_install.sh | sh
traex login --sso
```

Operators may override the binary with `TRAEX_BIN`. Solo service lifecycle
continues to use `make rebuild`, `make start`, and `make stop`.
