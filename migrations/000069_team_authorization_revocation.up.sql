CREATE FUNCTION solo_revoke_team_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.member_type='agent' THEN
  UPDATE channel_team_versions SET approved_owner_ids=array_remove(approved_owner_ids,(SELECT owner_id FROM agents WHERE id=OLD.member_id)) WHERE channel_id=OLD.channel_id;
 END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER team_confirmation_revoked AFTER DELETE ON channel_members FOR EACH ROW EXECUTE FUNCTION solo_revoke_team_confirmation();

CREATE FUNCTION solo_revoke_team_relationship() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE channels SET team_version_id=NULL WHERE id=OLD.channel_id;
 UPDATE agent_relationship_proposals SET status='withdrawn' WHERE relationship_id=OLD.id;
 RETURN OLD;
END $$;
CREATE TRIGGER team_relationship_revoked BEFORE DELETE ON agent_relationships FOR EACH ROW EXECUTE FUNCTION solo_revoke_team_relationship();
