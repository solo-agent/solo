# Workspace and Multi-Daemon Architecture

Status: implementation design
Scope: one hosted Solo Server, many logical Workspaces, many users, many Computers/Daemons per Workspace, and multiple Daemon instances on one physical machine.

## 1. Product boundary

Solo remains a single hosted platform. A Workspace is the top-level collaboration and data-isolation boundary inside that platform; it is not another Solo Server and it is not an Agent filesystem workspace.

```text
Solo Server
├── Public Workspace (every registered user)
├── Private Workspace A
│   ├── User A -> Computer A -> Agent A
│   └── User B -> Computer B -> Agent B
└── Private Workspace B
```

The first complete flow must allow a user to create Workspaces, switch between them, add registered users, and run Agents owned by different Workspace members on different Daemons. A single physical machine may run multiple independent Daemon instances for development, testing, or multi-account use.

Server federation is explicitly out of scope.

## 2. Domain model and ownership

### Workspace

- Fields: ID, name, icon, visibility (`public` or `private`), immutable `is_default`, creator, timestamps.
- The Server owns one non-deletable default public Workspace.
- A user may create private Workspaces and becomes their `owner`.
- Workspace roles are `owner`, `admin`, and `member`.
- Every verified registered user has a durable `workspace_members` row in the public Workspace.

### Channel and collaboration data

- Every Channel, including DMs and Lucy Channels, belongs to exactly one Workspace.
- Messages, Threads, Tasks, Thinking Spaces, Artifacts, Runs, and relationships inherit their Workspace through their Channel or Agent. We avoid duplicating `workspace_id` where the existing foreign-key path is authoritative.
- Channel names are unique only inside a Workspace.
- Workspace membership grants every User membership of every active ordinary Channel (`type='channel'`) in that Workspace. This is the initial Discord-like visibility rule: selecting a Workspace shows all of its ordinary Channels without a separate User join flow.
- User Channel memberships are a materialized authorization/index record derived from Workspace membership. Creating an ordinary Channel inserts all current Workspace Users; adding a Workspace User inserts that User into all existing ordinary Channels; removing a Workspace User removes their inherited Channel memberships.
- Agent Channel membership remains explicit and is the finer-grained execution/context boundary. Lucy Channels and DMs are excluded from User inheritance and keep their existing participant-specific membership.
- The ordinary Channel creator is recorded as Channel `owner`; other inherited Users are `member`. Future private/permission-overridden Channels may refine this rule, but are outside the first implementation.

### User, Computer, Daemon, and Agent

- Users are platform-global identities.
- Computers are owned by users and remain platform-global; they are not transferred to a Workspace.
- One Computer record represents one independently paired Daemon instance. Multiple Computer records may have the same physical hostname.
- Agents remain owned by users. An Agent belongs to a Workspace through its immutable home Channel.
- Lucy is a Workspace steward: one active Lucy Agent instance per `(owner, Workspace)`, rather than one Lucy for the entire platform account. Each instance has a distinct Agent ID, home Channel, filesystem workspace, memory, and provider Session chain. Only execution configuration may be copied.
- A Workspace can therefore contain Agents owned by many users and executed by many Daemons.
- Other members may discover and collaborate with an Agent but cannot read or mutate its private runtime configuration, credentials, environment, arguments, Computer binding, or filesystem.

### Invitations, allowlists, guests, and embeds

- A registered-user add writes `workspace_members(user_id)` directly.
- A pre-registration invitation is keyed by normalized verified email and becomes a user-ID membership atomically after registration verification.
- Workspace join rules are explicit email/domain allowlist entries; they are not membership records.
- Guest access uses a separate short-lived guest identity and a scope-bound token. Guests never own Computers or Agents.
- Embed access is a guest capability restricted to one Workspace and an allowlisted set of Channels.

These entry modes build on the same Workspace authorization boundary; they must not create a second membership truth.

## 3. Lifecycle

### Public Workspace bootstrap

1. Migration creates the default public Workspace.
2. Existing active users are backfilled as members, but their pre-Workspace Channels are not public content: each user receives a personal Workspace and their legacy Lucy/owned history moves there. Existing Channel collaborators become members of that personal Workspace.
3. Public contains the shared public lobby and content explicitly created there after Workspace support; it is not a catch-all migration bucket for private history.
4. New registration verification atomically joins public and creates the user's personal Workspace.
5. The onboarding Lucy Channel is created inside the personal Workspace.

