DROP TABLE IF EXISTS workspace_embed_channels;
DROP TABLE IF EXISTS workspace_embed_policies;
DROP TABLE IF EXISTS workspace_join_rules;
DROP TABLE IF EXISTS workspace_invitations;

DELETE FROM agents
WHERE home_channel_id IN (
    SELECT id FROM channels
    WHERE workspace_id <> '00000000-0000-0000-0000-000000000001'
);

DELETE FROM channels
WHERE workspace_id <> '00000000-0000-0000-0000-000000000001';

DROP INDEX IF EXISTS idx_channels_workspace_created;
DROP INDEX IF EXISTS idx_channels_workspace_active_name;
CREATE UNIQUE INDEX idx_channels_active_name
    ON channels(name) WHERE type = 'channel' AND is_archived = false;

ALTER TABLE channels DROP COLUMN IF EXISTS workspace_id;
DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS workspaces;
