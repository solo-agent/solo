ALTER TABLE budget_policies
    ADD COLUMN IF NOT EXISTS daily_limit_tokens BIGINT NOT NULL DEFAULT 0 CHECK (daily_limit_tokens >= 0),
    ADD COLUMN IF NOT EXISTS per_run_reserve_tokens BIGINT NOT NULL DEFAULT 0 CHECK (per_run_reserve_tokens >= 0);
