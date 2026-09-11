DROP TRIGGER IF EXISTS task_source_message ON tasks;
DROP FUNCTION IF EXISTS enforce_task_source_message();
DROP INDEX IF EXISTS unique_task_source_message;
ALTER TABLE tasks DROP COLUMN IF EXISTS legacy_message_source;
CREATE INDEX IF NOT EXISTS idx_tasks_message ON tasks(message_id);
