DROP INDEX IF EXISTS idx_thinking_nodes_source_message;

ALTER TABLE thinking_nodes DROP COLUMN IF EXISTS source_message_id;

DROP TABLE IF EXISTS message_favorites;
