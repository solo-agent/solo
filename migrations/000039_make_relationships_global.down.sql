DROP INDEX IF EXISTS idx_collab_bidirectional_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_collab_bidirectional_unique
    ON agent_relationships(LEAST(from_agent_id, to_agent_id), GREATEST(from_agent_id, to_agent_id), rel_type, channel_id)
    WHERE rel_type = 'collaborates_with';
