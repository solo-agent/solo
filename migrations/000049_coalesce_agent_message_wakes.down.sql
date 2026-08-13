ALTER TABLE agent_runs
    DROP CONSTRAINT IF EXISTS chk_agent_run_wake_range,
    DROP COLUMN IF EXISTS wake_requires_visible_result,
    DROP COLUMN IF EXISTS wake_latest_message_seq,
    DROP COLUMN IF EXISTS wake_first_message_seq;

DROP TABLE IF EXISTS agent_pending_message_wakes;
DROP TABLE IF EXISTS agent_message_wake_slots;
