ALTER TABLE channels ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}';
COMMENT ON COLUMN channels.config IS 'Channel-level configuration. Keys: max_swarm_decomposes_per_day (int, default 5), default_watchdog_deadline_hours (int, default 48)';
