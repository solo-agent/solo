-- Migration 000022: Create user_inbox_state table for Inbox read tracking.
-- Stores each user's last_read_at timestamp for unread-count calculation.

CREATE TABLE user_inbox_state (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
