CREATE TABLE computers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    daemon_id VARCHAR(100),
    daemon_url VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'offline',
    last_heartbeat TIMESTAMPTZ,
    agent_ids UUID[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_computers_owner_id ON computers(owner_id);
CREATE UNIQUE INDEX idx_computers_daemon_id_unique ON computers(daemon_id) WHERE daemon_id IS NOT NULL;
CREATE INDEX idx_computers_status ON computers(status);
