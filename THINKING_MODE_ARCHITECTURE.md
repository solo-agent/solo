# Solo Thinking Mode Architecture

## 1. Purpose and implementation gate

Thinking mode turns one channel conversation into a tree of isolated conversations without changing the normal channel and thread paths.

This document is the implementation gate for the feature. Code is considered conforming only when its data model, runtime behavior, frontend state, recovery behavior, and tests match the decisions below. Existing uncommitted Thinking code is a draft to audit against this design, not proof that the feature is complete.

## 2. Product invariants

1. A Thinking node is a conversation scope. Raw messages from one node never appear in another node.
2. An Agent and a Session are different concepts:
   - Agent is the existing Solo identity, configuration, local runtime type, workspace, memory, and credentials.
   - Session is one independent conversation owned by that Agent.
3. Root and first-level nodes reuse existing team Agents. Thinking mode never clones or creates Agent records.
4. A second-level or deeper split creates a new node Session and inherits the parent's `agent_id` by default. It does not create a new Agent.
5. Every cross-node transfer uses an Agent-authored Handoff. Mechanical truncation, last-message snapshots, and raw node history are never used as cross-node context.
6. Normal channel messages, threads, tasks, and existing Agent sessions keep their current behavior when Thinking mode is not active.
7. A test is end-to-end only if it passes through the real browser UI, API server, PostgreSQL, daemon, and installed local Agent CLI runtime.
8. Returning a node is terminal for runtime mutation and topology: once `returned_at` is set, it cannot accept new messages, resume its Agent Runtime, or split. The node remains selectable as a read-only historical conversation, and its final handoff remains visible from the graph and parent.
9. Return is a real local-Agent handoff, not a mechanical copy of the last message. The node's existing Agent and provider Session produce one bounded structured handoff before the node is sealed.

## 3. Reference decisions

The useful Stello idea is its separation of topology from conversation runtime:

- root and child are the same kind of Session;
- a fork is an independent Session plus a topology node;
- context inheritance happens once at fork time;
- later cross-branch communication uses Agent-authored Handoffs instead of sharing raw history;
- turns serialize within one Session while different Sessions can be scheduled independently.

Solo keeps these semantics but uses its existing local Agent Runtime, Agent identities, channel membership, daemon, message API, and visual style. Stello is a design reference, not a runtime dependency.

The supplied product screenshot contributes the split-screen interaction: the selected node's conversation stays on the left and the navigable node graph stays on the right. The linked Kitkit space currently returns `Unauthorized`, so no inaccessible behavior is assumed.

## 4. Domain model

### 4.1 ThinkingSpace

- One space per channel.
- Created idempotently when the user enters Thinking mode.
- Owns one rooted node tree.
- Deleted with its channel.

### 4.2 ThinkingNode

A node owns four independent concerns:

| Concern | Field/source | Meaning |
| --- | --- | --- |
| Topology | `space_id`, `parent_id`, `depth`, `sort_order` | Location in the brainstorm tree |
| Agent ownership | `agent_id` | Existing Solo Agent that performs this node's turns |
| Conversation | `messages.thinking_node_id` | Raw node-local message history |
| Runtime continuity | `agent_session_id` | Current persisted local-runtime session, assigned lazily after first turn |
| Handoffs | `inherited_handoff`, `checkpoint_handoff`, `returned_handoff` | Fork context, active sibling awareness, and terminal parent transfer |
| Lifecycle | `fork_handoff_pending`, `returning_at`, `returned_at` | Preparing initial context, active, returning, or terminally returned |

`source` records why topology was created: `root`, `team`, `manual`, or `auto`. It never changes runtime semantics.

### 4.3 Agent binding rules

When the space is created:

1. Load active Agent members of the channel and their `assigns_to` relationships.
2. Pick the team root as the Agent with no incoming assignment and the strongest outgoing assignment role; fall back deterministically to the first channel Agent.
3. Bind the root node to that existing Agent, normally Lead.
4. Create first-level nodes for the root Agent's direct team children, normally PM/RD/FE/QA, and bind each node to the corresponding existing Agent.
5. If relationships are absent, bind the root to the deterministic coordinator and use the remaining channel Agents as first-level nodes.
6. If a channel has no Agent, create an unbound root that supports manual notes/splits but does not dispatch a runtime turn.

