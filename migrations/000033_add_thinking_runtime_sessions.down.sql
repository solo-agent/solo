DROP INDEX IF EXISTS idx_agent_runs_thinking_node;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS thinking_node_id;
DROP INDEX IF EXISTS idx_thinking_nodes_agent_session;
ALTER TABLE thinking_nodes DROP COLUMN IF EXISTS agent_session_id;
