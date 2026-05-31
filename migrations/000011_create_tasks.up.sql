CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID REFERENCES channels(id),
    creator_id UUID NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT DEFAULT 'todo',
    assignee_id UUID,
    assignee_type TEXT,
    priority TEXT DEFAULT 'none',
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_tasks_channel_id ON tasks(channel_id);
CREATE INDEX idx_tasks_assignee ON tasks(assignee_type, assignee_id);
CREATE INDEX idx_tasks_status ON tasks(status);