### Private Workspace

1. An authenticated user creates a Workspace.
2. Server transaction creates Workspace, owner membership, a `general` Channel, and a pinned Lucy Channel. If the owner already configured Lucy, the Server creates a new Lucy Agent and copies only that owner's runtime/Computer/model configuration; the source Lucy Agent, memory, and Sessions are never reused. Otherwise the existing setup card is shown.
3. Owner/admin adds members or creates invitations/join rules.
4. Every ordinary Channel immediately contains all current Workspace Users, and every subsequently added User is inserted into every existing ordinary Channel in the same membership transaction.
5. A member creates their own Agent in a Channel and chooses one of their accessible Computers. Agents are not propagated to other Channels.
6. Workspace deletion is owner-only, rejects the default Workspace, rejects deletion while Runs are active, and soft-deletes the Workspace. Channels are archived, Agents and sessions are deactivated, Guest links are revoked, and Daemon workspaces are cleaned best-effort. Persisted collaboration/run history and user-owned Computers survive for auditability.

### Daemon instance

1. `solo daemon connect --profile NAME` stores state in `~/.solo/daemons/NAME/`.
2. Profile owns its credentials, PID, log, HTTP port, Daemon ID, and process lock.
3. The paired Computer ID remains the Server-side routing identity.
4. Starting the same profile twice is rejected; different profiles on one machine may run concurrently.
5. Existing no-profile commands map to the `default` profile for compatibility.
6. Workspace switching never reconnects a Daemon. One connected Computer can execute all Agents that its owner binds to it across any number of Workspaces; profiles model independent Computers/accounts, not Workspace sessions.
7. Agent Sessions remain lazy runtime records. The first turn of each Workspace's Lucy creates or binds a Session for that Lucy's distinct Agent ID; the Server never resumes another Workspace Lucy's Session.

## 4. Data flow and authorization

### Browser request

1. Workspace Provider loads `/api/v1/workspaces` after authentication.
2. It restores an accessible active Workspace ID from local storage, otherwise selects the public Workspace, otherwise the first accessible Workspace.
3. API client sends `X-Workspace-ID` on Workspace-scoped requests.
4. Server middleware validates the user has a Workspace membership and stores the Workspace ID and role in request context.
5. Handlers scope collection queries by that Workspace. Resource handlers verify the resource resolves to that Workspace as well as applying existing owner/member rules.
6. Ordinary Channel lists continue using `channel_members`; the inherited User rows make the existing frontend, message authorization, WebSocket subscription, and member directory paths agree without special cases.

Computer management and profile management are intentionally user-global. Workspace CRUD routes identify their Workspace in the URL and perform their own membership checks.

### WebSocket

1. Browser connects with `workspace_id` in the WebSocket query.
2. Server validates Workspace membership before accepting scoped subscriptions.
3. Channel/Thread/DM subscription verifies both resource Workspace and resource membership.
4. Switching Workspaces reconnects the socket, clears previous subscriptions, and refetches Workspace-scoped state.

### Agent run

1. A Channel message selects only Agents whose home Channel resolves to the active Workspace.
2. Agent ownership resolves its configured Computer.
3. Server routes the run to the control connection keyed by that Computer ID.
4. Different Agents in one Workspace may route to different users' Computers and Daemons.
5. Run events persist through existing Agent/Channel foreign-key paths and are returned only to members of the corresponding Workspace.

## 5. Persistence and constraints

New core tables:

- `workspaces`
- `workspace_members`
- `workspace_invitations`
- `workspace_join_rules`
- `workspace_embed_policies`
- `workspace_embed_channels`

Core schema changes:

- `channels.workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE`
- Replace the global active Channel-name unique index with a Workspace-local one.
- Add indexes for membership lookup and Workspace Channel lists.
- `channel_members(member_type='user')` materializes the Workspace-to-ordinary-Channel inheritance rule; `member_type='agent'` remains explicit per Channel.

The public Workspace uses a stable UUID constant so migrations, registration, recovery, and tests agree on identity without a name lookup.

## 6. API contract

Core endpoints:

- `GET /api/v1/workspaces`
- `POST /api/v1/workspaces`
- `GET/PATCH/DELETE /api/v1/workspaces/{workspaceID}`
- `GET/POST /api/v1/workspaces/{workspaceID}/members`
- `PATCH/DELETE /api/v1/workspaces/{workspaceID}/members/{userID}`
- invitation and join-rule CRUD below the same Workspace route
- embed policy and scoped guest-token minting below the same Workspace route

