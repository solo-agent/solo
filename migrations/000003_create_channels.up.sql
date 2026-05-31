CREATE TABLE channels (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    type            VARCHAR(20) NOT NULL DEFAULT 'channel',  -- 'channel', 'dm'
    created_by      UUID NOT NULL REFERENCES users(id),
    is_archived     BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_channels_type ON channels(type);
CREATE INDEX idx_channels_created_at ON channels(created_at DESC);

-- Unique channel name for non-archived non-dm channels
CREATE UNIQUE INDEX idx_channels_active_name ON channels(name) WHERE type = 'channel' AND is_archived = false;

-- Channel members
CREATE TABLE channel_members (
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    member_type     VARCHAR(20) NOT NULL,  -- 'user', 'agent'
    member_id       UUID NOT NULL,          -- references users(id) or agents(id)
    role            VARCHAR(20) NOT NULL DEFAULT 'member',  -- 'owner', 'admin', 'member'
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, member_type, member_id)
);

CREATE INDEX idx_channel_members_member ON channel_members(member_type, member_id);
CREATE INDEX idx_channel_members_channel ON channel_members(channel_id, joined_at DESC);
