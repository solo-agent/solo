CREATE TABLE task_dependencies (
    blocker_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_task_id, blocked_task_id),
    CHECK (blocker_task_id != blocked_task_id)
);

CREATE INDEX idx_deps_blocked ON task_dependencies(blocked_task_id);
CREATE INDEX idx_deps_blocker ON task_dependencies(blocker_task_id);
