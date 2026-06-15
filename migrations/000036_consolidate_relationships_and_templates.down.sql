BEGIN;

DROP TABLE IF EXISTS agent_templates;

DROP INDEX IF EXISTS idx_rel_assigns_to_unique;
DROP INDEX IF EXISTS idx_collab_bidirectional_unique;

ALTER TABLE agent_relationships
    DROP CONSTRAINT IF EXISTS agent_relationships_rel_type_check;
ALTER TABLE agent_relationships
    ADD CONSTRAINT agent_relationships_rel_type_check
    CHECK (rel_type IN ('reports_to', 'delegates_to', 'collaborates_with', 'escalates_to'));

-- Best-effort split: assigns_to that previously was reports_to/delegates_to/escalates_to
-- cannot be reconstructed, so we leave them as assigns_to (will fail the new CHECK).
-- The migration is not cleanly reversible by design.

COMMIT;
