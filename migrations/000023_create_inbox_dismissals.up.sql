-- Migration 000023: Create user_inbox_dismissals for inbox dismiss/done system.
-- Replaces the read/unread model — dismissed messages are removed from the inbox view.

CREATE TABLE user_inbox_dismissals (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id  UUID NOT NULL,
    dismissed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, message_id)
);
