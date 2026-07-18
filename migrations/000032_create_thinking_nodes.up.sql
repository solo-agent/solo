CREATE TABLE thinking_spaces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id  UUID NOT NULL UNIQUE REFERENCES channels(id) ON DELETE CASCADE,
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE thinking_nodes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id            UUID NOT NULL REFERENCES thinking_spaces(id) ON DELETE CASCADE,
    parent_id           UUID REFERENCES thinking_nodes(id) ON DELETE CASCADE,
    agent_id            UUID REFERENCES agents(id) ON DELETE SET NULL,
    agent_session_id    UUID REFERENCES agent_sessions(id) ON DELETE SET NULL,
    title               VARCHAR(100) NOT NULL,
    source              VARCHAR(20) NOT NULL DEFAULT 'manual'
                        CHECK (source IN ('root', 'team', 'manual', 'auto')),
    checkpoint_handoff  TEXT NOT NULL DEFAULT '',
    checkpoint_handoff_at TIMESTAMPTZ,
    inherited_handoff   TEXT NOT NULL DEFAULT '',
    fork_handoff_pending BOOLEAN NOT NULL DEFAULT false,
    fork_handoff_at     TIMESTAMPTZ,
    returned_handoff    TEXT NOT NULL DEFAULT '',
    returning_at        TIMESTAMPTZ,
    returned_at         TIMESTAMPTZ,
    depth               SMALLINT NOT NULL DEFAULT 0 CHECK (depth BETWEEN 0 AND 6),
    sort_order          INTEGER NOT NULL DEFAULT 0,
    created_by          UUID NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_thinking_nodes_root
    ON thinking_nodes(space_id) WHERE parent_id IS NULL;
CREATE UNIQUE INDEX idx_thinking_nodes_sibling_title
    ON thinking_nodes(parent_id, lower(title)) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_thinking_nodes_space_parent
    ON thinking_nodes(space_id, parent_id, sort_order, created_at);
CREATE UNIQUE INDEX idx_thinking_nodes_agent_session
    ON thinking_nodes(agent_session_id) WHERE agent_session_id IS NOT NULL;

ALTER TABLE messages
    ADD COLUMN thinking_node_id UUID REFERENCES thinking_nodes(id) ON DELETE CASCADE,
    ADD CONSTRAINT messages_single_conversation_scope
        CHECK (thread_id IS NULL OR thinking_node_id IS NULL);

CREATE INDEX idx_messages_thinking_node
    ON messages(thinking_node_id, created_at DESC, id DESC)
    WHERE thinking_node_id IS NOT NULL;

ALTER TABLE agent_runs
    ADD COLUMN thinking_node_id UUID REFERENCES thinking_nodes(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_runs_thinking_node
    ON agent_runs(thinking_node_id, started_at DESC)
    WHERE thinking_node_id IS NOT NULL;
