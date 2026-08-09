CREATE TABLE IF NOT EXISTS budget_policies (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope_type             TEXT NOT NULL CHECK (scope_type IN ('user', 'agent')),
    agent_id               UUID REFERENCES agents(id) ON DELETE CASCADE,
    enabled                BOOLEAN NOT NULL DEFAULT false,
    monthly_limit_tokens   BIGINT NOT NULL DEFAULT 0 CHECK (monthly_limit_tokens >= 0),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scope_type = 'user' AND agent_id IS NULL)
        OR (scope_type = 'agent' AND agent_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_budget_policies_user
    ON budget_policies(owner_id)
    WHERE scope_type = 'user';

CREATE UNIQUE INDEX IF NOT EXISTS idx_budget_policies_agent
    ON budget_policies(agent_id)
    WHERE scope_type = 'agent';

CREATE TABLE IF NOT EXISTS agent_run_token_usage (
    run_id             UUID PRIMARY KEY REFERENCES agent_runs(id) ON DELETE CASCADE,
    owner_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id           UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    state              TEXT NOT NULL CHECK (state IN ('pending', 'settled', 'released', 'usage_unknown')),
    reserved_tokens    BIGINT NOT NULL DEFAULT 0 CHECK (reserved_tokens >= 0),
    actual_tokens      BIGINT CHECK (actual_tokens IS NULL OR actual_tokens >= 0),
    input_tokens       BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens      BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cache_read_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    cache_write_tokens BIGINT NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    overrun            BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_agent_run_token_usage_owner_created
    ON agent_run_token_usage(owner_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_run_token_usage_agent_created
    ON agent_run_token_usage(agent_id, created_at DESC);

CREATE TABLE IF NOT EXISTS budget_reservations (
    run_id            UUID NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    policy_id         UUID NOT NULL REFERENCES budget_policies(id) ON DELETE CASCADE,
    reserved_tokens   BIGINT NOT NULL CHECK (reserved_tokens >= 0),
    accounted_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (accounted_tokens >= 0),
    state             TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'settled', 'released')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at        TIMESTAMPTZ,
    PRIMARY KEY (run_id, policy_id)
);

CREATE INDEX IF NOT EXISTS idx_budget_reservations_policy_state
    ON budget_reservations(policy_id, state, created_at DESC);
