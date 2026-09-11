CREATE TABLE task_waits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    task_version BIGINT NOT NULL,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    condition JSONB NOT NULL,
    handoff JSONB NOT NULL,
    next_action TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting','resumed','cancelled')),
    created_by UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    fulfillment JSONB NOT NULL DEFAULT '{}',
    resolution_reason TEXT NOT NULL DEFAULT '',
    run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    UNIQUE(task_id,idempotency_key)
);
CREATE UNIQUE INDEX task_waits_one_active ON task_waits(task_id) WHERE status='waiting';
CREATE INDEX task_waits_pending ON task_waits(next_attempt_at) WHERE status='waiting';

CREATE FUNCTION invalidate_task_wait() RETURNS trigger AS $$
BEGIN
    IF NEW.version <> OLD.version OR NEW.status <> 'in_progress' THEN
        UPDATE task_waits SET status='cancelled',resolved_at=now(),resolution_reason='Task changed; register a new condition against its current version'
        WHERE task_id=NEW.id AND status='waiting';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER tasks_invalidate_wait AFTER UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION invalidate_task_wait();
