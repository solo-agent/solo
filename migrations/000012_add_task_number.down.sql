-- 000012_add_task_number.down.sql

-- Restore cancelled status from closed (only for rows that were originally cancelled)
-- Note: this is a best-effort reversal; new closed tasks created after migration will be affected.
UPDATE tasks SET status = 'cancelled' WHERE status = 'closed';

DROP INDEX IF EXISTS idx_tasks_channel_status;
DROP INDEX IF EXISTS idx_tasks_task_number;

ALTER TABLE tasks DROP COLUMN task_number;
