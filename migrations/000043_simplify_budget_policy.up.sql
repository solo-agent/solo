UPDATE budget_policies
   SET enabled = false,
       updated_at = now()
 WHERE enabled = true
   AND monthly_limit_tokens = 0;

ALTER TABLE budget_policies
    DROP COLUMN IF EXISTS daily_limit_tokens,
    DROP COLUMN IF EXISTS per_run_reserve_tokens;
