DROP TRIGGER IF EXISTS trg_enforce_relationship_channel_scope ON agent_relationships;
DROP FUNCTION IF EXISTS enforce_relationship_channel_scope();

DROP TRIGGER IF EXISTS trg_enforce_agent_channel_membership ON channel_members;
DROP FUNCTION IF EXISTS enforce_agent_channel_membership();

DROP INDEX IF EXISTS idx_channels_source_template;
DROP INDEX IF EXISTS idx_agents_home_channel_active;
DROP INDEX IF EXISTS idx_agents_owner_lucy_active;
DROP INDEX IF EXISTS idx_agents_home_channel_name_active;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_owner_name_active
    ON agents(owner_id, name)
    WHERE is_active = true;

UPDATE channels SET type = 'channel' WHERE type = 'lucy';

ALTER TABLE agent_templates DROP COLUMN relationships;
ALTER TABLE messages DROP COLUMN metadata;
ALTER TABLE agents DROP COLUMN kind, DROP COLUMN home_channel_id;
ALTER TABLE channels DROP COLUMN source_template_id;
