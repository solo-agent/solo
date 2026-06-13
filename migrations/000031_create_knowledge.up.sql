CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE knowledge (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    embedding       vector(1536),
    source          VARCHAR(50) NOT NULL DEFAULT 'manual',
    source_ref      TEXT,
    view_count      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_knowledge_fts ON knowledge USING GIN (to_tsvector('simple', title || ' ' || content));
CREATE INDEX idx_knowledge_channel_time ON knowledge(channel_id, created_at DESC);
CREATE INDEX idx_knowledge_tags ON knowledge USING GIN (tags);
