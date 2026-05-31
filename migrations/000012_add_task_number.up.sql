-- 000012_add_task_number.up.sql
-- P0: Add task_number SERIAL column and replace cancelled with closed

ALTER TABLE tasks ADD COLUMN task_number SERIAL;

-- Update existing cancelled tasks to closed status
UPDATE tasks SET status = 'closed' WHERE status = 'cancelled';

-- Index for number-based lookups
CREATE INDEX idx_tasks_task_number ON tasks(task_number);

-- Index for board view queries (channel + status)
CREATE INDEX idx_tasks_channel_status ON tasks(channel_id, status);
