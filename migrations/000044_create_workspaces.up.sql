CREATE TABLE workspaces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    icon        VARCHAR(32) NOT NULL DEFAULT 'S',
    visibility  VARCHAR(20) NOT NULL DEFAULT 'private'
                CHECK (visibility IN ('public', 'private')),
    is_default  BOOLEAN NOT NULL DEFAULT false,
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_workspaces_single_default
    ON workspaces(is_default) WHERE is_default = true;

INSERT INTO workspaces (id, name, icon, visibility, is_default)
VALUES ('00000000-0000-0000-0000-000000000001', 'Solo Public', 'S', 'public', true);

CREATE TABLE workspace_members (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         VARCHAR(20) NOT NULL DEFAULT 'member'
                 CHECK (role IN ('owner', 'admin', 'member')),
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX idx_workspace_members_user
    ON workspace_members(user_id, joined_at DESC);

INSERT INTO workspace_members (workspace_id, user_id, role, joined_at)
SELECT '00000000-0000-0000-0000-000000000001', id, 'member', created_at
FROM users
ON CONFLICT DO NOTHING;

ALTER TABLE channels
    ADD COLUMN workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE;

UPDATE channels
SET workspace_id = '00000000-0000-0000-0000-000000000001'
WHERE workspace_id IS NULL;

ALTER TABLE channels ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE channels ALTER COLUMN workspace_id
    SET DEFAULT '00000000-0000-0000-0000-000000000001';

DROP INDEX idx_channels_active_name;

INSERT INTO channels (id, name, description, type, created_by, workspace_id)
SELECT '00000000-0000-0000-0000-000000000002', 'general',
       'Public lobby for everyone on Solo', 'channel', id,
       '00000000-0000-0000-0000-000000000001'
  FROM users
 WHERE NOT EXISTS (
       SELECT 1 FROM channels
        WHERE workspace_id = '00000000-0000-0000-0000-000000000001'
          AND name = 'general' AND type = 'channel' AND is_archived = false
 )
 ORDER BY created_at ASC
 LIMIT 1;

INSERT INTO channel_members (channel_id, member_type, member_id, role)
SELECT public_channel.id, 'user', wm.user_id, 'member'
  FROM workspace_members wm
  JOIN LATERAL (
       SELECT id FROM channels
        WHERE workspace_id = '00000000-0000-0000-0000-000000000001'
          AND name = 'general' AND type = 'channel' AND is_archived = false
        ORDER BY created_at ASC LIMIT 1
  ) public_channel ON true
 WHERE wm.workspace_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

CREATE UNIQUE INDEX idx_channels_workspace_active_name
    ON channels(workspace_id, name)
    WHERE type = 'channel' AND is_archived = false;
CREATE INDEX idx_channels_workspace_created
    ON channels(workspace_id, created_at DESC);

CREATE TABLE workspace_invitations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email          VARCHAR(255) NOT NULL,
    role           VARCHAR(20) NOT NULL DEFAULT 'member'
                   CHECK (role IN ('admin', 'member')),
    invited_by     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    accepted_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    accepted_at    TIMESTAMPTZ,
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '30 days'),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_workspace_invitations_pending
    ON workspace_invitations(workspace_id, lower(email))
    WHERE accepted_at IS NULL;
CREATE INDEX idx_workspace_invitations_email
    ON workspace_invitations(lower(email), expires_at)
    WHERE accepted_at IS NULL;

CREATE TABLE workspace_join_rules (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    rule_type     VARCHAR(20) NOT NULL CHECK (rule_type IN ('email', 'domain')),
    value         VARCHAR(255) NOT NULL,
    role          VARCHAR(20) NOT NULL DEFAULT 'member' CHECK (role = 'member'),
    created_by    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, rule_type, value)
);

CREATE TABLE workspace_embed_policies (
    workspace_id             UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    enabled                  BOOLEAN NOT NULL DEFAULT false,
    allow_agent_invocations  BOOLEAN NOT NULL DEFAULT false,
    updated_by               UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace_embed_channels (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id   UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    PRIMARY KEY (workspace_id, channel_id)
);
