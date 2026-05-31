-- Remove is_deleted column from messages table
ALTER TABLE messages DROP COLUMN IF EXISTS is_deleted;
