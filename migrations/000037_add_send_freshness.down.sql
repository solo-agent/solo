ALTER TABLE agent_runs
    DROP COLUMN freshness_held_at,
    DROP COLUMN freshness_seen_seq;

DROP INDEX idx_messages_scope_seq;
DROP INDEX idx_messages_seq;

ALTER TABLE messages DROP COLUMN seq;
