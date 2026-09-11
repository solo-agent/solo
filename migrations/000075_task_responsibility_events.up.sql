CREATE TABLE task_responsibility_events (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 task_version BIGINT NOT NULL,
 actor_id UUID NOT NULL,
 assignee_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK (kind IN ('assigned','self_assigned','claimed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(task_id,task_version,assignee_id)
);
CREATE INDEX task_responsibility_assignee ON task_responsibility_events(assignee_id,created_at);
CREATE FUNCTION record_task_initial_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.claimer_id IS NOT NULL THEN
  INSERT INTO task_responsibility_events(task_id,task_version,actor_id,assignee_id,kind)
  VALUES(NEW.id,NEW.version,NEW.creator_id,NEW.claimer_id,CASE WHEN NEW.creator_id=NEW.claimer_id THEN 'self_assigned' ELSE 'assigned' END);
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER task_initial_assignment AFTER INSERT ON tasks FOR EACH ROW EXECUTE FUNCTION record_task_initial_assignment();
