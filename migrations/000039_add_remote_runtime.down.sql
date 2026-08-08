DROP INDEX IF EXISTS idx_agent_run_events_attempt_source;

ALTER TABLE agent_run_events
    DROP COLUMN IF EXISTS source_seq,
    DROP COLUMN IF EXISTS attempt_id;

DROP INDEX IF EXISTS idx_agent_runs_agent_accepted;
DROP INDEX IF EXISTS idx_agent_runs_computer_delivery;

ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS delivery_count,
    DROP COLUMN IF EXISTS retry_of_run_id,
    DROP COLUMN IF EXISTS accepted_at,
    DROP COLUMN IF EXISTS execution_attempt_id,
    DROP COLUMN IF EXISTS delivery_expires_at,
    DROP COLUMN IF EXISTS dispatch_payload,
    DROP COLUMN IF EXISTS computer_id;

DROP INDEX IF EXISTS idx_computers_credential_hash;

ALTER TABLE computers
    DROP COLUMN IF EXISTS last_connected_at,
    DROP COLUMN IF EXISTS runtime_inventory,
    DROP COLUMN IF EXISTS daemon_version,
    DROP COLUMN IF EXISTS protocol_version,
    DROP COLUMN IF EXISTS enrollment_used_at,
    DROP COLUMN IF EXISTS enrollment_expires_at,
    DROP COLUMN IF EXISTS enrollment_token_hash,
    DROP COLUMN IF EXISTS credential_revoked_at,
    DROP COLUMN IF EXISTS credential_created_at,
    DROP COLUMN IF EXISTS credential_hash;