For manual or automatic splits at depth two and deeper:

- copy the parent's `agent_id` to the child;
- leave `agent_session_id` empty;
- mark `fork_handoff_pending` and ask the parent's existing local Agent Session for a child-specific `inherited_handoff`;
- keep the child read-only until that Handoff arrives; a failed preparation remains retryable instead of falling back to mechanical context;
- lazily create a distinct runtime Session on the child's first user turn after preparation.

When the root already has `source=team` children, the root cannot create additional manual or automatic children. This keeps the default first layer identical to the configured Agent Team. A root without team children may still be split manually so channels without a team are usable.

Mentioning another Agent inside a node does not change node ownership. The node's bound Agent remains the sole runtime target. Supporting guest Agents would require a separate participant/session model and is outside this feature.

### 4.4 Session identity

The daemon must separate the runtime pool key from Agent identity:

- normal Solo session key: `agent:<agent_id>`;
- Thinking session key: `thinking:<thinking_node_id>`;
- actual runtime Agent ID: always the node's existing `agent_id`.

Therefore one Agent may own several independent node Sessions while continuing to use the same Agent workspace, memory, provider configuration, token, and identity.

The daemon session pool, busy-turn state, pending messages, failed-start cooldown, crash watcher, and resume metadata are keyed by the session key. Presence reporting and Agent deletion remain keyed by/de-duplicated on actual Agent ID.

Because node Sessions of the same coding Agent share one workspace, turns are serialized per actual Agent even though their conversation processes and histories are separate. Different Agents can run in parallel. This preserves Solo's existing workspace safety while allowing the default first-level team to brainstorm concurrently.

### 4.5 Runtime process budget

- Node records and provider Sessions are durable; local CLI processes are disposable execution resources.
- A child node remains process-free until its first real turn. Preparing its fork Handoff reuses the parent process.
- A successfully returned node closes `thinking:<node_id>` immediately and can never wake it again.
- A non-returned Thinking process that is idle for 30 minutes is gracefully put to sleep. A daemon-wide one-minute sweep applies only to `thinking:*`; normal `agent:*` channel/thread/task processes retain their existing lifecycle.
- The production defaults can be tuned operationally with `THINKING_SESSION_IDLE_TTL` and `THINKING_SESSION_SWEEP_INTERVAL` duration values; this also permits real short-TTL process lifecycle verification without changing product semantics.
- Sleeping preserves the latest external provider Session ID in the node/session records. The next node message starts a replacement local process with `--resume`, so raw context is not rebuilt or copied across nodes.
- The sweeper never closes a Session whose turn lock is held or queued. It marks the entry asleep before closing so the normal crash watcher does not restart an intentional sleep.
- Archiving a channel best-effort closes every node process in its Thinking space on every online daemon. Persisted topology, messages, Handoffs, and Session provenance remain because channel deletion is currently a soft archive.

## 5. Persistence design

### 5.1 Tables

`thinking_spaces` stores channel ownership.

`thinking_nodes` stores topology, Agent ownership, the latest three Handoff roles, and nullable `agent_session_id -> agent_sessions.id`.

`messages.thinking_node_id` stores node-local raw conversation. The database check keeps thread and node scopes mutually exclusive.

`agent_sessions` remains the canonical provider-session record (`agent_id`, provider, external session ID, transcript path). It is not duplicated per turn.

`agent_runs.thinking_node_id` records which node caused a real runtime run. Existing channel/thread run columns remain unchanged.

### 5.2 Runtime binding

1. The first node turn starts a local persistent CLI session.
2. The daemon emits the real external session ID through its existing SSE `session` event.
3. The server upserts `agent_sessions`, binds the run, and atomically updates the node's `agent_session_id`.
4. Later node turns resolve the external session ID through that binding and send it to the daemon as the requested resume ID.
5. A newly created child always starts with no binding, ensuring its external session ID differs from the parent and siblings.

