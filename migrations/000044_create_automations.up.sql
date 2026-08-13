CREATE TABLE automations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    creator_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    task_title TEXT NOT NULL,
    task_description TEXT NOT NULL DEFAULT '',
    target_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    schedule_type TEXT NOT NULL CHECK (schedule_type IN ('daily', 'weekdays', 'weekly')),
    schedule_hour SMALLINT NOT NULL CHECK (schedule_hour BETWEEN 0 AND 23),
    schedule_minute SMALLINT NOT NULL CHECK (schedule_minute BETWEEN 0 AND 59),
    schedule_weekday SMALLINT CHECK (schedule_weekday BETWEEN 0 AND 6),
    timezone TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT automations_weekly_day_required CHECK (
        (schedule_type = 'weekly' AND schedule_weekday IS NOT NULL)
        OR (schedule_type <> 'weekly' AND schedule_weekday IS NULL)
    )
);

CREATE INDEX idx_automations_channel ON automations(channel_id, created_at DESC);
CREATE INDEX idx_automations_due ON automations(next_run_at)
    WHERE enabled = true AND next_run_at IS NOT NULL;

CREATE TABLE automation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    automation_id UUID NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('scheduled', 'manual')),
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'skipped', 'failed')),
    scheduled_for TIMESTAMPTZ NOT NULL,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    coalesced_into_run_id UUID REFERENCES automation_runs(id) ON DELETE SET NULL,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_automation_runs_history
    ON automation_runs(automation_id, created_at DESC);
CREATE UNIQUE INDEX idx_automation_runs_one_active
    ON automation_runs(automation_id)
    WHERE status = 'running';
