CREATE TABLE thought_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'in_progress',
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT thought_sessions_status_check
        CHECK (status IN ('todo', 'in_progress', 'in_review', 'done'))
);

CREATE TABLE thought_nodes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thought_id  UUID NOT NULL REFERENCES thought_sessions(id) ON DELETE CASCADE,
    parent_id   UUID REFERENCES thought_nodes(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'todo',
    is_root     BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT thought_nodes_status_check
        CHECK (status IN ('todo', 'in_progress', 'in_review', 'done'))
);

CREATE INDEX idx_thought_sessions_channel ON thought_sessions(channel_id, created_at DESC);
CREATE INDEX idx_thought_sessions_status ON thought_sessions(channel_id, status, created_at DESC);
CREATE INDEX idx_thought_nodes_thought ON thought_nodes(thought_id, created_at ASC);
CREATE INDEX idx_thought_nodes_parent ON thought_nodes(parent_id);
CREATE UNIQUE INDEX idx_thought_nodes_one_root ON thought_nodes(thought_id) WHERE is_root;