The pointer lives on `thinking_nodes` rather than making `agent_sessions` one-to-one with a node. This allows a node to rotate to a replacement provider session after a failed resume while preserving historical Agent run/session records.

### 5.3 Unified Handoff semantics

- `inherited_handoff`: immutable, child-specific fork context authored by the parent's existing Agent Session.
- `checkpoint_handoff`: the latest Agent-authored active-branch checkpoint exposed to siblings. The Agent updates it only when objective, conclusions, evidence, risks, or unresolved work materially change; it is not created by truncating a chat message.
- `returned_handoff`: the final Agent-authored transfer to the parent.
- All three use a bounded Markdown contract: objective/scope, confirmed conclusions, evidence or artifacts, unresolved questions, risks/assumptions, and recommended next action. Detailed material is referenced as an Artifact instead of copied into the Handoff.
- Handoff protocol messages are authenticated `solo message send` calls from the node's assigned real Agent, but are intercepted as control-plane writes and never inserted into raw node conversation history.
- A manually or automatically split child is `fork_handoff_pending` until the parent Session posts its targeted Handoff. Runtime failure keeps the child pending and exposes Retry; it never substitutes the parent's last message.
- `returning_at`: a transient lock while the node's existing local Agent Session generates the handoff. User messages and splits are rejected during this state; only the assigned Agent may post the completing handoff.
- `returned_handoff` and `returned_at`: the final structured handoff and terminal seal.
- Completing Return atomically stores only the authoritative Handoff in `returned_handoff` and inserts one `thinking_handoff` system message in the parent node. That message is rendered with the shared Markdown renderer, while legacy handoff system messages remain detectable by their generated prefix.
- If the local Agent run fails or completes without posting the handoff, `returning_at` is cleared and the node remains active for retry.
- After successful Return the daemon closes `thinking:<node_id>` after the completing turn finishes. Persisted Session and Run records remain for provenance, but the node can never resume them.

Handoff transfer never copies child messages into the parent.

### 5.4 Return state machine

```text
ACTIVE --request return--> RETURNING --Agent handoff posted--> RETURNED
  ^                            |
  +------ runtime failure -----+
```

Return is not recursively propagated. The parent receives the handoff in its branch inbox and runtime turn context, but it runs only on the next explicit user turn. When every direct child is returned, the UI may offer an explicit synthesis action; it does not automatically wake the parent or return it to its own parent.

Return eligibility is authoritative on the server and mirrored by the frontend: a non-root node needs at least one persisted conversation message, no active Agent run, and no direct child lacking `returned_at`. Because every child is subject to the same rule, the direct-child check inductively seals the whole descendant subtree. A blocked parent remains readable and writable; only its Return action is disabled until all children finish.

## 6. Runtime turn lifecycle

```mermaid
sequenceDiagram
    participant UI as Browser UI
    participant API as Solo API
    participant DB as PostgreSQL
    participant D as Local daemon
    participant R as Local Agent Runtime

    UI->>API: POST node-scoped message
    API->>DB: persist message(thinking_node_id)
    API->>DB: load node owner, Handoffs, runtime binding
    API->>D: dispatch(agent_id, node_id, resume_id, turn context)
    D->>D: select session key thinking:node_id
    D->>R: Start/Resume or Send on persistent CLI session
    R->>D: tool call: solo message send
    D->>API: authenticated proxy with thinking_node_id
    API->>DB: persist visible Agent message or intercept Handoff protocol; create optional split nodes
    API-->>UI: WebSocket message/node update
    R-->>D: external session ID and completion
    D-->>API: SSE session/run events
    API->>DB: bind agent_session + finish agent_run
```

### 6.1 Prompt/context layers

The runtime input is deliberately split:

1. Persistent Agent system prompt: existing Agent identity, Solo protocol, workspace, and tools. The administrator-defined role stays under `Initial role`; node-scoped static identity/split rules use a separate `Thinking Runtime` section that is absent from normal channel, thread, and task runs.
2. Turn context: inherited fork Handoff, latest sibling checkpoint/return Handoffs, and returned-child Handoffs loaded at dispatch time.
3. Raw conversation: only messages whose `thinking_node_id` equals the selected node.
4. Current user message.

