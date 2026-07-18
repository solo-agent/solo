ALTER TABLE thinking_nodes
    ADD COLUMN agent_session_id UUID REFERENCES agent_sessions(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX idx_thinking_nodes_agent_session
    ON thinking_nodes(agent_session_id) WHERE agent_session_id IS NOT NULL;

ALTER TABLE agent_runs
    ADD COLUMN thinking_node_id UUID REFERENCES thinking_nodes(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_runs_thinking_node
    ON agent_runs(thinking_node_id, started_at DESC)
    WHERE thinking_node_id IS NOT NULL;
