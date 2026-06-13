CREATE TABLE agent_relationships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    rel_type      VARCHAR(20) NOT NULL CHECK (rel_type IN ('reports_to', 'delegates_to', 'collaborates_with', 'escalates_to')),
    channel_id    UUID REFERENCES channels(id) ON DELETE CASCADE,
    weight        REAL NOT NULL DEFAULT 1.0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE agent_relationships ADD CONSTRAINT chk_no_self_relationship
    CHECK (from_agent_id != to_agent_id);

CREATE UNIQUE INDEX idx_rel_global
    ON agent_relationships(from_agent_id, to_agent_id, rel_type)
    WHERE rel_type IN ('reports_to', 'escalates_to');

CREATE UNIQUE INDEX idx_rel_channel
    ON agent_relationships(from_agent_id, to_agent_id, rel_type, channel_id)
    WHERE rel_type IN ('delegates_to', 'collaborates_with');

CREATE UNIQUE INDEX idx_collab_bidirectional
    ON agent_relationships(LEAST(from_agent_id, to_agent_id), GREATEST(from_agent_id, to_agent_id), rel_type, channel_id)
    WHERE rel_type = 'collaborates_with';

CREATE INDEX idx_rel_from ON agent_relationships(from_agent_id);
CREATE INDEX idx_rel_to ON agent_relationships(to_agent_id);
CREATE INDEX idx_rel_channel_id ON agent_relationships(channel_id) WHERE channel_id IS NOT NULL;
