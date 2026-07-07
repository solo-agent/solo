DROP INDEX IF EXISTS idx_messages_workspace_scope;

ALTER TABLE messages
  DROP COLUMN IF EXISTS subject_id,
  DROP COLUMN IF EXISTS subject_type,
  DROP COLUMN IF EXISTS workspace_scope;
