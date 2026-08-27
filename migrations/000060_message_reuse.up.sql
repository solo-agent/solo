CREATE TABLE message_favorites (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, message_id)
);

CREATE INDEX idx_message_favorites_user_created
    ON message_favorites (user_id, created_at DESC);

ALTER TABLE thinking_nodes
    ADD COLUMN source_message_id UUID REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX idx_thinking_nodes_source_message
    ON thinking_nodes (source_message_id)
    WHERE source_message_id IS NOT NULL;
