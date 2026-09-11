DROP TRIGGER agent_run_capture_revision ON agent_runs;
DROP FUNCTION solo_capture_run_revision();
DROP FUNCTION solo_agent_config(agents);
ALTER TABLE agent_runs DROP COLUMN team_version_id, DROP COLUMN agent_revision_id;
ALTER TABLE channels DROP COLUMN team_version_id;
DROP TABLE channel_team_versions;
DROP TABLE agent_revision_publications;
DROP TABLE agent_revisions;
DROP FUNCTION solo_immutable_revision();
