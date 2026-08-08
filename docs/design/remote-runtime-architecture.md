# Solo Remote Runtime V1 Architecture

> Status: implemented; verification results are recorded in section 16  
> Date: 2026-08-06  
> Baseline: `7a63a79`  
> Scope: one remote Solo deployment, one active Daemon connection per Computer, many Computers per deployment

## 1. Goal

Solo's Frontend, API Server, PostgreSQL, attachments, and artifacts run on a remote server. Each user's Daemon, provider CLIs, persistent sessions, workspace, transcript, skills, and provider secrets remain on that user's computer.

The complete path must work without exposing a local Daemon port and without giving a Daemon database credentials or the Server JWT signing secret.

```text
Browser --HTTPS/WSS--> Remote Frontend/API --SQL--> PostgreSQL
                              ^
                              | WSS control + HTTPS pull/callback
                              |
                    Local Daemon --stdio/ACP--> Agent Runtime
                              |
                     Workspace / Transcript / Secrets
```

V1 deliberately does not add Redis, Kafka, a generic message bus, multi-Server coordination, automatic cross-Computer failover, cloud provider-secret storage, or a second durable queue beside PostgreSQL.

## 2. Existing behavior that must remain true

- Channel, DM, Thread, Task, router, M3 freshness, M4 idle sleep, M5 Agent/Computer affinity, M6 WebSocket recovery, M8 behavior limits, and M9 runtime metadata retain their product semantics.
- A Thread uses its own conversation context. A parent Channel supplies eligible Agent identities only.
- One Agent executes at most one Run at a time on its bound Computer.
- Agent messages use the existing message reliability and send-freshness contracts.
- Persistent provider sessions remain local and resumable.
- A human message is stored even while the target Computer is offline.
- Existing `computer_members` access remains valid. The owner manages pairing and credentials; existing members may bind and use their own Agents on that Computer.

## 3. Current gaps

The current local architecture cannot be exposed safely or work through NAT:

- Server calls `http://daemon-host:port` and consumes Daemon SSE.
- Daemon connects directly to PostgreSQL.
- all Daemons share `INTERNAL_TOKEN_SECRET`, with a permissive empty-secret development fallback;
- Daemon can mint long-lived Agent JWTs because it has the Server JWT secret;
- Server exposes attachments without authentication;
- ordinary Agent creation silently chooses the first online Computer;
- runtime detection is global rather than Computer-scoped;
- pending task ownership and live event replay rely partly on Server/Daemon memory.

Remote V1 removes these assumptions instead of tunnelling them through a public Daemon endpoint.

## 4. Trust boundaries and invariants

1. The Browser trusts only the HTTPS origin and authenticates with the existing user access/refresh tokens.
2. A Computer authenticates with one random, revocable machine credential. Server stores only its SHA-256 hash.
3. Enrollment tokens are random, one-time, hashed, short-lived, and scoped to one Computer.
4. A local Daemon opens outbound connections only. Its HTTP listener remains loopback-only for the local `solo` CLI.
5. Daemon never receives `DATABASE_URL`, `JWT_SECRET`, or another Computer's credential.
6. Provider keys, workspace files, transcripts, memory, and installed skills remain local unless a user explicitly requests a bounded read through an authorized API.
7. PostgreSQL `agent_runs` is the durable delivery queue and source of Run truth. WSS is only a wake/control channel.
8. A Run starts only after the current Daemon accepts it. Server creates one `execution_attempt_id` while holding the Run row lock; duplicate accepts return that same attempt.
9. All events and terminal callbacks are validated against Run, Agent, Computer, and execution attempt.
10. One Computer has at most one current ready control connection. A new ready connection replaces the old connection.
11. The replaced connection cannot accept new Runs or publish state. The stale Daemon shuts down; reconciliation clears attempts absent from the replacement Daemon and redispatches them from PostgreSQL.
12. Revoking a Computer credential fails closed: disconnect it and reject later control, Run, event, and RPC calls. Re-pairing plus reconciliation retries accepted work that the replacement Daemon no longer reports.

## 5. Domain model and persistence

### 5.1 Computer

Reuse `computers` and `computer_members`. Add these columns to `computers`:

