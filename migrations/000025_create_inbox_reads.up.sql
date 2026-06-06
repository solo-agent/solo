-- Migration 000025: Per-message read tracking for inbox.
-- Each row = the user has read this specific message.

CREATE TABLE user_inbox_reads (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL,
    read_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, message_id)
);
