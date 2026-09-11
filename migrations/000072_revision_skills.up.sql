ALTER TABLE agents ADD COLUMN skills jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(skills)='array');
CREATE OR REPLACE FUNCTION solo_agent_config(a agents) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_build_object('system_prompt',COALESCE(a.system_prompt,''),'model_provider',a.model_provider,'model_name',a.model_name,'custom_args',COALESCE(a.custom_args,'[]'::jsonb))
  || CASE WHEN a.skills='[]'::jsonb THEN '{}'::jsonb ELSE jsonb_build_object('skills',a.skills) END
$$;
