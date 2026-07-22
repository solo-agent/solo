ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS origin_run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_messages_origin_run
    ON messages(origin_run_id, created_at ASC)
    WHERE origin_run_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS agent_run_task_actions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id     UUID NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    task_id    UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    actor_id   UUID NOT NULL,
    action     TEXT NOT NULL CHECK (action IN ('claim', 'unclaim', 'submit', 'accept', 'reject', 'close', 'reopen')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_run_task_actions_task_created
    ON agent_run_task_actions(task_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_agent_run_task_actions_run_created
    ON agent_run_task_actions(run_id, created_at ASC);
