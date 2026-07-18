DROP INDEX IF EXISTS idx_messages_thinking_node;
ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_single_conversation_scope,
    DROP COLUMN IF EXISTS thinking_node_id;
DROP TABLE IF EXISTS thinking_nodes;
DROP TABLE IF EXISTS thinking_spaces;
