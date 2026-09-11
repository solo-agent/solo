ALTER TABLE tasks
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN contract JSONB,
    ADD COLUMN current_submission_id UUID;

CREATE TABLE task_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    submitted_by UUID NOT NULL,
    run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    task_version BIGINT NOT NULL,
    contract JSONB NOT NULL,
    handoff JSONB NOT NULL,
    artifact_version TEXT NOT NULL,
    evidence JSONB NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, idempotency_key),
    UNIQUE (task_id, task_version)
);
ALTER TABLE tasks ADD CONSTRAINT tasks_current_submission_fk
    FOREIGN KEY (current_submission_id) REFERENCES task_submissions(id) ON DELETE SET NULL;

ALTER TABLE task_reviews
    DROP CONSTRAINT task_reviews_decision_check,
    ADD CONSTRAINT task_reviews_decision_check CHECK (decision IN ('accepted', 'rejected', 'needs_human')),
    ADD COLUMN submission_id UUID REFERENCES task_submissions(id) ON DELETE CASCADE,
    ADD COLUMN evidence JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN checks JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN idempotency_key TEXT,
    ADD COLUMN request_hash TEXT;
CREATE UNIQUE INDEX task_reviews_submission_request ON task_reviews(submission_id, idempotency_key)
    WHERE submission_id IS NOT NULL;
CREATE UNIQUE INDEX task_reviews_submission_final ON task_reviews(submission_id)
    WHERE submission_id IS NOT NULL AND decision IN ('accepted', 'rejected');

CREATE TABLE task_review_deliveries (
    submission_id UUID PRIMARY KEY REFERENCES task_submissions(id) ON DELETE CASCADE,
    reviewer_id UUID NOT NULL,
    run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE FUNCTION bump_task_version() RETURNS trigger AS $$
BEGIN
    IF ROW(NEW.title, NEW.description, NEW.status, NEW.claimer_id, NEW.contract, NEW.priority, NEW.due_date)
       IS DISTINCT FROM ROW(OLD.title, OLD.description, OLD.status, OLD.claimer_id, OLD.contract, OLD.priority, OLD.due_date) THEN
        NEW.version := OLD.version + 1;
    ELSE
        NEW.version := OLD.version;
    END IF;
    IF NEW.contract IS NOT NULL AND OLD.status = 'in_review'
       AND ROW(NEW.title, NEW.description, NEW.contract, NEW.claimer_id, NEW.priority, NEW.due_date)
       IS DISTINCT FROM ROW(OLD.title, OLD.description, OLD.contract, OLD.claimer_id, OLD.priority, OLD.due_date) THEN
        NEW.status := CASE WHEN NEW.claimer_id IS NULL THEN 'todo' ELSE 'in_progress' END;
        NEW.current_submission_id := NULL;
    END IF;
    IF NEW.contract IS NOT NULL AND NEW.status = 'done' AND OLD.status <> 'done'
       AND NOT EXISTS (SELECT 1 FROM task_reviews r JOIN task_submissions s ON s.id=r.submission_id
                       WHERE r.submission_id=NEW.current_submission_id AND s.task_id=NEW.id
                         AND s.task_version + 1=OLD.version AND r.decision='accepted') THEN
        RAISE EXCEPTION 'contract task requires acceptance of its current submission';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER tasks_version BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION bump_task_version();
