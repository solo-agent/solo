CREATE TABLE agent_run_delivery_events (
    run_id UUID NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    attempt_id UUID NOT NULL,
    source_seq BIGINT NOT NULL,
    event TEXT NOT NULL,
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, attempt_id, source_seq)
);

CREATE INDEX idx_agent_run_delivery_events_task
    ON agent_run_delivery_events(task_id, source_seq);
