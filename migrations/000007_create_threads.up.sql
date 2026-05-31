CREATE TABLE threads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    root_message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    reply_count     INT NOT NULL DEFAULT 0,
    last_reply_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE messages ADD COLUMN thread_id UUID REFERENCES threads(id) ON DELETE SET NULL;

CREATE INDEX idx_threads_channel ON threads(channel_id, last_reply_at DESC NULLS LAST);
CREATE INDEX idx_threads_root_message ON threads(root_message_id);
CREATE INDEX idx_messages_thread ON messages(thread_id);
