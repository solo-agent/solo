-- Migration 000024: Create user_mentions for real-time @mention tracking.
-- Records @mentions at message send time so inbox can use indexed JOIN instead of ILIKE.

CREATE TABLE user_mentions (
    message_id        UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    mentioned_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, mentioned_user_id)
);

CREATE INDEX idx_user_mentions_user ON user_mentions(mentioned_user_id, created_at DESC);
