CREATE TABLE task_reviews (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id       UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    reviewer_id   UUID NOT NULL,
    decision      TEXT NOT NULL CHECK (decision IN ('accepted', 'rejected')),
    reason        TEXT NOT NULL DEFAULT '',
    artifact_id   UUID REFERENCES artifacts(id) ON DELETE SET NULL,
    next_owner_id UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_task_reviews_reviewer_created
    ON task_reviews(reviewer_id, created_at DESC);

CREATE INDEX idx_task_reviews_task_created
    ON task_reviews(task_id, created_at DESC);
