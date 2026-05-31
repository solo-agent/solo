DROP INDEX IF EXISTS idx_messages_thread;
DROP INDEX IF EXISTS idx_threads_root_message;
DROP INDEX IF EXISTS idx_threads_channel;

ALTER TABLE messages DROP COLUMN IF EXISTS thread_id;

DROP TABLE IF EXISTS threads;
