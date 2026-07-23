DROP TABLE IF EXISTS agent_run_task_actions;
DROP INDEX IF EXISTS idx_messages_origin_run;
ALTER TABLE messages DROP COLUMN IF EXISTS origin_run_id;
