CREATE TABLE reminders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel_id      UUID REFERENCES channels(id) ON DELETE CASCADE,
    task_id         UUID REFERENCES tasks(id) ON DELETE CASCADE,
    reminder_type   VARCHAR(30) NOT NULL CHECK (reminder_type IN ('task_deadline', 'stale_task', 'periodic_checkin', 'custom')),
    remind_at       TIMESTAMPTZ NOT NULL,
    message         TEXT NOT NULL,
    is_recurring    BOOLEAN NOT NULL DEFAULT false,
    recurring_rule  VARCHAR(100),
    is_fired        BOOLEAN NOT NULL DEFAULT false,
    fired_at        TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reminders_pending ON reminders(remind_at) WHERE is_fired = false;
CREATE INDEX idx_reminders_agent ON reminders(agent_id, remind_at);
CREATE INDEX idx_reminders_task ON reminders(task_id);
