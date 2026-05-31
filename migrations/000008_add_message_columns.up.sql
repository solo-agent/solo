-- Add missing columns to messages table for thread replies and agent mentions
ALTER TABLE messages ADD COLUMN IF NOT EXISTS mentioned_agent_ids UUID[] DEFAULT '{}';
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_to UUID;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS is_edited BOOLEAN DEFAULT false;

-- Index for reply_to lookups (finding replies to a specific message)
CREATE INDEX IF NOT EXISTS idx_messages_reply_to ON messages(reply_to);

-- Index for mentioned agent lookups (GIN index for array containment queries)
CREATE INDEX IF NOT EXISTS idx_messages_mentioned_agents ON messages USING GIN (mentioned_agent_ids);
