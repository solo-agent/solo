CREATE TABLE task_watchdog (
    task_id        UUID PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    claimer_id     UUID NOT NULL REFERENCES agents(id),
    claimed_at     TIMESTAMPTZ NOT NULL,
    deadline       TIMESTAMPTZ NOT NULL,
    last_activity  TIMESTAMPTZ NOT NULL DEFAULT now(),
    timeout_action VARCHAR(20) NOT NULL DEFAULT 'remind'
                   CHECK (timeout_action IN ('remind', 'escalate', 'unclaim')),
    escalate_to    UUID REFERENCES agents(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_watchdog_deadline ON task_watchdog(deadline);
