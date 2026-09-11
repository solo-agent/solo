CREATE TABLE agent_selections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 agent_id uuid NOT NULL REFERENCES agents(id),
 baseline_revision_id uuid NOT NULL REFERENCES agent_revisions(id),
 candidate_revision_id uuid NOT NULL REFERENCES agent_revisions(id),
 receiver_revision_id uuid NOT NULL REFERENCES agent_revisions(id),
 created_by uuid NOT NULL REFERENCES users(id),
 plan jsonb NOT NULL,
 idempotency_key text NOT NULL,
 request_hash text NOT NULL,
 status text NOT NULL DEFAULT 'running' CHECK(status IN ('running','accepted','rejected','observe')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(agent_id,idempotency_key)
);
CREATE TABLE agent_selection_trials (
 selection_id uuid NOT NULL REFERENCES agent_selections(id),
 arm text NOT NULL CHECK(arm IN ('baseline','candidate','receiver')),
 agent_id uuid NOT NULL UNIQUE REFERENCES agents(id),
 channel_id uuid NOT NULL UNIQUE REFERENCES channels(id),
 revision_id uuid NOT NULL REFERENCES agent_revisions(id),
 token_budget bigint NOT NULL CHECK(token_budget>0),
 PRIMARY KEY(selection_id,arm)
);
CREATE TABLE agent_selection_tasks (
 selection_id uuid NOT NULL,
 arm text NOT NULL,
 case_index integer NOT NULL CHECK(case_index>=0),
 task_id uuid NOT NULL UNIQUE REFERENCES tasks(id),
 run_id uuid REFERENCES agent_runs(id),
 input_submission_id uuid REFERENCES task_submissions(id),
 dispatched_version bigint NOT NULL DEFAULT 0,
 attempts integer NOT NULL DEFAULT 0,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '',
 PRIMARY KEY(selection_id,arm,case_index),
 FOREIGN KEY(selection_id,arm) REFERENCES agent_selection_trials(selection_id,arm)
);
CREATE TABLE agent_selection_decisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 selection_id uuid NOT NULL REFERENCES agent_selections(id),
 decision text NOT NULL CHECK(decision IN ('accepted','rejected','observe')),
 reason text NOT NULL,
 evidence jsonb NOT NULL,
 created_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION solo_selection_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='agent_selections' AND TG_OP='UPDATE' AND (to_jsonb(NEW)-'status')=(to_jsonb(OLD)-'status') THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'Selection plans and decisions are immutable';
END $$;
CREATE TRIGGER selection_plan_immutable BEFORE UPDATE ON agent_selections FOR EACH ROW EXECUTE FUNCTION solo_selection_immutable();
CREATE TRIGGER selection_trial_immutable BEFORE UPDATE ON agent_selection_trials FOR EACH ROW EXECUTE FUNCTION solo_selection_immutable();
CREATE TRIGGER selection_decision_immutable BEFORE UPDATE ON agent_selection_decisions FOR EACH ROW EXECUTE FUNCTION solo_selection_immutable();
CREATE FUNCTION solo_selection_task_fence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE selection uuid;
BEGIN
 SELECT selection_id INTO selection FROM agent_selection_tasks WHERE task_id=OLD.id;
 IF selection IS NULL THEN RETURN NEW; END IF;
 IF NEW.title IS DISTINCT FROM OLD.title OR NEW.description IS DISTINCT FROM OLD.description OR NEW.contract IS DISTINCT FROM OLD.contract OR (NEW.claimer_id IS DISTINCT FROM OLD.claimer_id AND NOT (NEW.claimer_id IS NULL AND NOT EXISTS(SELECT 1 FROM agents WHERE id=OLD.claimer_id AND is_active))) OR NEW.channel_id IS DISTINCT FROM OLD.channel_id OR NEW.parent_task_id IS DISTINCT FROM OLD.parent_task_id THEN
  RAISE EXCEPTION 'Selection inputs, criteria and assignee are fixed; create a new Selection';
 END IF;
 UPDATE agent_selections SET status='observe' WHERE id=selection AND status='accepted';
 RETURN NEW;
END $$;
CREATE TRIGGER selection_task_fence BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION solo_selection_task_fence();
