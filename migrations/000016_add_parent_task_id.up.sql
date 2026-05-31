ALTER TABLE tasks ADD COLUMN parent_task_id UUID REFERENCES tasks(id) ON DELETE SET NULL;
CREATE INDEX idx_tasks_parent ON tasks(parent_task_id);
