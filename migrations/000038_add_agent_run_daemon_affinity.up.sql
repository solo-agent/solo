ALTER TABLE agent_runs ADD COLUMN daemon_id TEXT;

UPDATE agent_runs r
   SET daemon_id = c.daemon_id
  FROM agents a
  JOIN computers c ON c.id::text = a.runtime_id
 WHERE r.agent_id = a.id
   AND r.daemon_id IS NULL
   AND c.daemon_id IS NOT NULL;

UPDATE agent_runs
   SET daemon_id = (SELECT daemon_id FROM computers WHERE daemon_id IS NOT NULL LIMIT 1)
 WHERE daemon_id IS NULL
   AND (SELECT count(*) FROM computers WHERE daemon_id IS NOT NULL) = 1;

CREATE INDEX idx_agent_runs_daemon_active
    ON agent_runs(daemon_id, updated_at DESC)
    WHERE finished_at IS NULL;
