DROP INDEX IF EXISTS idx_workspaces_active;
ALTER TABLE workspaces DROP COLUMN IF EXISTS deleted_at;
