CREATE TABLE agent_relationships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    rel_type      VARCHAR(32) NOT NULL CHECK (rel_type IN ('assigns_to', 'collaborates_with')),
    weight        DOUBLE PRECISION NOT NULL DEFAULT 1 CHECK (weight >= 0 AND weight <= 10),
    instruction   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_agent_id <> to_agent_id)
);

CREATE UNIQUE INDEX idx_agent_relationships_assigns_to
    ON agent_relationships(from_agent_id, to_agent_id)
    WHERE rel_type = 'assigns_to';

CREATE UNIQUE INDEX idx_agent_relationships_collaborates_with
    ON agent_relationships(LEAST(from_agent_id, to_agent_id), GREATEST(from_agent_id, to_agent_id))
    WHERE rel_type = 'collaborates_with';

CREATE INDEX idx_agent_relationships_from ON agent_relationships(from_agent_id);
CREATE INDEX idx_agent_relationships_to ON agent_relationships(to_agent_id);
