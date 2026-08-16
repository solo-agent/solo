ALTER TABLE workspace_invite_links
    ADD COLUMN target_channel_id UUID REFERENCES channels(id) ON DELETE SET NULL;

CREATE INDEX idx_workspace_invite_links_target_channel
    ON workspace_invite_links(target_channel_id)
    WHERE target_channel_id IS NOT NULL;