The daemon must not rely on a `system` role inside `Messages` for node context because current persistent Claude/Codex send paths omit system-role messages. Static node instructions and the checkpoint protocol must be part of the runtime system prompt, and changing Handoff awareness must be sent in a clearly delimited runtime turn-context message.

For an already active or successfully resumed Session, only fresh turn context and the new triggering message are delivered. Full recent node history is reserved for a cold start without a resumable provider Session, preventing duplicate history in persistent CLIs.

### 6.2 Visible responses and automatic split

Solo's dual-channel runtime contract remains unchanged:

- model text is internal runtime output;
- a visible response is created only when the real Agent calls `solo message send`;
- `SOLO_NODE_ID` makes the current CLI attach `thinking_node_id` explicitly;
- the daemon treats the scoped Session that currently owns the Agent turn as the authoritative runtime route. If an older CLI omits the node ID, the proxy restores it from the active `thinking:<node_id>` turn before forwarding. An explicit ID that conflicts with the active turn is rejected instead of being redirected;
- the API independently scopes legacy direct reads/checks from the unique executing node Run. Ambiguous runtime ownership fails closed; it never falls back to channel history;
- only an authenticated Agent message may contain `[[split: title]]` directives;
- the API strips at most three directives and creates idempotent `source=auto` child nodes under the responding node.
- authenticated Handoff protocol messages are parsed only at the message trust boundary, validated against node ownership/lifecycle, persisted to their dedicated field, and omitted from the visible message stream.

The automatic split test must be produced by the local CLI Agent. A synthetic Agent JWT or direct test POST does not satisfy this path.

## 7. API and realtime contract

Channel-scoped endpoints:

- `POST /channels/{channel}/thinking`: idempotently create/load the space.
- `GET /channels/{channel}/thinking`: load topology and fork/checkpoint/return Handoffs.
- `POST /channels/{channel}/thinking/nodes/{node}/children`: manual split.
- `POST /channels/{channel}/thinking/nodes/{node}/handoff/retry`: retry the parent's fork Handoff while a child is pending.
- `POST /channels/{channel}/thinking/nodes/{node}/return`: ask the node's existing Agent Session to prepare and return a handoff to its parent.
- existing message create/list endpoints accept `thinking_node_id` and reject simultaneous thread/task scope.

Authorization always verifies both channel membership and node ownership by that channel.

The Return endpoint starts the handoff and returns the node in `returning` state. The assigned Agent's authenticated node message completes the transition. Message creation rejects user/Agent writes to a returned node and rejects non-owner writes while returning.

Realtime events carry `thinking_node_id`. Topology and Handoff operations emit a Thinking-space update event so all clients refresh manual splits, automatic splits, preparation, checkpoints, and returns without polling. Channel and thread events remain backward compatible.

The server-to-daemon control plane also exposes an internal, idempotent Thinking cleanup call carrying the channel's node IDs. It is not a user API and performs no database mutation; channel authorization and archival remain authoritative on the API server.

## 8. Frontend architecture

