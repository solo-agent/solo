ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS project_version,
    DROP COLUMN IF EXISTS project_path,
    DROP COLUMN IF EXISTS project_computer_id;

DROP TABLE IF EXISTS channel_project_mappings;

ALTER TABLE channels
    DROP COLUMN IF EXISTS project_baseline,
    DROP COLUMN IF EXISTS project_source;
