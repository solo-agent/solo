ALTER TABLE messages
  ADD COLUMN IF NOT EXISTS workspace_scope VARCHAR(20) NOT NULL DEFAULT 'channel',
  ADD COLUMN IF NOT EXISTS subject_type VARCHAR(40),
  ADD COLUMN IF NOT EXISTS subject_id UUID;

CREATE INDEX IF NOT EXISTS idx_messages_workspace_scope
  ON messages(channel_id, workspace_scope, subject_type, subject_id, created_at DESC);
