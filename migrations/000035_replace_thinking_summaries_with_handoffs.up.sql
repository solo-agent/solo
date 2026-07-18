ALTER TABLE thinking_nodes
    RENAME COLUMN summary TO checkpoint_handoff;

ALTER TABLE thinking_nodes
    RENAME COLUMN inherited_summary TO inherited_handoff;

ALTER TABLE thinking_nodes
    RENAME COLUMN returned_summary TO returned_handoff;

ALTER TABLE thinking_nodes
    ADD COLUMN checkpoint_handoff_at TIMESTAMPTZ,
    ADD COLUMN fork_handoff_pending BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN fork_handoff_at TIMESTAMPTZ;

-- Mechanical snapshots are intentionally discarded. Raw node conversations
-- remain authoritative and future cross-node context must be Agent-authored.
UPDATE thinking_nodes
   SET checkpoint_handoff = '',
       inherited_handoff = '';