| Column | Meaning |
|---|---|
| `credential_hash` | Current machine credential hash; raw value is never stored |
| `credential_created_at` | Last successful enrollment/rotation |
| `credential_revoked_at` | Explicit revocation time |
| `enrollment_token_hash` | Pending one-time token hash |
| `enrollment_expires_at` | Enrollment expiry; default 10 minutes |
| `enrollment_used_at` | Audit time for the last exchange |
| `protocol_version` | Last ready control protocol version |
| `daemon_version` | Last ready Daemon version |
| `runtime_inventory` | Last known bounded backend detection result as JSONB |
| `last_connected_at` | Last successful ready handshake |

`status` remains `online` or `offline`. Pairing state is derived from enrollment and credential columns and returned separately as `pairing_status` (`pending`, `paired`, `revoked`, `unpaired`). `daemon_url` is no longer a routing input and is retained only during migration.

New Computers are created by an authenticated user, who becomes owner and receives the raw enrollment token once. The open-ended “claim any online Computer” flow is retained only for legacy unowned rows; new paired Computers are not globally discoverable.

Computer deletion is rejected while active Agents or non-terminal Runs are bound to it. Credential revocation is always allowed and immediately stops new work.

### 5.2 Agent binding

V1 continues using the existing `agents.runtime_id` value as the persisted Computer UUID to avoid a parallel binding model. Public APIs call it `computer_id`.

- creating a normal Agent requires `computer_id`;
- Lucy, template provisioning, and team formation carry the selected/inherited `computer_id` explicitly;
- Server verifies that the caller is a Computer member;
- changing `computer_id` is allowed only while the Agent has no non-terminal Run;
- migration keeps messages, Tasks, Runs, and Server session audit, but closes the old local provider Session and rebuilds local execution context on the new Computer;
- a legacy unbound Agent may be backfilled only when the user has exactly one usable Computer. Otherwise the API returns `computer_selection_required`.

### 5.3 Durable Run delivery

Add to `agent_runs`:

| Column | Meaning |
|---|---|
| `computer_id` | Immutable Computer target captured when the Run is created |
| `dispatch_payload` | Server-built runtime input required to execute after a restart/offline period |
| `delivery_expires_at` | Queue expiry; default 24 hours |
| `execution_attempt_id` | Current accepted local execution attempt |
| `accepted_at` | Conditional accept time |
| `retry_of_run_id` | Previous failed Run when recovery creates a replacement |
| `delivery_count` | Number of successful accepts, for audit only |

`dispatch_payload` includes prompt/context, Agent name/config, Channel name, peer workspace descriptors, messages, attachment metadata, model selection, and resume identifiers. It never contains provider credentials or machine credentials.

Daemon transport events are stored in `agent_run_delivery_events`, keyed by `(run_id, attempt_id, source_seq)`. They are replayable without duplicate lifecycle effects and remain distinct from the existing Server-generated semantic `agent_run_events` timeline.

No `claimed` status and no independent Run lease are introduced. Existing statuses remain. `queued` means durable and not accepted; `running` begins at successful accept. Provider start remains visible through `backend_started_at`.

## 6. Ownership and lifecycle

### 6.1 Computer lifecycle

```text
created/unpaired -> enrollment pending -> paired/offline -> online
                          |                    |          |
                          +-- expired ---------+          +-- disconnect -> offline
                                               +-- revoke -> revoked/offline
```

- Owner creates, renames, pairs, rotates, revokes, and deletes.
- Members can view status, select the Computer for their own Agents, and use its runtime inventory.
- The raw enrollment token and raw machine credential are returned exactly once.
- Daemon stores `{server_url, computer_id, credential}` in a `0600` local file. Environment variables may override it for ephemeral/container use.
- Re-pairing replaces the old credential and closes its current connection.

### 6.2 Control connection lifecycle

1. Daemon authenticates the WSS upgrade using `X-Solo-Computer-ID`, `X-Solo-Protocol-Version`, and `Authorization: Computer <credential>`.
2. Daemon sends `hello` with Daemon version, runtime inventory, system info, cached Agent IDs, and active `{run_id, execution_attempt_id}` pairs.
3. Server validates compatibility and returns `ready` with connection ID, heartbeat interval, and Server time.
4. Only after `ready` does the connection become current. It atomically replaces and closes the previous current connection.
5. Heartbeats update in-memory liveness and persisted Computer status/last-seen metadata.
6. Normal network loss keeps a short in-memory grace period so HTTPS callbacks/polling can finish. A replacement ready connection revokes that grace immediately.
7. Server restart loses connection leases by design; Daemon reconnects and receives a new one. PostgreSQL Run/attempt state survives.

