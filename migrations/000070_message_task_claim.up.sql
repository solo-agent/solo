-- Grandfather existing duplicate links without choosing or detaching a Task.
-- Only the migration can grant this exception; new links remain unique.
DROP TRIGGER IF EXISTS task_source_message ON tasks;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS legacy_message_source boolean NOT NULL DEFAULT false;
UPDATE tasks SET legacy_message_source=true
 WHERE message_id IN (SELECT message_id FROM tasks WHERE message_id IS NOT NULL GROUP BY message_id HAVING count(*)>1);
DROP INDEX IF EXISTS unique_task_source_message;
CREATE UNIQUE INDEX unique_task_source_message ON tasks(message_id)
 WHERE message_id IS NOT NULL AND NOT legacy_message_source;
CREATE INDEX IF NOT EXISTS idx_tasks_message ON tasks(message_id);

CREATE OR REPLACE FUNCTION enforce_task_source_message() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.message_id IS NOT DISTINCT FROM OLD.message_id THEN
  NEW.legacy_message_source := OLD.legacy_message_source;
  IF NEW.channel_id IS NOT DISTINCT FROM OLD.channel_id THEN RETURN NEW; END IF;
 ELSE
  NEW.legacy_message_source := false;
 END IF;
 IF NEW.message_id IS NOT NULL THEN
  IF NOT EXISTS(SELECT 1 FROM messages WHERE id=NEW.message_id AND channel_id=NEW.channel_id AND NOT COALESCE(is_deleted,false) AND thinking_node_id IS NULL) THEN
   RAISE EXCEPTION 'Task source must be a visible ordinary message in its channel';
  END IF;
  IF EXISTS(SELECT 1 FROM tasks WHERE message_id=NEW.message_id AND legacy_message_source AND id<>NEW.id) THEN
   RAISE EXCEPTION 'This message already has historical tasks; use a task number or UUID'
    USING ERRCODE='23505', CONSTRAINT='unique_task_source_message';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER task_source_message BEFORE INSERT OR UPDATE OF message_id,channel_id,legacy_message_source ON tasks FOR EACH ROW EXECUTE FUNCTION enforce_task_source_message();
