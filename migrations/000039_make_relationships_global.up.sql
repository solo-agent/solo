-- 000039: Make both relationship types global (no channel_id scoping).
-- Drops the collaborates_with unique index that included channel_id,
-- replacing it with a global bidirectional unique index.
DROP INDEX IF EXISTS idx_collab_bidirectional_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_collab_bidirectional_unique
    ON agent_relationships(LEAST(from_agent_id, to_agent_id), GREATEST(from_agent_id, to_agent_id), rel_type)
    WHERE rel_type = 'collaborates_with';
