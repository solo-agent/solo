DROP INDEX IF EXISTS idx_agent_runs_rollover_from_session;

ALTER TABLE agent_runs DROP COLUMN IF EXISTS rollover_from_session_id;
