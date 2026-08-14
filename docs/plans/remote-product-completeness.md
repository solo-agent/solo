# Remote product completeness

## Goal

Make the remote Server + local Daemon topology complete for unattended work,
durable files, multi-user governance, and truthful Token accounting without
changing Solo's existing Agent routing, Task discussion, or local deployment
semantics.

The work is split into four independent modules and shipped together so their
shared remote E2E can validate the complete product path.

## 1. Automation completion policy

### Domain model and lifecycle

An Automation remains the persistent schedule definition. Every occurrence
still creates a distinct `automation_run`, Task, root message, and Thread. The
Task title remains stable; the run and Task number provide occurrence identity.

Add `completion_policy` to an Automation:

- `auto_complete`: after the primary Agent Run completes successfully, mark the
  generated Task `done` and the Automation Run `completed`.
- `review_required`: preserve the current behavior and move the Task to
  `in_review` after the Agent succeeds.

Existing rows migrate to `review_required` for compatibility. Newly created
Automations default to `auto_complete`, matching the common unattended use case.
A running Agent still prevents duplicate dispatch. A Task awaiting review does
not block the next scheduled occurrence, preserving current scheduling behavior.

### Data, API, and frontend

`automations.completion_policy` is returned by all Automation APIs and accepted
by create/update. The Automation form exposes a two-option completion selector;
cards and run history show the effective policy. Invalid policy values are
rejected by the Server.

### Failure recovery

Only a successfully completed primary Agent Run may auto-complete its Task.
Failed, timed-out, cancelled, unavailable, or human-returned work never becomes
Done. Reconciliation stays idempotent so scheduler restarts cannot double-apply
completion.

## 2. Durable remote file storage

### Ownership and persistence

Attachments and Artifacts remain Server-owned files in Docker named volumes.
PostgreSQL stores their metadata; Agents receive authenticated downloads through
the Server and materialize them on the local Daemon as today. Object storage is
not introduced in this release.

The production image runs as the non-root `solo` user. A one-shot
`storage-init` service owns both named-volume roots and sets them to `solo:solo`
before migration and Server startup. The Server validates both configured roots
at startup by creating the root and a temporary probe file. Production fails
fast with a precise error rather than accepting traffic with broken uploads.

### Failure recovery and compatibility

The init step is idempotent and repairs existing root-owned volumes. Files are
not deleted or moved. Local non-production deployments keep their current
default paths but receive the same writable-directory validation when a path is
explicitly configured.

## 3. Workspace governance, pinning, and muting

### Roles

Workspace roles remain `owner`, `admin`, and `member`:

| Capability | Owner | Admin | Member |
| --- | --- | --- | --- |
| Read, post, use Tasks and Agents | yes | yes | yes |
| Update Workspace/Channel information | yes | yes | no |
| Pin and unpin Channel messages | yes | yes | no |
| Restrict Channel posting or mute a Member | yes | yes | no |
| Invite/remove Members in private Workspace | yes | yes | no |
| Promote/demote/remove Admin | yes | no | no |
| Delete Workspace | yes | no | no |

The default Public Workspace keeps its current open-membership rule: nobody may
be removed. Owner/Admin moderation still applies to posting, pins, and metadata.
Private Workspaces permit Owner/Admin to remove Members. An Admin cannot mutate
or remove another Admin or an Owner.

`SOLO_PUBLIC_OWNER_EMAILS` is a comma-separated deployment setting. On Server
startup, matching verified users are idempotently promoted to Owner in the
Public Workspace. Registration also applies the rule, so configured owners get
the role regardless of registration order. This is not a global private-
Workspace backdoor.

### Channel moderation model

Moderation authority derives from the caller's Workspace role; Channel member
roles continue to describe membership and Agent ownership but do not create a
second conflicting human administrator hierarchy.

Each Channel receives `posting_policy` (`everyone` or `admins_only`, default
`everyone`). Per-user mutes live in `channel_member_mutes` with optional
`expires_at`, reason, and actor. Expired mutes are ignored. Human message, Thread
reply, and Task creation paths in ordinary Channels enforce posting permission. DM
and private system scopes keep their existing behavior. Agent
runtime delivery is not muted by human moderation in this version.

