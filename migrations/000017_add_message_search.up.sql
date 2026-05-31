-- Add generated tsvector column for full-text search on messages.
-- Uses 'simple' config to avoid language-specific stemming/stopwords.
-- GENERATED ALWAYS ensures the column stays in sync on INSERT/UPDATE automatically.

ALTER TABLE messages ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', COALESCE(content, ''))) STORED;

-- GIN index for fast full-text search queries
CREATE INDEX IF NOT EXISTS idx_messages_search ON messages USING GIN (search_vector);