### 6.3 Run lifecycle

```text
human/agent event -> Router -> queued Run in PostgreSQL
                               |
                     WSS run.available (hint)
                               |
                  Daemon has local Agent slot
                               |
                       HTTPS accept(run)
                               |
             PostgreSQL queued -> running (CAS)
                               |
                    local Backend / Session
                               |
                HTTPS events + terminal callback
                               |
                     terminal Run in PostgreSQL
```

Accept succeeds only when:

- credential belongs to `run.computer_id`;
- connection is current or inside its allowed disconnect grace;
- Run is queued and not expired;
- the Run has no attempt yet, or the accept is an idempotent retry for the attempt Server already created.

Retrying accept is idempotently successful. PostgreSQL does not duplicate the already-existing per-Agent execution queue: the Daemon accepts durable Runs and its shared Agent turn gate executes them one at a time.

The Server creates a queued Run even when the Computer is offline. The UI shows “waiting for computer” rather than a generic failure. A sweeper expires unaccepted Runs after 24 hours and records a retryable delivery error.

## 7. Wire protocol and APIs

### 7.1 Control envelopes

All control frames use JSON:

```json
{
  "type": "run.available",
  "request_id": "uuid",
  "protocol_version": 1,
  "payload": { "run_id": "uuid" }
}
```

Frame size is capped at 1 MiB. There is one reader and one writer per socket, bounded send queues, write deadlines, ping/pong, inbound watchdog, exponential reconnect backoff with jitter, and system HTTP proxy support.

Control types in V1:

- `hello`, `ready`, `heartbeat`, `heartbeat.ack`;
- `run.available`;
- `rpc.request`, `rpc.response`, `rpc.cancel`;
- `connection.replaced`, `error`.

WSS does not carry full Run payloads, file contents, streamed model output, or terminal truth.

### 7.2 Machine HTTP endpoints

Unauthenticated but rate-limited:

- `POST /internal/v1/daemon/enroll` exchanges one enrollment token for one machine credential.

Machine-credential authenticated:

- `GET /internal/v1/daemon/connect` upgrades to WSS;
- `GET /internal/v1/daemon/runs/pending` is the reconnect/polling fallback;
- `POST /internal/v1/daemon/runs/{runID}/accept` accepts the Run and returns the Server-created attempt, execution payload, and per-Run Agent token;
- `POST /internal/v1/daemon/runs/{runID}/events` ingests idempotent sequenced events;

Every authenticated call validates Computer ID and credential in constant time, limits body size, and logs Computer/run/request IDs without secrets.

### 7.3 User APIs

- `POST /api/v1/computers` returns Computer plus one-time enrollment token.
- `POST /api/v1/computers/{id}/enrollment` rotates/generates an enrollment token.
- `POST /api/v1/computers/{id}/credential/revoke` revokes the current credential.
- `GET /api/v1/computers` returns all accessible Computers, including offline/unpaired rows.
- `GET /api/v1/computers/{id}/backends/detect` performs a live Computer-scoped RPC.
- Agent create/update responses include `computer_id`, Computer status, and runtime availability.
- `PATCH /api/v1/agents/{id}` accepts `computer_id` for idle migration.

Existing message/Task APIs remain the Agent-facing product protocol.

## 8. Daemon resource RPC

Server-initiated local reads/actions use request/response RPC over the current control socket:

| Method | Authorization before RPC | Limit |
|---|---|---|
| `backend.detect` | Computer member | 900 KiB, 10 s |
| `workspace.list` | Agent owner/member access + Agent bound to Computer | 900 KiB, 10 s |
| `workspace.read` | Same, existing traversal/symlink checks | 900 KiB, 10 s |
| `skills.list` | Same | 512 KiB, 10 s |
| `transcript.read` | authenticated Run/session lookup + bound Computer | 900 KiB, 10 s |
| `agent.cleanup` | Agent owner + bound Computer | 30 s |
| `thinking.cleanup` | owning Channel operation | 30 s |
| `run.cancel` | authorized Run cancellation | 10 s |

