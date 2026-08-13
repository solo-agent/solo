DROP INDEX IF EXISTS idx_agents_owner_lucy_home_active;
DROP INDEX IF EXISTS idx_channels_workspace_owner_lucy_active;
DROP INDEX IF EXISTS idx_workspaces_active_personal_owner;

WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY owner_id ORDER BY created_at ASC) AS position
      FROM agents
     WHERE kind = 'lucy' AND is_active = true
)
UPDATE agents a
   SET is_active = false, updated_at = now()
  FROM ranked r
 WHERE r.position > 1 AND a.id = r.id;

CREATE UNIQUE INDEX idx_agents_owner_lucy_active
    ON agents(owner_id)
    WHERE is_active = true AND kind = 'lucy';

ALTER TABLE workspaces DROP COLUMN IF EXISTS is_personal;
