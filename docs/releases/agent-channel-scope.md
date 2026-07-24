# Agent Channel Scope

This release changes the ownership boundary of ordinary Agents: every newly
created Agent belongs to exactly one visible Channel.

## What changes

- Create and list ordinary Agents from inside a Channel.
- An Agent cannot be moved to or reused by another visible Channel.
- Creating a Channel from an official template atomically creates fresh
  Channel-scoped Agents and their relationships.
- The former Teams page is removed. The active team is shown and edited in its
  Channel.
- Welcome becomes the pinned Lucy Channel, with Lucy's special icon and the
  same chat-and-Team layout as other Channels. Lucy responds there and in
  hidden DMs, but does not receive messages from other Channels.
- Closing a Channel deactivates its Agents, releases unfinished claimed work,
  cancels active runs, closes sessions, and preserves history.

Hidden DMs and the existing All Channel are not removed in this release.

## Existing Agents require action

The database migration intentionally **does not guess a Channel** for existing
global ordinary Agents. It leaves their active state, memberships, tasks, runs,
sessions, configuration, and runtime behavior unchanged.

After upgrading:

Existing global Agents are not offered by the new Agent or Team creation flows.
When you are ready to adopt Channel scope:

1. Open the target Channel.
2. Create a fresh Agent there, apply an official template to an empty Channel,
   or create a new Channel from an official template.
3. Copy any still-relevant prompt or runtime configuration from the old Agent
   manually.
4. Verify the new Agent in its Channel before manually retiring the old Agent
   or deleting any external workspace files you still need.

There is no migration compatibility layer, automatic clone, or automatic
rewiring. Legacy global Agents continue on their old path until the user
chooses to replace or remove them.

## Lucy migration

Each user's existing Welcome Channel becomes the pinned Lucy Channel. An
existing Lucy Agent is attached to that Channel and removed from other visible
Channel memberships. Users who had not configured Lucy can finish setup inside
the Lucy Channel.

## Templates

This release ships 32 official built-in templates adapted from
agency-orchestrator's documented workflows. Users can preview and select them
from `/templates`, apply one to an empty Channel, create a new Channel from one,
or ask Lucy to recommend one. Saving a customized team as “My template” is
planned for a later release.

Lucy queries the live catalog with:

```sh
solo template list --json
```

and creates the selected team with `solo team form` using an exact
`template_id`. Template member and relationship overrides are not accepted.

## Rollback note

The down migration removes the new schema constraints and columns. It does not
reconstruct prior memberships or undo Channel-scoped Agents created after the
upgrade. Take a database backup before upgrading if a full data rollback is
required.
