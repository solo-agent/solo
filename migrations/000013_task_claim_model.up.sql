-- 000013_task_claim_model.up.sql
-- Phase 1: Claim model — replace assignee with claimer, add message_id, per-channel numbering.

-- Step 1: Add claimer_id column (replaces assignee_id/assignee_type)
ALTER TABLE tasks ADD COLUMN claimer_id UUID;

-- Step 2: Migrate existing assignee data to claimer_id
UPDATE tasks SET claimer_id = assignee_id WHERE assignee_id IS NOT NULL;

-- Step 3: Drop old assignee columns and indexes
DROP INDEX IF EXISTS idx_tasks_assignee;
ALTER TABLE tasks DROP COLUMN IF EXISTS assignee_id;
ALTER TABLE tasks DROP COLUMN IF EXISTS assignee_type;

-- Step 4: Add message_id for asTask (message -> task conversion)
ALTER TABLE tasks ADD COLUMN message_id UUID REFERENCES messages(id) ON DELETE SET NULL;

-- Step 5: Per-channel numbering — drop global serial default, add unique constraint
ALTER TABLE tasks ALTER COLUMN task_number DROP DEFAULT;
ALTER TABLE tasks ADD CONSTRAINT unique_channel_task_number UNIQUE(channel_id, task_number);

-- Step 6: Index for claimer lookups
CREATE INDEX idx_tasks_claimer ON tasks(claimer_id);
CREATE INDEX idx_tasks_message ON tasks(message_id);
