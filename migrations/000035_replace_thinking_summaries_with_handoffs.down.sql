ALTER TABLE thinking_nodes
    DROP COLUMN checkpoint_handoff_at,
    DROP COLUMN fork_handoff_pending,
    DROP COLUMN fork_handoff_at;

ALTER TABLE thinking_nodes
    RENAME COLUMN checkpoint_handoff TO summary;

ALTER TABLE thinking_nodes
    RENAME COLUMN inherited_handoff TO inherited_summary;

ALTER TABLE thinking_nodes
    RENAME COLUMN returned_handoff TO returned_summary;