- Thinking is a workspace/view mode, not a replacement route or a new channel type.
- The URL is the shareable source of navigation state: channel + `view=thinking` + selected `node`.
- The left conversation panel loads messages with the selected node ID and posts with the same ID.
- The right graph renders topology only; selecting a node changes the URL and the left conversation scope.
- The topology is a center-out brainstorm map, not a top-down organization chart. Each subtree owns one continuous angular sector, deeper nodes move onto outer rings, and the complete layout is recomputed after every split so straight parent-child edges remain non-crossing.
- Live activity has one canonical client state keyed by `agent_run.id`. Teams derives its latest card stack by `agent_id`; Thinking derives it by `thinking_node_id`. This preserves the existing Human message / Activity / Tool cards and status halo while preventing two independent node Sessions owned by the same Agent from displaying each other's work.
- Thinking activity stacks are placed on the node's outward radial side so the selected branch stays readable without turning the brainstorm map back into a top-down tree.
- Small and medium maps (up to ten nodes) keep a complete interactive overview at a minimum readable zoom. Larger topology updates focus newly created nodes together with their parent; larger-map selection focuses the node with its parent, children, and siblings. Fitting the entire graph remains an explicit user action.
- Entering Thinking mode calls the idempotent ensure endpoint and defaults to the root.
- Returning to Team mode restores the existing channel behavior and clears node state.
- Thread and task actions are unavailable in a node conversation because the scopes are mutually exclusive.
- A fork-pending child keeps its conversation read-only and shows preparation/retry state. A returning node keeps the conversation visible but read-only with a progress state. When Return completes, it stays selected so the completed history remains visible. Returned nodes remain selectable from the graph as read-only historical conversations; selecting one never recreates its Runtime session.
- Loading, empty, error, selected, running, and returned states use Solo's existing brutalist tokens, typography, borders, and spacing.

## 9. Concurrency, failure, and cleanup

- Space creation is protected by the unique channel constraint and transaction.
- Root uniqueness and sibling-title uniqueness make retries idempotent.
- Turns for one node serialize on its session key.
- Turns for multiple nodes owned by the same Agent serialize on the Agent workspace lock.
- While that Agent lock is held, the Session manager publishes the exact active scope to the daemon proxy. This binding is runtime state, not Agent-authored input, so stale Agent memory or an absolute path to an older `solo` binary cannot send a node response into the channel.
- Turns owned by different Agents may run concurrently.
- A local process crash resumes the same external provider session using the same node key and `SOLO_NODE_ID`.
- A daemon restart recovers continuity from `thinking_nodes.agent_session_id -> agent_sessions.external_session_id` on the next turn.
- If resume cannot be established, the turn reports a visible runtime error; a controlled cold-start retry uses only that node's recent messages and then rebinds the node to the replacement session. It never falls back to a remote API provider silently.
- Agent deletion closes every normal and node-scoped process owned by that Agent before database deletion.
- Channel archive blocks further message and Thinking access. Its processes remain under the existing persistent-session lifecycle; daemon shutdown closes all sessions and Agent deletion closes every normal and node-scoped Session owned by that Agent. A hard database channel delete cascades persisted Thinking rows.
- Node sessions are created lazily, so unopened branches consume no local runtime process. Open non-returned branches release their local process after the Thinking-only idle timeout and resume from their persisted provider Session on demand.
- Return is rejected while another run owns the node. Its handoff run is the final allowed turn, and successful completion closes that scoped Session.
- After a successful channel soft-delete, the server loads its Thinking node IDs and asynchronously asks every online daemon to close those scoped processes. Cleanup failure is logged and remains recoverable through idle sleep or daemon shutdown; it does not roll back a successful archive.

## 10. Compatibility and migration

- Existing channel rows and messages receive no new behavior unless `thinking_node_id` is present.
- Existing thread APIs and WebSocket scopes are unchanged.
- Existing Agent session manager methods keep their Agent-scoped wrappers; scoped methods extend rather than replace them.
- The idle policy requires no database migration and no frontend state. `agent_session_id` continues to describe provider continuity, not whether a local OS process is currently resident.
- Existing Agent session/run records remain valid because new node foreign keys are nullable.
- The single Thinking migration creates the final Handoff schema directly; no mechanical summary columns or transitional rename/cleanup steps are retained.
- Older `solo` binaries remain compatible while called from a live persistent node turn: the daemon reconstructs missing route metadata. Current binaries still send the explicit field, and normal channel turns have no inferred node scope.
- The migration is reversible: remove node/run scope columns and indexes before dropping Thinking tables/message scope.
- No data is copied from normal channel history into Thinking nodes. Starting Thinking creates a new root conversation boundary.

## 11. Verification matrix

### 11.1 Backend integration tests with real PostgreSQL

