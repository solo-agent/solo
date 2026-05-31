DROP INDEX IF EXISTS idx_messages_attachment_ids;
ALTER TABLE messages DROP COLUMN IF EXISTS attachment_ids;
