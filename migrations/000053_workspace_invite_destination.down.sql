DROP INDEX IF EXISTS idx_workspace_invite_links_target_channel;

ALTER TABLE workspace_invite_links
    DROP COLUMN IF EXISTS target_channel_id;
