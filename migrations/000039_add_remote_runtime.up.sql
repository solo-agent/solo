ALTER TABLE computers
    ADD COLUMN credential_hash TEXT,
    ADD COLUMN credential_created_at TIMESTAMPTZ,
    ADD COLUMN credential_revoked_at TIMESTAMPTZ,
    ADD COLUMN enrollment_token_hash TEXT,
    ADD COLUMN enrollment_expires_at TIMESTAMPTZ,
    ADD COLUMN enrollment_used_at TIMESTAMPTZ,
    ADD COLUMN protocol_version INTEGER,
    ADD COLUMN daemon_version TEXT,
    ADD COLUMN runtime_inventory JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN last_connected_at TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_computers_credential_hash
    ON computers(credential_hash)
    WHERE credential_hash IS NOT NULL;

ALTER TABLE agent_runs
    ADD COLUMN computer_id UUID REFERENCES computers(id) ON DELETE SET NULL,
    ADD COLUMN dispatch_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN delivery_expires_at TIMESTAMPTZ,
    ADD COLUMN execution_attempt_id UUID,
    ADD COLUMN accepted_at TIMESTAMPTZ,
    ADD COLUMN retry_of_run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    ADD COLUMN delivery_count INTEGER NOT NULL DEFAULT 0;

UPDATE agent_runs r
   SET computer_id = c.id
  FROM agents a
  JOIN computers c ON c.id::text = a.runtime_id
 WHERE r.agent_id = a.id
   AND r.computer_id IS NULL;

CREATE INDEX idx_agent_runs_computer_delivery
    ON agent_runs(computer_id, status, started_at)
    WHERE finished_at IS NULL;

CREATE INDEX idx_agent_runs_agent_accepted
    ON agent_runs(agent_id, accepted_at)
    WHERE finished_at IS NULL AND execution_attempt_id IS NOT NULL;

ALTER TABLE agent_run_events
    ADD COLUMN attempt_id UUID,
    ADD COLUMN source_seq BIGINT;

CREATE UNIQUE INDEX idx_agent_run_events_attempt_source
    ON agent_run_events(run_id, attempt_id, source_seq)
    WHERE attempt_id IS NOT NULL AND source_seq IS NOT NULL;
