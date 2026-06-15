-- Phase 0/Task 0.2: Consolidate 4 relationship types → 2.

BEGIN;

-- 0. Drop the old 4-value CHECK first; otherwise the UPDATE in step 1
-- would write 'assigns_to' while the old CHECK still forbids it.
ALTER TABLE agent_relationships
    DROP CONSTRAINT IF EXISTS agent_relationships_rel_type_check;

-- 1. Merge reports_to/delegates_to/escalates_to → assigns_to.
UPDATE agent_relationships
   SET rel_type = 'assigns_to'
 WHERE rel_type IN ('reports_to', 'delegates_to', 'escalates_to');

-- 2. Deduplicate (same from+to+channel) for assigns_to.
DELETE FROM agent_relationships a
USING agent_relationships b
WHERE a.rel_type = 'assigns_to'
  AND b.rel_type = 'assigns_to'
  AND a.ctid > b.ctid
  AND a.from_agent_id = b.from_agent_id
  AND a.to_agent_id   = b.to_agent_id
  AND a.channel_id IS NOT DISTINCT FROM b.channel_id;

-- 3. Tighten CHECK to 2 types.
ALTER TABLE agent_relationships
    ADD CONSTRAINT agent_relationships_rel_type_check
    CHECK (rel_type IN ('assigns_to', 'collaborates_with'));

-- 4. Drop old per-type unique indexes, replace with consolidated ones.
DROP INDEX IF EXISTS idx_rel_global;
DROP INDEX IF EXISTS idx_rel_channel;
CREATE UNIQUE INDEX IF NOT EXISTS idx_rel_assigns_to_unique
    ON agent_relationships(from_agent_id, to_agent_id, COALESCE(channel_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE rel_type = 'assigns_to';
CREATE UNIQUE INDEX IF NOT EXISTS idx_collab_bidirectional_unique
    ON agent_relationships(LEAST(from_agent_id, to_agent_id),
                           GREATEST(from_agent_id, to_agent_id),
                           rel_type,
                           COALESCE(channel_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE rel_type = 'collaborates_with';

-- 5. New agent_templates table.
CREATE TABLE IF NOT EXISTS agent_templates (
    id          VARCHAR(50) PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    category    VARCHAR(50) NOT NULL,
    icon        VARCHAR(20),
    members     JSONB NOT NULL,
    is_official BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_templates_category ON agent_templates(category);

COMMIT;
