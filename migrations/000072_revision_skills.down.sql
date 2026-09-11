DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM agents WHERE skills <> '[]'::jsonb)
 OR EXISTS(SELECT 1 FROM agent_runs r JOIN agent_revisions v ON v.id=r.agent_revision_id WHERE r.finished_at IS NULL AND COALESCE(v.config->'skills','[]'::jsonb)<>'[]'::jsonb) THEN
  RAISE EXCEPTION 'Remove live Skill bundles and finish their Runs before rollback; historical Revision files are retained';
 END IF;
END $$;
CREATE OR REPLACE FUNCTION solo_agent_config(a agents) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_build_object('system_prompt',COALESCE(a.system_prompt,''),'model_provider',a.model_provider,'model_name',a.model_name,'custom_args',COALESCE(a.custom_args,'[]'::jsonb))
$$;
ALTER TABLE agents DROP COLUMN skills;
