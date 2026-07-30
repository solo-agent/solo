# Space-Containing Agent Mentions Design

## Problem

Solo currently inserts a selected agent mention into message text as
`@<display name>`, but both the frontend and backend parsers stop at the first
whitespace character. An agent named `dataleap-global coding developer`
therefore appears correctly in the UI while being parsed as
`@dataleap-global`.

The resulting message has an empty `mentioned_agent_ids` array. When that
message becomes a task, Solo routes it to the channel coordinator instead of
the intended agent. The task remains unclaimed by the named Trae role even
though its card visually contains the full `@name`.

## Goals

- Support agent display names containing spaces across channel messages,
  threads, direct messages, task creation, and message-to-task conversion.
- Resolve manually typed exact full names as well as names inserted from the
  mention suggestion list.
- Preserve readable `@display name` message text and the current database/API
  representation.
- Route mentioned tasks to the intended active channel agents.
- Avoid prefix matches and accidental fuzzy matches.
- Recognize Trae ACP terminal calls that execute `solo message send`.

## Non-Goals

- Migrating message text to an embedded ID syntax such as `<@agent-id>`.
- Fuzzy matching misspelled or partial agent names.
- Automatically changing or rerunning historical tasks.
- Changing task claim-window or coordinator-selection semantics.

## Chosen Approach

Use channel-member-aware longest exact matching.

The parser receives message content and the active agent members visible in the
conversation. At each `@` position it considers member display names and
selects the longest exact name whose end is followed by the end of the message
or a valid mention boundary. Matches are returned in message order and
deduplicated by agent ID.

This approach keeps current storage and APIs intact, supports existing readable
message text, and fixes both suggestion-selected and manually typed mentions.

Alternatives rejected:

- Quoted syntax such as `@"Agent Name"` would be unergonomic and would not fix
  existing user input.
- Embedded ID tokens would be robust against renames but require a broader
  message-format, editor, renderer, search, and migration redesign.

## Mention Semantics

### Matching

- Matching is exact and case-sensitive, consistent with current agent-name
  resolution.
- Longer matching names take precedence over their prefixes.
- A match must start immediately after `@`.
- A match must end at end-of-content or before whitespace or punctuation.
- Supported names naturally include Unicode letters, digits, spaces, hyphens,
  underscores, and dots because matching uses stored names rather than a
  restricted name regex.
- Unknown `@text` remains ordinary message text and produces no agent ID.
- Repeated mentions of the same agent produce one ID.
- Multiple matching agents with the same active display name produce all
  matching IDs; the parser does not silently choose one.

### Scope

Only active agent members of the current channel or DM are eligible. Users are
not returned as agent mentions. Existing user-mention handling remains
independent.

## Architecture

### Backend

`MentionService.ResolveMentions` becomes the canonical server-side resolver:

1. Detect whether the content contains any `@` character.
2. Load active agent members for the supplied channel.
3. Sort candidate names by descending character length.
4. Scan each `@` position and apply exact name plus boundary matching.
5. Return deduplicated IDs in message order.

All existing callers retain the same method contract. This covers REST channel
messages, direct messages, message conversion, and task routing without
duplicating parsing logic.

For message creation requests that already contain explicit
`mentioned_agent_ids`, the server continues to respect and validate those IDs.
Server-side text resolution remains the compatibility path for manually typed
mentions and message-to-task conversion.

### Frontend

The mention hook continues to insert `@<full display name>` when a suggestion
is selected. Its highlighting and `mentionedAgentIds` derivation use the same
member-aware longest-match semantics instead of `/@(\S+)/`.

Channel, DM, and thread composers continue sending the existing
`mentioned_agent_ids` field. No wire-format change is required.

### Task Routing

Task routing behavior remains unchanged after resolution:

- If one or more agent IDs are resolved, only those agents are immediately
  triggered and receive the priority claim window.
- If none are resolved, Solo triggers the channel coordinator.
- The triggered agent decides whether to claim through `solo task claim`.

### Trae CLI Message Detection

Daemon message-send detection is extracted into a small provider-neutral
helper. It recognizes shell-like tool names used by supported backends,
including `Bash` and Trae ACP's `terminal`, and checks their normalized command
payload for `solo message send`.

This does not create channel messages itself. It accurately records that the
agent already created the visible message through the Solo CLI and prevents
misleading suppression diagnostics.

## Error Handling

- Database errors while loading channel agents are returned to callers.
- No match is a normal result, not an error.
- Unknown or incomplete `@` fragments never trigger an agent.
- A failed task mention resolution preserves the existing coordinator fallback
  instead of dropping the task.
- The parser operates on Unicode text and does not split stored display names
  on whitespace.

## Testing

### Backend Resolver

Tests cover:

- a full name containing spaces;
- manual exact input;
- prefix conflicts such as `data` and `data engineer`;
- Chinese and mixed-language names;
- punctuation and end-of-content boundaries;
- multiple and duplicate mentions;
- unknown or partial names;
- active channel membership filtering;
- duplicate active display names.

### Handlers and Routing

Tests cover:

- channel message creation;
- thread messages;
- direct messages;
- direct `as_task` creation;
- message-to-task conversion;
- resolved IDs passed to `TriggerAllAgentsForTask`;
- coordinator fallback when no exact match exists.

### Frontend

Tests cover suggestion selection, manual full-name input, highlighting, prefix
conflicts, and the outgoing `mentioned_agent_ids` payload.

### Trae Regression

Tests verify that a `terminal` ACP tool call containing `solo message send` is
recognized as a CLI-sent message.

A real end-to-end check creates or uses a Trae agent whose display name
contains spaces, sends it a task mention, and verifies:

- the message stores the Trae agent ID;
- the task run uses provider `trae`;
- the Trae agent claims the task;
- the task moves to `in_progress`;
- the reply is visible in the task thread.

## Rollout and Compatibility

No schema migration or API version change is required. Historical messages and
tasks remain unchanged. After deployment, newly sent messages and newly
converted tasks use the corrected resolver. Existing task #3 may be explicitly
retriggered after the fix if desired.
