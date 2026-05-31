-- Restore the original composite index without the id tiebreaker.

DROP INDEX IF EXISTS idx_messages_channel;

CREATE INDEX idx_messages_channel ON messages(channel_id, created_at DESC);
