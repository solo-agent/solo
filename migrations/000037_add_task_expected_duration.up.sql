-- Add expected_duration_minutes for per-task execution timeout override.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS expected_duration_minutes INTEGER NOT NULL DEFAULT 0;
COMMENT ON COLUMN tasks.expected_duration_minutes IS 'Custom execution timeout in minutes (0 = use default 6 min, max 120 min)';

-- Add lifecycle_mode to agents for declaring session lifecycle intent.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS lifecycle_mode TEXT NOT NULL DEFAULT 'episodic';
COMMENT ON COLUMN agents.lifecycle_mode IS 'Agent lifecycle: episodic (one-shot), continuous (wait for messages), or batch (process queue then exit)';
