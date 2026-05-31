-- Remove columns added in 000008
DROP INDEX IF EXISTS idx_messages_mentioned_agents;
DROP INDEX IF EXISTS idx_messages_reply_to;

ALTER TABLE messages DROP COLUMN IF EXISTS is_edited;
ALTER TABLE messages DROP COLUMN IF EXISTS reply_to;
ALTER TABLE messages DROP COLUMN IF EXISTS mentioned_agent_ids;
