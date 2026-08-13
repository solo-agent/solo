ALTER TABLE workspaces ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX idx_workspaces_active
    ON workspaces(id) WHERE deleted_at IS NULL;
