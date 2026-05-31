ALTER TABLE messages ADD COLUMN IF NOT EXISTS attachment_ids UUID[] DEFAULT '{}';
CREATE INDEX IF NOT EXISTS idx_messages_attachment_ids ON messages USING GIN (attachment_ids);
