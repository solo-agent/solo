DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM agent_selection_trials tr JOIN agents a ON a.id=tr.agent_id WHERE a.is_active)
 OR EXISTS(SELECT 1 FROM agent_selection_trials tr JOIN agent_runs r ON r.agent_id=tr.agent_id WHERE r.finished_at IS NULL) THEN
  RAISE EXCEPTION 'Retire Selection trial Agents and finish their Runs before rollback';
 END IF;
END $$;
DROP TRIGGER selection_task_fence ON tasks;
DROP FUNCTION solo_selection_task_fence();
DROP TABLE agent_selection_decisions;
DROP TABLE agent_selection_tasks;
DROP TABLE agent_selection_trials;
DROP TABLE agent_selections;
DROP FUNCTION solo_selection_immutable();
