CREATE TABLE team_formations (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_by_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    source_channel_id     UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    source_message_id     UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    target_channel_id     UUID REFERENCES channels(id) ON DELETE SET NULL,
    status                VARCHAR(20) NOT NULL DEFAULT 'provisioning'
                          CHECK (status IN ('provisioning', 'completed', 'failed')),
    plan                  JSONB NOT NULL,
    result                JSONB,
    error                 TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_message_id)
);

CREATE INDEX idx_team_formations_owner_created
    ON team_formations(owner_id, created_at DESC);

CREATE INDEX idx_team_formations_status
    ON team_formations(status, updated_at);