The Server keeps an in-memory `request_id -> response channel` map only while a request is active. It is not a durable queue. Disconnect or timeout returns a structured unavailable error.

## 9. Agent authentication and local proxy

Daemon no longer generates or stores Server JWTs.

On successful Run accept, Server signs a short-lived token containing:

- actor type `agent_run`;
- Agent subject;
- Run ID;
- Computer ID;
- expiry after 24 hours.

Authenticated Server middleware checks that the referenced Run is still non-terminal and still belongs to the Agent and Computer. This makes terminal completion an immediate logical revocation without a token blacklist.

`delivery_expires_at` applies only before a Daemon accepts a queued Run. Once accepted, the active-Run database check is the authority, so a legitimate long-running Run is not cut off by its former queue deadline.

The persistent provider process calls the loopback Daemon proxy. The Daemon selects the token from the currently executing Run, never from Agent-supplied routing fields. The `solo` CLI must not fall back to a direct remote request when `SOLO_DAEMON_URL` is configured; this prevents an old persistent-session environment token from escaping the local proxy boundary.

Existing legacy Agent token files are ignored after upgrade. No new long-lived Agent token is written to disk.

## 10. Events, visible results, and recovery

Daemon converts Backend chunks into the existing semantic event names and POSTs them with monotonically increasing `source_seq` for one attempt. Server owns:

- updating `agent_runs` and `agent_sessions`;
- appending `agent_run_events`;
- M3 freshness/visible-message checks;
- result-reminder retry;
- WebSocket broadcasts to browsers;
- Task linkage and final status.

While the Daemon process remains alive, every task keeps replayable sequenced events in its bounded local task history and retries upload across socket or Server loss. A Daemon process crash intentionally falls back to the persisted Run payload: reconciliation clears the missing attempt and re-executes the same Run with a new attempt. V1 does not add a second disk queue beside PostgreSQL.

Recovery rules:

- WSS loss alone does not fail an accepted Run; HTTPS callbacks continue and polling restores wakeups.
- Server restart: Daemon reconnects with active attempts. Matching attempts remain running; queued Runs are re-announced.
- Daemon process restart: machine lock guarantees the previous local process has stopped. Missing accepted attempts are cleared and the same persisted Run is delivered again with a new attempt.
- New control connection replacement shuts down the stale Daemon. Attempts not reported by the replacement are retried from their persisted dispatch payload with a new attempt.
- First terminal callback wins. Replays return the stored terminal result.
- Run cancellation reuses the existing task cancellation path through reverse RPC when the Computer is online; an unavailable Computer remains explicit rather than pretending that local execution stopped.

## 11. Attachments, artifacts, and local files

- Attachments and artifacts are Server-owned persistent files backed by mounted volumes.
- Attachment download/thumbnail routes require a user JWT or valid Agent-Run token; UUID knowledge alone is insufficient.
- Frontend loads protected media through authenticated fetch and object URLs.
- Daemon materializes message attachments using its per-Run Agent token and keeps the existing size, path, and MIME checks.
- Workspace, transcript, memory, and skills are never copied to PostgreSQL as part of remote enablement.
- Transcript paths stored in PostgreSQL are audit references only; content reads must be proxied to the bound online Computer. Server must not attempt `os.Stat` on a local-computer path.

## 12. Frontend state and UX

Computer pages show these independent facts:

- pairing state;
- online/offline state and last seen;
- Daemon/protocol version compatibility;
- last runtime inventory and live detection errors;
- bound Agents and queued/running counts.

“Add Computer” creates the row first, then shows the one-time enrollment token and copyable startup command. The page supports retry enrollment and credential revocation.

Every Agent creation path selects Computer before runtime. Runtime options come from that Computer. Agent create/update APIs expose the binding and permit idle migration. Offline is selectable when a cached runtime inventory exists; messages then create queued Runs and show “waiting for computer”.

Computer lists no longer filter out offline rows. Normal routing remains UI-silent; queue/offline/expired/revoked failures are visible.

## 13. Configuration and deployment

### Remote Server

Required production configuration:

- `APP_ENV=production`;
- strong `JWT_SECRET`;
- `DATABASE_URL` with TLS/private networking as appropriate;
- explicit `CORS_ALLOWED_ORIGINS`;
- persistent `ATTACHMENTS_DIR` and `ARTIFACTS_DIR` volumes.

