ALTER TABLE agent_runs
    ADD COLUMN rollover_from_session_id UUID
    REFERENCES agent_sessions(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_runs_rollover_from_session
    ON agent_runs(rollover_from_session_id)
    WHERE rollover_from_session_id IS NOT NULL;
