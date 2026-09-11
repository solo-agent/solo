DROP VIEW task_delivery_measurements;
DROP TABLE task_observations;
DROP FUNCTION immutable_task_observation();
ALTER TABLE agent_run_token_usage DROP COLUMN budget_context;
