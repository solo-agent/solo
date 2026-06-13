-- Try to enable pgvector. If the extension is not available,
-- the knowledge table is still created without the embedding column.
-- Semantic search degrades gracefully to FTS-only mode.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector extension not available — knowledge will use FTS-only search';
END $$;

CREATE TABLE knowledge (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    source          VARCHAR(50) NOT NULL DEFAULT 'manual',
    source_ref      TEXT,
    view_count      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_knowledge_fts ON knowledge USING GIN (to_tsvector('simple', title || ' ' || content));
CREATE INDEX idx_knowledge_channel_time ON knowledge(channel_id, created_at DESC);
CREATE INDEX idx_knowledge_tags ON knowledge USING GIN (tags);

-- Add embedding column only if vector extension is available.
DO $$
BEGIN
    ALTER TABLE knowledge ADD COLUMN embedding vector(1536);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector not available — embedding column skipped';
END $$;
