DELETE FROM agent_templates
 WHERE id IN ('starter-web-page', 'starter-data-analysis', 'starter-study-organizer');

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_project_binding_complete;
ALTER TABLE channels DROP COLUMN IF EXISTS project_path;
ALTER TABLE channels DROP COLUMN IF EXISTS project_computer_id;
