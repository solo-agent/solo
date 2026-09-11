-- Also repair installations which already applied the original 68/70 migrations.
-- Do not infer a channel for historical relationships without a home.
ALTER TABLE agent_relationships ALTER COLUMN channel_id DROP NOT NULL;
DROP TRIGGER trg_enforce_relationship_channel_scope ON agent_relationships;
CREATE TRIGGER trg_enforce_relationship_channel_scope
 BEFORE INSERT OR UPDATE OF from_agent_id,to_agent_id,channel_id,rel_type,instruction ON agent_relationships
 FOR EACH ROW EXECUTE FUNCTION enforce_relationship_channel_scope();
DROP TRIGGER agent_relationship_scope ON agent_relationships;
CREATE TRIGGER agent_relationship_scope BEFORE INSERT OR UPDATE OF channel_id ON agent_relationships FOR EACH ROW EXECUTE FUNCTION solo_relationship_scope();

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

-- Legacy global relationships are projected only into channels both same-owner
-- Agents still participate in. No persistent channel is inferred or assigned.
-- An explicit scoped agreement overrides the old global relationship.
CREATE OR REPLACE VIEW agent_relationship_scopes AS
 SELECT id,from_agent_id,to_agent_id,rel_type,weight,instruction,created_at,updated_at,channel_id
 FROM agent_relationships WHERE channel_id IS NOT NULL
 UNION ALL
 SELECT r.id,r.from_agent_id,r.to_agent_id,r.rel_type,r.weight,r.instruction,r.created_at,r.updated_at,f.channel_id
 FROM agent_relationships r
 JOIN agents fa ON fa.id=r.from_agent_id
 JOIN agents ta ON ta.id=r.to_agent_id AND ta.owner_id=fa.owner_id
 JOIN channel_members f ON f.member_id=r.from_agent_id AND f.member_type='agent'
 JOIN channel_members t ON t.channel_id=f.channel_id AND t.member_id=r.to_agent_id AND t.member_type='agent'
 WHERE r.channel_id IS NULL AND NOT EXISTS(
  SELECT 1 FROM agent_relationships scoped WHERE scoped.channel_id=f.channel_id AND scoped.rel_type=r.rel_type
   AND ((scoped.from_agent_id=r.from_agent_id AND scoped.to_agent_id=r.to_agent_id)
    OR (r.rel_type='collaborates_with' AND scoped.from_agent_id=r.to_agent_id AND scoped.to_agent_id=r.from_agent_id))
 );