Production startup fails on default/empty secrets or wildcard credentialed CORS. Metrics are not exposed by the public reverse proxy.

The repository provides a single-machine remote Compose stack containing PostgreSQL, migration, API Server, Frontend, and TLS reverse proxy. PostgreSQL, attachment, artifact, and proxy state use named volumes. Daemon is intentionally not part of the remote stack.

### Local Daemon

Daemon needs only Server URL plus either an enrollment token for first use or its stored machine credential. It does not need database or Server JWT configuration. Provider secrets remain in its environment/local provider configuration.

Paired local development uses the same reverse protocol through `make rebuild`. For migration compatibility only, an unpaired localhost Daemon may still use the legacy loopback transport; production Server routing never exposes those legacy endpoints.

## 14. Migration and compatibility

1. Add nullable columns and indexes without changing existing Run statuses.
2. Backfill `agent_runs.computer_id` from the Agent binding and existing `daemon_id` relationship.
3. Existing Computer rows begin `unpaired`; an owner generates an enrollment token before the upgraded Daemon connects.
4. Existing `agents.runtime_id` UUID bindings remain valid.
5. Existing queued/running rows without a dispatch payload are not redispatched; they converge using current watchdog behavior and are labelled legacy in logs.
6. Remove Server-to-Daemon HTTP/SSE dispatch only after the reverse client, accept endpoints, and recovery tests pass.
7. Remove Daemon PostgreSQL initialization. Legacy internal shared-secret routes remain localhost-development compatibility only and are not registered when `APP_ENV=production`.
8. Local service scripts continue to use `make rebuild`; `.env.example` and setup docs change to the new pairing inputs.

No destructive data rewrite or automatic cross-Computer migration is performed.

## 15. Observability

Structured logs include `request_id`, `computer_id`, `connection_id`, `run_id`, `execution_attempt_id`, `agent_id`, event sequence, and failure code where applicable. Credentials, enrollment tokens, prompts, file contents, and provider secrets are never logged.

The existing Prometheus endpoint adds bounded counters/gauges for current Computer controls, successful handshakes, queued/accepted remote Runs, duplicate event replays, and reverse-RPC timeouts. It remains outside the public Caddy route. V1 does not add a Dashboard or a label-heavy metrics subsystem.

## 16. Real-component verification

Product-flow E2E uses the real Frontend, API Server, PostgreSQL, Daemon, and local Agent Runtime. HTTP routes, services, and database behavior are not mocked. Lower-level trust-boundary and replay cases use Go integration tests against PostgreSQL where a browser adds no extra product evidence.

| Verification layer | Proven behavior |
|---|---|
| `test-e2e-remote-server` | one-time Computer enrollment, outbound authenticated control, Computer-scoped runtime detection, normal Agent binding, online visible result, transcript/session/workspace/skills reverse RPC, authenticated attachment download and local materialization, credential revoke, offline durable Run, re-pair, recovered visible result, browser rendering, and persisted Computer/Run/attempt/event/message/session truth |
| Tencent Cloud acceptance (2026-08-08) | the private remote Compose stack, SSH-tunnel browser access, a local paired Daemon, real Claude Code and Codex results in one Channel, Server-restart reconnection, offline Run persistence, single redelivery after Daemon recovery, and remote PostgreSQL truth |
| `test-e2e-agent-delivery` + `test-e2e-agent-idle-resume` | visible-result contract, router scope, interrupted Run convergence, provider Session resume, idle sleep/resume, Task reassignment, and retry exhaustion |
| `test-e2e-m8`, `test-e2e-m9`, `test-e2e-websocket-recovery` | Channel/Thread/DM scope, busy-Agent queueing, result rescue, Agent cascade limits, delegation, runtime metadata, and foreground/network message recovery |
| Go PostgreSQL integration tests | one-time/expired enrollment, credential revocation, replacement-connection fencing, durable payload replay, idempotent accept/event handling, wrong task/attempt rejection, per-Run token scope, attachment channel scope, Agent/Computer affinity, and template/team binding propagation |
| Production build checks | strict production configuration, explicit credentialed CORS, Frontend production compilation, Compose rendering, and Server/Frontend/migration image builds |

The remote E2E passes only after both the rendered browser result and PostgreSQL state agree; a completed model process without a Run-linked visible Message is not accepted as success.
