CREATE TABLE agents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model_provider  VARCHAR(50) NOT NULL DEFAULT 'anthropic',
    model_name      VARCHAR(100) NOT NULL,
    system_prompt   TEXT NOT NULL DEFAULT '',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    avatar_url      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_agents_owner_name_active ON agents(owner_id, name) WHERE is_active = true;
CREATE INDEX idx_agents_owner ON agents(owner_id);
CREATE INDEX idx_agents_owner_active ON agents(owner_id, is_active);
