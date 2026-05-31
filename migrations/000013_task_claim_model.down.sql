-- 000013_task_claim_model.down.sql
-- Revert Phase 1 changes.

DROP INDEX IF EXISTS idx_tasks_message;
DROP INDEX IF EXISTS idx_tasks_claimer;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS unique_channel_task_number;

ALTER TABLE tasks DROP COLUMN IF EXISTS message_id;

ALTER TABLE tasks ADD COLUMN assignee_id UUID;
ALTER TABLE tasks ADD COLUMN assignee_type TEXT;
UPDATE tasks SET assignee_id = claimer_id WHERE claimer_id IS NOT NULL;
ALTER TABLE tasks DROP COLUMN claimer_id;

CREATE INDEX idx_tasks_assignee ON tasks(assignee_type, assignee_id);

-- Restore SERIAL default (the sequence still exists, we just reattach it)
ALTER TABLE tasks ALTER COLUMN task_number SET DEFAULT nextval('tasks_task_number_seq');
