-- Add is_deleted column to messages table for soft delete support (W3-02-BE, W3-03-BE)
ALTER TABLE messages ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN NOT NULL DEFAULT false;
