DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM agent_pending_work WHERE status='pending') THEN
  RAISE EXCEPTION 'Resolve or explicitly cancel pending Agent work before removing its durable queue';
 END IF;
END $$;
DROP VIEW agent_inbox_heads;
DROP VIEW agent_inbox;
DROP TABLE agent_pending_work;
DROP FUNCTION release_cancelled_inbox_return();
