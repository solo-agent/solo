CREATE TABLE agent_message_wake_slots (
    agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel_id   UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    active_run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, channel_id)
);

CREATE INDEX idx_agent_message_wake_slots_active_run
    ON agent_message_wake_slots(active_run_id)
    WHERE active_run_id IS NOT NULL;

CREATE TABLE agent_pending_message_wakes (
    agent_id             UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel_id           UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    scope_key            TEXT NOT NULL,
    thread_id            UUID REFERENCES threads(id) ON DELETE CASCADE,
    first_message_seq    BIGINT NOT NULL CHECK (first_message_seq > 0),
    latest_message_seq   BIGINT NOT NULL CHECK (latest_message_seq >= first_message_seq),
    requires_visible_result BOOLEAN NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, channel_id, scope_key),
    CHECK (
        (thread_id IS NULL AND scope_key = 'channel') OR
        (thread_id IS NOT NULL AND scope_key = 'thread:' || thread_id::text)
    )
);

CREATE INDEX idx_agent_pending_message_wakes_oldest
    ON agent_pending_message_wakes(agent_id, channel_id, created_at, scope_key);

ALTER TABLE agent_runs
    ADD COLUMN wake_first_message_seq BIGINT,
    ADD COLUMN wake_latest_message_seq BIGINT,
    ADD COLUMN wake_requires_visible_result BOOLEAN NOT NULL DEFAULT false,
    ADD CONSTRAINT chk_agent_run_wake_range CHECK (
        (wake_first_message_seq IS NULL AND wake_latest_message_seq IS NULL) OR
        (wake_first_message_seq > 0 AND wake_latest_message_seq >= wake_first_message_seq)
    );
