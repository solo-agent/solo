ALTER TABLE agents
    DROP COLUMN IF EXISTS runtime_id,
    DROP COLUMN IF EXISTS custom_args,
    DROP COLUMN IF EXISTS custom_env;
