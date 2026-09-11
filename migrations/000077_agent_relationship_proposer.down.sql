DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM agent_relationship_proposals WHERE proposed_by_agent_id IS NOT NULL AND status='pending') THEN
  RAISE EXCEPTION 'Resolve Agent-initiated pending agreements and export initiator history before rollback';
 END IF;
END $$;
ALTER TABLE agent_relationship_proposals DROP COLUMN proposed_by_agent_id;