Message pins live in `channel_message_pins` with one row per Channel/message and
the actor/time. Pins do not alter message ordering or content. List APIs include
pin state; a dedicated endpoint lists pinned messages. Only non-deleted messages
in the same Channel can be pinned.

### API and frontend state

Workspace member APIs enforce the hierarchy above. Channel update, moderation,
and pin endpoints return 403 for insufficient roles. The members/settings UI
shows only actions the current role can perform. A pinned indicator and pin
action are available in the message menu; Channel settings expose posting policy
and active member mutes. WebSocket events invalidate message/member state after
pin or moderation changes.

Moderation controls represent actions and state separately. An unmuted Member
has an explicit mute action; after activation the control enters a disabled
loading state, then changes to a persistent highlighted "muted" state whose
action is unmute. Tooltips and accessible labels name both actions. Pinning uses
the same loading and confirmed-state feedback for both human and Agent messages.
Success and failure toasts make the result observable even when a realtime
refresh is delayed. These controls do not optimistically invent Server state:
the confirmed style is derived from the refreshed pin/mute collections, while a
per-control pending state prevents duplicate writes.

### Failure recovery

All policy decisions are enforced on the Server, not only hidden in the UI.
Mute expiry is evaluated at write time and needs no background worker. Soft-
deleting a message removes its pin in the same transaction. Removing a private-
Workspace member removes their mutes in the same Workspace membership
transaction.

## 4. Remote per-turn Token accounting

### Ownership and data flow

The local Daemon is the only component that can read local provider transcripts,
so it owns usage recovery. Every backend returns usage for the current turn.
The Daemon sends that usage in the existing `complete`/`error` event; the Server
persists and settles it without reading remote paths.

Provider strategy:

- Claude: sum unique assistant usage records created during the current turn.
  The authoritative local transcript is preferred, with current-turn stream
  usage as the fallback. Duplicate transcript records are deduplicated by the stable model
  message ID, with the outer event UUID used only as a compatibility fallback.
- Codex: use `last_token_usage` for the completed turn when available. Otherwise
  compute the difference between the last cumulative `total_token_usage` at the
  turn start and finish. The scanner reads the authoritative transcript path,
  not a date directory derived from the new Run time, because persistent
  Sessions continue writing to their original file.
- Other providers retain their existing protocol usage behavior.

Usage is always a per-turn quantity. Persistent Session totals must never be
charged again on later Runs. Zero with no trustworthy measurement remains
`usage_unknown`; the UI presents it as unavailable rather than as measured zero.

### Compatibility and recovery

The Server's local-transcript fallback remains temporarily for old local Daemon
versions, but remote Computers never depend on it. The event wire format does
not change. Backend unit tests use real transcript fixtures, while final E2E uses
installed Claude and Codex runtimes.

## Migrations

One ordered migration adds:

- `automations.completion_policy`;
- `channels.posting_policy`;
- `channel_member_mutes`;
- `channel_message_pins`.

Down migration removes only these new objects. Public owners are configuration-
driven data and are not hard-coded into migration SQL.

## Verification

All product-flow validation uses the make-managed stack, real frontend, API
Server, PostgreSQL, and local Agent runtime where required.

1. Automation: two occurrences create distinct Tasks; auto mode reaches Done;
   review mode reaches In Review; an active Run is not duplicated.
2. Storage: a cold production volume accepts avatar, image, and file uploads;
   authenticated download and Agent materialization work; files survive restart.
3. Governance: configured Public owners are present; Public removal is rejected;
   private Member removal succeeds; Admin cannot manage Admin; Member cannot
   moderate; pin and mute behavior is visible and persisted.
4. Token usage: remote Server + local Claude and Codex each produce a non-zero
   Run; a second persistent-Session turn records only its own usage; database,
   budget settlement, and observability agree.

Production deployment happens only after code review and local E2E pass. The
existing Singapore database and volumes are backed up before migration; deploy
then runs the storage initializer, migrations, and health checks before remote
smoke/E2E.
