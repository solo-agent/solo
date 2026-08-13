ALTER TABLE workspaces
    ADD COLUMN is_personal BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX idx_workspaces_active_personal_owner
    ON workspaces(created_by)
    WHERE is_personal = true AND deleted_at IS NULL;

-- A user's pre-Workspace Solo history was private by default. Give every
-- account one personal Workspace so that history is not presented as public.
INSERT INTO workspaces (name, icon, visibility, is_personal, created_by)
SELECT left(u.display_name || '''s Workspace', 100),
       upper(left(COALESCE(NULLIF(trim(u.display_name), ''), 'S'), 1)),
       'private', true, u.id
  FROM users u
 WHERE NOT EXISTS (
       SELECT 1 FROM workspaces w
        WHERE w.created_by = u.id AND w.is_personal = true AND w.deleted_at IS NULL
 );

INSERT INTO workspace_members (workspace_id, user_id, role)
SELECT w.id, w.created_by, 'owner'
  FROM workspaces w
 WHERE w.is_personal = true AND w.deleted_at IS NULL AND w.created_by IS NOT NULL
ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = 'owner';

-- Migration 44 temporarily used Public as a catch-all. Move only rows that
-- predate that migration, plus unmistakably per-user onboarding Channels
-- created during the interim. Content explicitly created in Public afterward
-- stays public.
UPDATE channels c
   SET workspace_id = personal.id,
       updated_at = now()
  FROM workspaces personal
 WHERE personal.is_personal = true
   AND personal.deleted_at IS NULL
   AND personal.created_by = c.created_by
   AND c.workspace_id = '00000000-0000-0000-0000-000000000001'
   AND c.id <> '00000000-0000-0000-0000-000000000002'
   AND (
       c.created_at < COALESCE(
           (SELECT applied_at FROM schema_migrations WHERE version = '000044_create_workspaces'),
           now()
       )
       OR c.type = 'lucy'
       OR c.name LIKE 'all-%'
   );

INSERT INTO channels (id, workspace_id, name, description, type, created_by)
SELECT '00000000-0000-0000-0000-000000000002',
       '00000000-0000-0000-0000-000000000001',
       'general', 'Public lobby for everyone on Solo', 'channel', u.id
  FROM users u
 WHERE NOT EXISTS (
       SELECT 1 FROM channels c
        WHERE c.workspace_id = '00000000-0000-0000-0000-000000000001'
          AND c.name = 'general' AND c.type = 'channel' AND c.is_archived = false
 )
 ORDER BY u.created_at ASC
 LIMIT 1
ON CONFLICT (id) DO NOTHING;

INSERT INTO channel_members (channel_id, member_type, member_id, role)
SELECT public_channel.id, 'user', wm.user_id, 'member'
  FROM workspace_members wm
  JOIN LATERAL (
       SELECT id FROM channels
        WHERE workspace_id = '00000000-0000-0000-0000-000000000001'
          AND name = 'general' AND type = 'channel' AND is_archived = false
        ORDER BY created_at ASC LIMIT 1
  ) public_channel ON true
 WHERE wm.workspace_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

-- Preserve every historical collaborator when its Channel moves.
INSERT INTO workspace_members (workspace_id, user_id, role)
SELECT DISTINCT c.workspace_id,
       cm.member_id,
       CASE WHEN cm.member_id = w.created_by THEN 'owner' ELSE 'member' END
  FROM channels c
  JOIN workspaces w ON w.id = c.workspace_id AND w.is_personal = true
  JOIN channel_members cm ON cm.channel_id = c.id AND cm.member_type = 'user'
  JOIN users u ON u.id = cm.member_id
ON CONFLICT (workspace_id, user_id) DO NOTHING;

-- Every personal Workspace has a normal lobby. Every non-public Workspace has
-- one pinned Lucy Channel for its creator.
INSERT INTO channels (workspace_id, name, description, type, created_by)
SELECT w.id, 'general', 'Personal Workspace lobby', 'channel', w.created_by
  FROM workspaces w
 WHERE w.is_personal = true
   AND w.deleted_at IS NULL
   AND w.created_by IS NOT NULL
   AND NOT EXISTS (
       SELECT 1 FROM channels c
        WHERE c.workspace_id = w.id AND c.name = 'general'
          AND c.type = 'channel' AND c.is_archived = false
   );

INSERT INTO channels (workspace_id, name, description, type, created_by)
SELECT w.id, 'lucy', 'Your pinned Channel with Lucy, this Workspace''s steward.', 'lucy', w.created_by
  FROM workspaces w
 WHERE w.id <> '00000000-0000-0000-0000-000000000001'
   AND w.deleted_at IS NULL
   AND w.created_by IS NOT NULL
   AND NOT EXISTS (
       SELECT 1 FROM channels c
        WHERE c.workspace_id = w.id AND c.type = 'lucy' AND c.is_archived = false
   );

INSERT INTO channel_members (channel_id, member_type, member_id, role)
SELECT c.id, 'user', w.created_by, 'owner'
  FROM channels c
  JOIN workspaces w ON w.id = c.workspace_id
 WHERE w.created_by IS NOT NULL
   AND c.is_archived = false
   AND (c.type = 'lucy' OR (w.is_personal = true AND c.name = 'general' AND c.type = 'channel'))
ON CONFLICT DO NOTHING;

-- Lucy is unique inside an owner's Workspace, not across the whole account.
DROP INDEX IF EXISTS idx_agents_owner_lucy_active;

WITH duplicate_lucy_channels AS (
    SELECT c.id,
           row_number() OVER (
               PARTITION BY c.workspace_id, c.created_by
               ORDER BY EXISTS (
                   SELECT 1 FROM agents a
                    WHERE a.home_channel_id = c.id AND a.kind = 'lucy' AND a.is_active = true
               ) DESC,
               c.created_at ASC
           ) AS position
      FROM channels c
     WHERE c.type = 'lucy' AND c.is_archived = false
)
UPDATE agents a
   SET is_active = false, updated_at = now()
  FROM duplicate_lucy_channels duplicate
 WHERE duplicate.position > 1 AND a.home_channel_id = duplicate.id AND a.kind = 'lucy';

WITH duplicate_lucy_channels AS (
    SELECT c.id,
           row_number() OVER (
               PARTITION BY c.workspace_id, c.created_by
               ORDER BY EXISTS (
                   SELECT 1 FROM agents a
                    WHERE a.home_channel_id = c.id AND a.kind = 'lucy' AND a.is_active = true
               ) DESC,
               c.created_at ASC
           ) AS position
      FROM channels c
     WHERE c.type = 'lucy' AND c.is_archived = false
)
UPDATE channels c
   SET is_archived = true, updated_at = now()
  FROM duplicate_lucy_channels duplicate
 WHERE duplicate.position > 1 AND c.id = duplicate.id;

CREATE UNIQUE INDEX idx_channels_workspace_owner_lucy_active
    ON channels(workspace_id, created_by)
    WHERE type = 'lucy' AND is_archived = false;

CREATE UNIQUE INDEX idx_agents_owner_lucy_home_active
    ON agents(owner_id, home_channel_id)
    WHERE is_active = true AND kind = 'lucy';

-- Existing configured owners get a Lucy in every existing private Workspace.
-- The runtime binding is copied, but remains a user-global Computer resource.
INSERT INTO agents (
    name, description, owner_id, model_provider, model_name, system_prompt,
    runtime_id, custom_env, custom_args, avatar_url, home_channel_id, kind
)
SELECT 'Lucy',
       'Workspace steward — helps you create and manage Agent teams.',
       w.created_by,
       source.model_provider,
       source.model_name,
       source.system_prompt,
       source.runtime_id,
       source.custom_env,
       source.custom_args,
       'dicebear:pixel-art:lucy',
       lucy_channel.id,
       'lucy'
  FROM workspaces w
  JOIN channels lucy_channel
    ON lucy_channel.workspace_id = w.id
   AND lucy_channel.type = 'lucy'
   AND lucy_channel.is_archived = false
  JOIN LATERAL (
       SELECT a.model_provider, a.model_name, a.system_prompt, a.runtime_id,
              a.custom_env, a.custom_args
         FROM agents a
        WHERE a.owner_id = w.created_by
          AND a.kind = 'lucy'
          AND a.is_active = true
          AND a.runtime_id IS NOT NULL
        ORDER BY a.created_at ASC
        LIMIT 1
  ) source ON true
 WHERE w.id <> '00000000-0000-0000-0000-000000000001'
   AND w.deleted_at IS NULL
   AND NOT EXISTS (
       SELECT 1 FROM agents existing
        WHERE existing.owner_id = w.created_by
          AND existing.home_channel_id = lucy_channel.id
          AND existing.kind = 'lucy'
          AND existing.is_active = true
   );

INSERT INTO channel_members (channel_id, member_type, member_id, role)
SELECT a.home_channel_id, 'agent', a.id, 'member'
  FROM agents a
 WHERE a.kind = 'lucy' AND a.is_active = true AND a.home_channel_id IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO messages (channel_id, sender_type, sender_id, content, content_type)
SELECT c.id, 'system', '00000000-0000-0000-0000-000000000000',
       'Lucy belongs to this Workspace. Choose a Computer and runtime below to finish setting her up.',
       'system'
  FROM channels c
 WHERE c.type = 'lucy'
   AND c.is_archived = false
   AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.home_channel_id = c.id AND a.kind = 'lucy' AND a.is_active = true)
   AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.channel_id = c.id);