- concurrent ensure produces exactly one space and one root;
- team relationships bind Lead/root and first-level Agents correctly;
- manual/auto children inherit `agent_id` but have no initial runtime session;
- fork/checkpoint/return protocol messages persist dedicated Handoffs and never enter raw messages;
- pending fork Handoffs block child conversation and remain retryable after runtime failure;
- node message queries cannot leak sibling/channel/thread messages;
- Return is rejected while any child is active, and the terminal handoff persists exactly one Markdown-capable parent system message;
- Return uses the real assigned local Agent Session, produces a structured handoff, rejects concurrent messages/splits, and seals the node;
- automatic split parsing is bounded and sibling-title idempotency holds;
- runtime session/run binding stores the real node and external session ID.

### 11.2 Daemon/runtime tests

- two turns in one node reuse one external session ID;
- two nodes owned by one Agent use different external session IDs;
- both sessions use the same actual Agent ID/workspace/config;
- normal Agent session and node Session do not collide;
- crash/restart resumes by node key;
- deleting an Agent closes all its scoped sessions.

Unit fakes may test locking and error boundaries here, but they do not count as product E2E.

### 11.3 Real product E2E

Run with the real frontend, API, PostgreSQL, daemon, installed `claude` local CLI, and WebSocket:

1. Create a channel and real Claude-backed Lead/FE/BE/QA Agents with team relationships.
2. Enter Thinking from the UI and verify root/first-level binding through API and rendered graph.
3. Send a root message and observe a visible message posted by the real Lead through `solo message send`.
4. In a real root turn, deliberately remove `SOLO_NODE_ID` for one `solo message send` invocation to emulate a stale CLI. Verify the daemon restores the active Root scope, the Root UI/API/PostgreSQL contain the reply, and the channel scope does not.
5. Send two turns to FE and prove continuity with the same persisted external session ID.
6. Manually split FE; send a child turn; prove the child reuses FE Agent identity but has a different external session ID.
7. During that real child turn, verify the Thinking node renders the Human message, Activity, Tool, and animated status halo while its same-Agent parent renders none of the child's activity.
8. Ask the real FE runtime to emit a split directive and verify an automatic child appears without direct test API injection.
9. Have FE publish an Agent-authored checkpoint, then ask BE about sibling Handoffs; verify awareness without FE raw-message leakage or protocol messages in history.
10. Verify a parent cannot Return while a child is active; Return the child handoff, verify rendered Markdown in the parent UI, then verify the next parent runtime context.
11. While the handoff runs, verify the node is read-only. After completion verify direct message/split APIs reject it, clicking it shows the persisted history with a disabled composer without reopening its Runtime, its scoped Runtime is closed, and PostgreSQL contains the terminal handoff and exactly one parent transfer message.
12. Reload the browser and verify URL selection, topology, messages, all three Handoff roles, and session bindings are restored from PostgreSQL.
13. Confirm ordinary channel and thread messaging still works after leaving Thinking mode.

Passing UI assertions alone, direct signed-Agent test requests, mocked routes, mocked database behavior, or a remote API fallback cannot satisfy this matrix.

## 12. Implementation audit

The original draft deviations have been removed:

- Thinking nodes use persistent `AgentSessionManager` sessions keyed by `thinking:<node_id>` while retaining the existing `agent_id`.
- static node rules and checkpoint protocol are part of the actual runtime system prompt; fork/checkpoint/return Handoffs are supplied separately from raw conversation.
- `thinking_nodes.agent_session_id` and `agent_runs.thinking_node_id` persist node runtime continuity and provenance.
- Run/Session binding validates Agent ownership and exact node scope in one database transaction.
- authenticated Agent node messages must come from the node's assigned Agent.
- root nodes with team children cannot create a second, non-team first layer.
- Playwright starts/verifies the API server, PostgreSQL migration, daemon, frontend, companion `solo` CLI, and real local Claude provider.
- automatic split is emitted by the real local Agent; no signed test-Agent token or route/database mock is used.
- manual splits, automatic splits, Handoff changes, and Return state changes propagate through `thinking.updated` and node-scoped message events.

The verification commands and their terminal results must still be reported separately; this architecture document is not itself evidence of a passing run.
