ALTER TABLE automations
    ADD COLUMN completion_policy TEXT NOT NULL DEFAULT 'review_required'
    CHECK (completion_policy IN ('auto_complete', 'review_required'));

ALTER TABLE channels
    ADD COLUMN posting_policy TEXT NOT NULL DEFAULT 'everyone'
    CHECK (posting_policy IN ('everyone', 'admins_only'));

CREATE TABLE channel_member_mutes (
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    muted_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX idx_channel_member_mutes_active
    ON channel_member_mutes(channel_id, expires_at);

CREATE TABLE channel_message_pins (
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    pinned_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pinned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, message_id)
);

CREATE INDEX idx_channel_message_pins_recent
    ON channel_message_pins(channel_id, pinned_at DESC);