Existing Workspace-scoped endpoints keep their URLs for compatibility and use `X-Workspace-ID`. If the header is absent, the public Workspace is selected. Responses include `workspace_id` where it helps the client reject stale state.

## 7. Frontend state and design

`WorkspaceProvider` is mounted inside authentication and outside WebSocket/data hooks. It owns:

- accessible Workspaces
- active Workspace
- persisted selection
- create/update/delete/member mutations
- a monotonically changing Workspace key used to invalidate scoped hooks

The existing cream Channel Sidebar remains the only left column. Its Workspace identity header opens a Solo-themed switcher containing accessible Workspaces plus create/delete actions. A collapsible People section exposes everyday member visibility and invitation controls; advanced membership policy, allowlists, and Guest/Embed configuration live in Settings. No second Workspace rail is introduced.

Settings composes existing profile, budget, and active-Workspace state without
new persistence or API behavior. The Workspace card follows the Token Budget
visual hierarchy: themed card surface, full-width semantic-primary header,
left icon with title/subtitle, and a right-side Workspace mark. Theme tokens
continue to own color, border width, corner radius, and shadow, preserving both
Archive and Classic. The profile header icon uses the current foreground color
so it remains visible when Archive's primary and surface colors are both pale.

On switch, navigation returns to `/dashboard`, Channel/DM selection is cleared, queries refetch, reliable-send queues are not replayed into a different Workspace, and the WebSocket reconnects.

## 8. Failure recovery

- Workspace creation, soft deletion, registration membership, accepted invitations, and initial Channel creation are transactional.
- Ordinary Channel creation and Workspace User addition write all inherited User memberships in the same transaction. A failure rolls back the Channel or Workspace membership rather than leaving partial visibility.
- A failed invitation acceptance leaves the invitation pending and does not create a partial membership.
- Missing/inaccessible persisted active Workspace falls back deterministically.
- Deleting the active private Workspace switches clients to public after the delete response.
- Daemon profile startup writes PID only after spawn; stale PID files are removed after process checks.
- Port allocation is explicit or selected from an available loopback port and persisted per profile. Restart reuses it unless occupied, in which case startup fails with a clear profile/port error rather than controlling another instance.
- A Daemon control disconnect marks only its paired Computer offline. Other profiles on the same host continue running.

## 9. Migration and compatibility

- Existing users gain public membership and a personal Workspace.
- A data migration backfills every existing Workspace User into every active ordinary Channel in that Workspace. Lucy Channels and DMs are untouched.
- Channels that predate Workspace support move to the creator's personal Workspace without changing IDs. Lucy/onboarding Channels created during the incorrect interim public behavior are repaired as personal content too. Content explicitly created in Public after Workspace support remains public.
- Existing API and CLI callers without a Workspace header operate in public.
- Existing `solo daemon ...` commands operate on profile `default` and the historical credential is migrated/read compatibly.
- Existing deployments still use `make rebuild`; no additional service manager is introduced.
- Existing Agent filesystem workspaces are unrelated and do not move.

## 10. Validation

No route, service, database, or Agent runtime mocks are accepted for product-flow validation.

The final E2E must use the make-managed frontend, API Server, PostgreSQL, and real Daemon/Agent runtime path to prove:

1. User A and User B both belong to public.
2. User A creates Workspace Alpha and adds User B.
3. User A creates Workspace Beta and switches Alpha/Beta/public without data leakage.
4. Two Daemon profiles run concurrently on the same physical test machine and pair to two distinct Computers.
5. Agents owned by different users and bound to those distinct Computers join one Alpha Channel.
6. A visible message/task causes each selected Agent run to be delivered only to its owning Daemon.
7. Browser-visible messages/results and persisted Workspace/Channel/member/run state agree.
8. Users and WebSockets cannot access Workspace resources without membership.
9. Two Workspaces owned by one user have different Lucy Agent IDs, home Channels, filesystem workspaces, and Session owners even when both Lucy Agents use the same Computer.
10. An ordinary Channel created after multiple Users joined a Workspace is visible to all of them; a User added after Channel creation is backfilled into that Channel; Lucy and Agent memberships remain isolated.
