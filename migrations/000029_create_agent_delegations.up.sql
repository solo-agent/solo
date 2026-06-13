CREATE TABLE agent_delegations (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id       UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id           UUID REFERENCES tasks(id) ON DELETE SET NULL,
    channel_id        UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    status            VARCHAR(20) NOT NULL DEFAULT 'queued'
                      CHECK (status IN ('queued', 'delivered', 'started', 'completed', 'failed', 'rejected')),
    message           TEXT,
    start_if_inactive BOOLEAN NOT NULL DEFAULT false,
    rejection_reason  TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_delegations_from ON agent_delegations(from_agent_id, status);
CREATE INDEX idx_delegations_to ON agent_delegations(to_agent_id, status);
CREATE INDEX idx_delegations_channel ON agent_delegations(channel_id, status);
