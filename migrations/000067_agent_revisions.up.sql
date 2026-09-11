CREATE TABLE agent_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
 config jsonb NOT NULL CHECK(jsonb_typeof(config)='object'),
 config_hash text NOT NULL,
 summary text NOT NULL DEFAULT '',
 created_by uuid NOT NULL,
 evaluation_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(agent_id,config_hash)
);
CREATE TABLE agent_revision_publications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
 revision_id uuid NOT NULL REFERENCES agent_revisions(id) ON DELETE CASCADE,
 evaluation_task_id uuid REFERENCES tasks(id) ON DELETE SET NULL,
 published_by uuid NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE channel_team_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
 lockfile jsonb NOT NULL,
 created_by uuid NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE channels ADD COLUMN team_version_id uuid REFERENCES channel_team_versions(id) ON DELETE SET NULL;
ALTER TABLE agent_runs ADD COLUMN agent_revision_id uuid REFERENCES agent_revisions(id) ON DELETE SET NULL;
ALTER TABLE agent_runs ADD COLUMN team_version_id uuid REFERENCES channel_team_versions(id) ON DELETE SET NULL;

CREATE FUNCTION solo_agent_config(a agents) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_build_object('system_prompt',COALESCE(a.system_prompt,''),'model_provider',a.model_provider,'model_name',a.model_name,'custom_args',COALESCE(a.custom_args,'[]'::jsonb))
$$;
CREATE FUNCTION solo_capture_run_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE config_value jsonb; config_digest text; revision uuid; team_version uuid;
BEGIN
 SELECT c.team_version_id INTO team_version FROM channels c WHERE c.id=NEW.channel_id;
 IF team_version IS NOT NULL THEN
  SELECT (member->>'revision_id')::uuid INTO revision FROM channel_team_versions v, jsonb_array_elements(v.lockfile->'members') member WHERE v.id=team_version AND member->>'agent_id'=NEW.agent_id::text;
  IF revision IS NULL THEN RAISE EXCEPTION 'Agent is not in the published team; publish updated membership first'; END IF;
 ELSE
  SELECT solo_agent_config(a) INTO config_value FROM agents a WHERE a.id=NEW.agent_id;
  config_digest := encode(sha256(convert_to(config_value::text,'UTF8')),'hex');
  INSERT INTO agent_revisions(agent_id,config,config_hash,created_by,summary) VALUES(NEW.agent_id,config_value,config_digest,NEW.agent_id,'Captured execution configuration') ON CONFLICT(agent_id,config_hash) DO NOTHING;
  SELECT id INTO revision FROM agent_revisions WHERE agent_id=NEW.agent_id AND config_hash=config_digest;
 END IF;
 NEW.agent_revision_id := revision; NEW.team_version_id := team_version;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_run_capture_revision BEFORE INSERT ON agent_runs FOR EACH ROW EXECUTE FUNCTION solo_capture_run_revision();
CREATE INDEX agent_runs_revision_idx ON agent_runs(agent_revision_id,started_at);
CREATE FUNCTION solo_immutable_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.config IS DISTINCT FROM OLD.config OR NEW.agent_id IS DISTINCT FROM OLD.agent_id OR NEW.config_hash IS DISTINCT FROM OLD.config_hash THEN RAISE EXCEPTION 'Agent revision snapshots are immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_revision_immutable BEFORE UPDATE ON agent_revisions FOR EACH ROW EXECUTE FUNCTION solo_immutable_revision();
