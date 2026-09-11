-- Fail closed if rollback would merge channel-specific relationships or remove live participation.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' JOIN channels home ON home.id=a.home_channel_id JOIN channels target ON target.id=m.channel_id WHERE target.type<>'dm' AND home.workspace_id<>target.workspace_id) THEN RAISE EXCEPTION 'Withdraw cross-workspace Agents before rollback'; END IF;
 IF EXISTS(SELECT 1 FROM agent_relationships r JOIN agents a ON a.id=r.from_agent_id WHERE r.channel_id<>a.home_channel_id) THEN RAISE EXCEPTION 'Withdraw shared relationship agreements before rollback'; END IF;
END $$;
CREATE OR REPLACE FUNCTION enforce_relationship_channel_scope()
RETURNS trigger AS $$
DECLARE
    from_home UUID;
    to_home UUID;
BEGIN
    SELECT home_channel_id INTO from_home FROM agents WHERE id = NEW.from_agent_id;
    SELECT home_channel_id INTO to_home FROM agents WHERE id = NEW.to_agent_id;

    IF from_home IS NULL OR to_home IS NULL OR from_home <> to_home THEN
        RAISE EXCEPTION 'agent relationships must stay inside one home channel';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


DROP TRIGGER agent_participation_revoked ON channel_members;
DROP FUNCTION solo_revoke_agent_participation();
ALTER TABLE channel_team_versions DROP COLUMN approved_owner_ids;
DROP TABLE agent_relationship_proposals;
DROP TRIGGER agent_relationship_scope ON agent_relationships;
DROP FUNCTION solo_relationship_scope();
DROP VIEW IF EXISTS agent_relationship_scopes;
DROP INDEX idx_agent_relationships_assigns_to;
DROP INDEX idx_agent_relationships_collaborates_with;
DROP TRIGGER trg_enforce_relationship_channel_scope ON agent_relationships;
ALTER TABLE agent_relationships DROP COLUMN channel_id;
CREATE TRIGGER trg_enforce_relationship_channel_scope BEFORE INSERT OR UPDATE OF from_agent_id,to_agent_id ON agent_relationships FOR EACH ROW EXECUTE FUNCTION enforce_relationship_channel_scope();
CREATE UNIQUE INDEX idx_agent_relationships_assigns_to ON agent_relationships(from_agent_id,to_agent_id) WHERE rel_type='assigns_to';
CREATE UNIQUE INDEX idx_agent_relationships_collaborates_with ON agent_relationships(LEAST(from_agent_id,to_agent_id),GREATEST(from_agent_id,to_agent_id)) WHERE rel_type='collaborates_with';
CREATE OR REPLACE FUNCTION enforce_agent_channel_membership()
RETURNS trigger AS $$
DECLARE
    agent_kind VARCHAR(20);
    agent_home UUID;
    home_workspace UUID;
    channel_kind VARCHAR(20);
    target_workspace UUID;
BEGIN
    IF NEW.member_type <> 'agent' THEN
        RETURN NEW;
    END IF;

    SELECT a.kind, a.home_channel_id, home.workspace_id
      INTO agent_kind, agent_home, home_workspace
      FROM agents a
      LEFT JOIN channels home ON home.id = a.home_channel_id
     WHERE a.id = NEW.member_id;

    SELECT type, workspace_id
      INTO channel_kind, target_workspace
      FROM channels
     WHERE id = NEW.channel_id;

    IF channel_kind = 'dm' THEN
        RETURN NEW;
    END IF;

    IF agent_kind = 'lucy' THEN
        IF agent_home IS DISTINCT FROM NEW.channel_id OR channel_kind <> 'lucy' THEN
            RAISE EXCEPTION 'Lucy may only join her Lucy channel or a DM';
        END IF;
        RETURN NEW;
    END IF;

    IF channel_kind <> 'channel' THEN
        RAISE EXCEPTION 'ordinary agents may only join workspace channels or a DM';
    END IF;

    IF agent_home IS NULL OR home_workspace IS DISTINCT FROM target_workspace THEN
        RAISE EXCEPTION 'agent % belongs to another workspace', NEW.member_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
