-- A channel membership is the owner's explicit participation grant.
CREATE OR REPLACE FUNCTION enforce_agent_channel_membership() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE agent_kind text; agent_home uuid; owner uuid; channel_kind text; target_workspace uuid; home_workspace uuid;
BEGIN
 IF NEW.member_type<>'agent' THEN RETURN NEW; END IF;
 SELECT kind,home_channel_id,owner_id INTO agent_kind,agent_home,owner FROM agents WHERE id=NEW.member_id;
 SELECT type,workspace_id INTO channel_kind,target_workspace FROM channels WHERE id=NEW.channel_id;
 IF channel_kind='dm' THEN RETURN NEW; END IF;
 IF agent_kind='lucy' THEN
  IF agent_home IS DISTINCT FROM NEW.channel_id OR channel_kind<>'lucy' THEN RAISE EXCEPTION 'Lucy may only join her Lucy channel or a DM'; END IF;
  RETURN NEW;
 END IF;
 IF channel_kind<>'channel' OR agent_home IS NULL THEN RAISE EXCEPTION 'ordinary agents require a home and workspace channel'; END IF;
 SELECT workspace_id INTO home_workspace FROM channels WHERE id=agent_home;
 IF home_workspace=target_workspace THEN RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=target_workspace AND user_id=owner) OR NOT EXISTS(SELECT 1 FROM channel_members WHERE channel_id=NEW.channel_id AND member_type='user' AND member_id=owner) THEN RAISE EXCEPTION 'Agent owner must be a member of the target workspace and channel'; END IF;
 RETURN NEW;
END $$;

ALTER TABLE agent_relationships ADD COLUMN channel_id uuid REFERENCES channels(id) ON DELETE CASCADE;
UPDATE agent_relationships r SET channel_id=a.home_channel_id FROM agents a WHERE a.id=r.from_agent_id;
CREATE FUNCTION solo_relationship_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.channel_id IS NULL THEN SELECT home_channel_id INTO NEW.channel_id FROM agents WHERE id=NEW.from_agent_id; END IF;
 IF NEW.channel_id IS NULL THEN RAISE EXCEPTION 'relationship channel is required'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_relationship_scope BEFORE INSERT ON agent_relationships FOR EACH ROW EXECUTE FUNCTION solo_relationship_scope();
-- Unscoped historical relationships stay intact; new rows require a channel via the trigger.
DROP INDEX idx_agent_relationships_assigns_to;
DROP INDEX idx_agent_relationships_collaborates_with;
CREATE UNIQUE INDEX idx_agent_relationships_assigns_to ON agent_relationships(channel_id,from_agent_id,to_agent_id) WHERE rel_type='assigns_to';
CREATE UNIQUE INDEX idx_agent_relationships_collaborates_with ON agent_relationships(channel_id,LEAST(from_agent_id,to_agent_id),GREATEST(from_agent_id,to_agent_id)) WHERE rel_type='collaborates_with';
CREATE TABLE agent_relationship_proposals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
 from_agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,to_agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
 rel_type text NOT NULL CHECK(rel_type IN ('assigns_to','collaborates_with')), instruction text NOT NULL,
 proposed_by uuid NOT NULL REFERENCES users(id),approved_owner_ids uuid[] NOT NULL DEFAULT '{}',
 relationship_id uuid REFERENCES agent_relationships(id) ON DELETE SET NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','withdrawn')),
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE channel_team_versions ADD COLUMN approved_owner_ids uuid[] NOT NULL DEFAULT '{}';
UPDATE channel_team_versions SET approved_owner_ids=ARRAY[created_by];

CREATE FUNCTION solo_revoke_agent_participation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.member_type='agent' THEN
  DELETE FROM agent_pending_message_wakes WHERE channel_id=OLD.channel_id AND agent_id=OLD.member_id;
  UPDATE agent_work_marks SET status='cancelled',resolution='Channel participation revoked',updated_at=now() WHERE channel_id=OLD.channel_id AND agent_id=OLD.member_id AND status='open';
  UPDATE channels SET team_version_id=NULL WHERE id=OLD.channel_id;
  DELETE FROM agent_relationships WHERE channel_id=OLD.channel_id AND (from_agent_id=OLD.member_id OR to_agent_id=OLD.member_id);
  UPDATE agent_relationship_proposals SET status='withdrawn' WHERE channel_id=OLD.channel_id AND (from_agent_id=OLD.member_id OR to_agent_id=OLD.member_id);
 END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER agent_participation_revoked AFTER DELETE ON channel_members FOR EACH ROW EXECUTE FUNCTION solo_revoke_agent_participation();
-- Evaluation membership may disappear without changing its captured configuration.
CREATE OR REPLACE FUNCTION solo_immutable_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.config IS DISTINCT FROM OLD.config OR NEW.agent_id IS DISTINCT FROM OLD.agent_id OR NEW.config_hash IS DISTINCT FROM OLD.config_hash THEN RAISE EXCEPTION 'Agent revision snapshots are immutable'; END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION enforce_relationship_channel_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE from_home uuid; to_home uuid; from_owner uuid; to_owner uuid;
BEGIN
 SELECT home_channel_id,owner_id INTO from_home,from_owner FROM agents WHERE id=NEW.from_agent_id;
 SELECT home_channel_id,owner_id INTO to_home,to_owner FROM agents WHERE id=NEW.to_agent_id;
 IF from_home=to_home AND NEW.channel_id=from_home THEN RETURN NEW; END IF;
 IF (SELECT count(*) FROM channel_members WHERE channel_id=NEW.channel_id AND member_type='agent' AND member_id IN (NEW.from_agent_id,NEW.to_agent_id))<>2 THEN RAISE EXCEPTION 'Both Agents must participate in the relationship channel'; END IF;
 IF NOT EXISTS(SELECT 1 FROM agent_relationship_proposals p WHERE p.channel_id=NEW.channel_id AND p.from_agent_id=NEW.from_agent_id AND p.to_agent_id=NEW.to_agent_id AND p.rel_type=NEW.rel_type AND p.instruction=NEW.instruction AND p.status IN ('pending','accepted') AND from_owner=ANY(p.approved_owner_ids) AND to_owner=ANY(p.approved_owner_ids)) THEN RAISE EXCEPTION 'Both owners must approve this exact shared agreement'; END IF;
 RETURN NEW;
END $$;

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
