-- Replace the existing (channel_id, created_at) index with a composite index
-- that includes id as a tiebreaker for deterministic cursor pagination.
-- The tuple comparison (created_at, id) < (cursor_created_at, cursor_id)
-- leverages this covering index and avoids full table scans.

DROP INDEX IF EXISTS idx_messages_channel;

CREATE INDEX idx_messages_channel ON messages(channel_id, created_at DESC, id DESC);
