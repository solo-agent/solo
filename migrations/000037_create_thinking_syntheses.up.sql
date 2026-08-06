CREATE TABLE thinking_syntheses (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id             UUID NOT NULL REFERENCES thinking_spaces(id) ON DELETE CASCADE,
    created_by           UUID NOT NULL,
    title                VARCHAR(150) NOT NULL,
    objective            TEXT NOT NULL,
    constraints          JSONB NOT NULL DEFAULT '{}'::jsonb
                         CHECK (jsonb_typeof(constraints) = 'object'),
    mode                 VARCHAR(20) NOT NULL DEFAULT 'single_agent'
                         CHECK (mode IN ('single_agent', 'review_team')),
    coordinator_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    status               VARCHAR(20) NOT NULL DEFAULT 'draft'
                         CHECK (status IN ('draft', 'ready', 'running', 'reviewing', 'completed', 'failed', 'cancelled')),
    result_artifact_id   UUID REFERENCES artifacts(id) ON DELETE SET NULL,
    result_node_id       UUID REFERENCES thinking_nodes(id) ON DELETE SET NULL,
    error                TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_thinking_syntheses_space_created
    ON thinking_syntheses(space_id, created_at DESC);
CREATE INDEX idx_thinking_syntheses_status
    ON thinking_syntheses(status, updated_at DESC);

CREATE TABLE thinking_synthesis_sources (
    synthesis_id              UUID NOT NULL REFERENCES thinking_syntheses(id) ON DELETE CASCADE,
    node_id                   UUID NOT NULL,
    node_title_snapshot       TEXT NOT NULL,
    handoff_kind              VARCHAR(20) NOT NULL
                              CHECK (handoff_kind IN ('checkpoint', 'returned')),
    handoff_snapshot          TEXT NOT NULL,
    handoff_at                TIMESTAMPTZ NOT NULL,
    checkpoint_status_snapshot VARCHAR(20) NOT NULL
                              CHECK (checkpoint_status_snapshot IN ('fresh', 'stale', 'final')),
    path_snapshot             JSONB NOT NULL DEFAULT '[]'::jsonb
                              CHECK (jsonb_typeof(path_snapshot) = 'array'),
    user_note                 TEXT NOT NULL DEFAULT '',
    sort_order                INTEGER NOT NULL DEFAULT 0,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (synthesis_id, node_id),
    UNIQUE (synthesis_id, sort_order)
);

CREATE INDEX idx_thinking_synthesis_sources_node
    ON thinking_synthesis_sources(node_id, created_at DESC);
