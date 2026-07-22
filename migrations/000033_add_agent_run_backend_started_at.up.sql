ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS backend_started_at TIMESTAMPTZ;
