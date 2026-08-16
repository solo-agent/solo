CREATE TABLE workspace_invite_links (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    token_hash   BYTEA NOT NULL UNIQUE,
    created_by   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         VARCHAR(20) NOT NULL DEFAULT 'member' CHECK (role = 'member'),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '7 days'),
    revoked_at   TIMESTAMPTZ,
    use_count    INTEGER NOT NULL DEFAULT 0 CHECK (use_count >= 0),
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_invite_links_workspace_active
    ON workspace_invite_links(workspace_id, created_at DESC)
    WHERE revoked_at IS NULL;
