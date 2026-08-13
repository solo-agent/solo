# Agent Message Wake Coalescing

Status: implementation design
Scope: persisted Channel and Thread messages that may wake an Agent. Thinking-node, scheduled, greeting, and task Runs keep their existing explicit dispatch paths.

## 1. Product behavior

- A public ordinary Channel is mention-only for Agents. Persisting and broadcasting an unmentioned message still succeeds, but it does not create an Agent Run.
- DM and Lucy messages remain conversational and may wake an Agent without `@`.
- Private ordinary Channels retain the existing coordinator/small-team/recent-Agent routing fallback.
- One Agent executes at most one message-triggered Run per Channel at a time. This matches the runtime Session key, which is shared by the Channel and its Threads.
- Messages arriving while that Run is active are not overwritten. Each message remains an immutable `messages` row. The pending wake records the first and latest unconsumed sequence for each Channel or Thread scope, and the next Run receives every persisted message in that range in sequence order.
- An explicit mention, DM, or Lucy turn requires a user-visible result. An automatic private-Channel fallback may finish silently and is not retried solely because no visible message was sent.
- A Channel-level Run may deliver its visible result either to the Channel root or to a Thread under that same Channel. The reply stays attributed to the originating Run; otherwise the terminal checker would mistake a valid Thread reply for a missing result and enqueue a reminder loop. Thread and Thinking-node Runs remain bound to their exact scope.

## 2. Domain model, ownership, and lifecycle

`agent_message_wake_slots` is the server-owned single-flight lease for `(agent_id, channel_id)`. Its `active_run_id` points at the currently executing message Run. The row is created lazily and contains no user-authored content.

`agent_pending_message_wakes` is server-owned delivery state. A row is unique per `(agent_id, channel_id, scope_key)`, where `scope_key` is `channel` or `thread:<thread_id>`. It stores `first_message_seq`, `latest_message_seq`, and whether any merged trigger requires a visible result. The authoritative content remains in `messages`.

Lifecycle:

1. Routing selects a target Agent after a message is persisted.
2. A transaction locks the Agent/Channel wake slot.
3. If no message Run is active, the same transaction creates and budget-reserves a Run and assigns it to the slot.
4. If a Run is active, the transaction upserts the scope's pending interval using `LEAST(first_seq)` and `GREATEST(latest_seq)`; visibility requirements are combined with logical OR.
5. When the active Run reaches a terminal state, a transaction releases that exact Run, claims the oldest pending scope, creates and reserves its next Run, deletes only the claimed pending row, and assigns the new Run to the slot.
6. Messages that arrive after the claim block on the same slot lock and form a new pending interval, so they cannot be lost or accidentally included in the already claimed Run.

## 3. Data flow

For an immediate Run, the existing persisted trigger message is its turn input. For a coalesced Run, the server loads all non-deleted messages from the claimed Channel/Thread scope whose sequence is inside the pending interval, orders them by sequence, and converts them to the same runtime message format used by a normal wake. Gaps caused by messages in another Thread are ignored by the scope filter.

The latest message in the claimed range is the Run's audit `trigger_message_id`; the full range is its runtime input. Mention names are rebuilt from the loaded rows. Existing recent/cold-start context continues to be supplied where the runtime already expects it.

The browser and WebSocket protocol do not gain optimistic pending state. Users see their independently persisted messages immediately and see at most one active Agent turn indicator for the Agent/Channel Session.

## 4. Persistence and atomicity

The two wake tables reference Agents, Channels, Runs, and Threads with cascading or nulling foreign keys appropriate to their ownership. Pending range constraints require positive sequence numbers and `first_message_seq <= latest_message_seq`.

Run creation is factored into a transaction-aware primitive so Run insertion, token-budget reservation, and wake-slot assignment commit together. The slot row, not an in-memory mutex, serializes multiple API server processes.

The claimed sequence range and visible-result requirement are also persisted on the Run. For remote Computers, the complete dispatch payload is stored in that same transaction, closing the Server-crash gap between claim and Daemon notification. A legacy Daemon that confirms it never received a claimed Run requeues that Run's persisted range before marking the Run lost.

No message bodies are copied into the pending table, and no pending update deletes or rewrites a message.

## 5. Failure recovery

- A transaction failure leaves either the old active/pending state or the new state, never a half-claimed interval.
- If dispatch fails after a Run is created, the existing Run failure path terminates it and advances the queue.
- The Agent Run watchdog reconciles wake slots on every pass. A slot pointing to a terminal/missing Run is repaired, while a still-unfinished Run is preserved for normal daemon recovery.
- On upgrade, a newly created slot also checks existing unfinished message Runs before starting another. Existing duplicate Runs drain first; pending work starts only after the last unfinished legacy Run terminates.
- If the newest range message was deleted before claim, the server loads the remaining persisted messages. If none remain, it drops that empty pending interval and continues to the next one.
- Daemon disconnection does not discard pending ranges. A claimed Run follows the existing daemon-lost/timeout recovery and terminal handling.

## 6. API and frontend compatibility

No public HTTP or WebSocket schema changes. Existing message-send responses, reliable-send deduplication, Channel membership, Workspace scoping, and frontend state remain unchanged. Run/event payloads continue using the latest persisted trigger message ID.

Thinking nodes remain isolated and are not coalesced by this flow because their node Session, handoff, and debounce semantics are separate. Explicit tasks, schedules, and onboarding greetings are not converted into message ranges.

## 7. Migration

The migration only creates empty delivery-state tables and indexes. Existing messages and Runs are not rewritten. Lazy slot reconciliation provides rolling compatibility with unfinished pre-migration Runs. The down migration drops the delivery state without touching messages or Run history.

## 8. Validation

Validation uses the make-managed frontend, API server, PostgreSQL, and real Daemon/Agent runtime without route, service, database, or provider mocks. It must prove:

1. Several messages sent while one real Agent Run is active persist as distinct rows and produce one pending range.
2. The terminal transition atomically starts exactly one next Run containing all of those messages in sequence order.
3. A message arriving after the claim forms a separate pending range.
4. Public unmentioned messages create no Run or pending wake, while an explicit mention does.
5. DM/Lucy and explicit mentions retain the visible-result contract; automatic private fallback does not generate missing-visible-result reminder loops.
6. A Channel Run replying into its trigger message's Thread is attributed to that Run and does not generate a result reminder.
7. Browser-visible messages, Run indicators, terminal results, and database Run/wake/message state agree.
