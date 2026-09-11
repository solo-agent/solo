DROP TRIGGER tasks_version ON tasks;
DROP FUNCTION bump_task_version();
DROP TABLE task_review_deliveries;
ALTER TABLE tasks DROP CONSTRAINT tasks_current_submission_fk;
ALTER TABLE task_reviews DROP COLUMN submission_id, DROP COLUMN evidence, DROP COLUMN checks, DROP COLUMN request_hash, DROP COLUMN idempotency_key;
DROP TABLE task_submissions;
ALTER TABLE tasks DROP COLUMN version, DROP COLUMN contract, DROP COLUMN current_submission_id;
-- Keep needs_human audit rows when rolling back the application.
