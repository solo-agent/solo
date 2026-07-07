CREATE TABLE channel_contexts (
    channel_id      UUID PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    target          TEXT NOT NULL DEFAULT '',
    agenda_json     JSONB NOT NULL DEFAULT '[]'::jsonb,
    context_version INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE interaction_cards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    scope           TEXT NOT NULL DEFAULT 'channel',
    subject_type    TEXT,
    subject_id      UUID,
    card_type       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open',
    title           TEXT NOT NULL,
    body            TEXT,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    actions_json    JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT interaction_cards_scope_check
        CHECK (scope IN ('channel', 'team', 'thought', 'task')),
    CONSTRAINT interaction_cards_subject_type_check
        CHECK (subject_type IS NULL OR subject_type IN ('channel', 'team', 'thought', 'thought_node', 'task')),
    CONSTRAINT interaction_cards_type_check
        CHECK (card_type IN ('setup', 'channel_create', 'next_step', 'thought_review', 'tasks_created', 'task_review')),
    CONSTRAINT interaction_cards_status_check
        CHECK (status IN ('open', 'accepted', 'dismissed', 'expired'))
);

CREATE TABLE context_records (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id        UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    scope             TEXT NOT NULL DEFAULT 'channel',
    subject_type      TEXT,
    subject_id        UUID,
    record_type       TEXT NOT NULL,
    title             TEXT NOT NULL,
    body              TEXT NOT NULL,
    author_type       TEXT NOT NULL,
    author_id         UUID,
    artifact_ref_json JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT context_records_scope_check
        CHECK (scope IN ('channel', 'team', 'thought', 'task')),
    CONSTRAINT context_records_subject_type_check
        CHECK (subject_type IS NULL OR subject_type IN ('channel', 'team', 'thought', 'thought_node', 'task')),
    CONSTRAINT context_records_type_check
        CHECK (record_type IN ('summary', 'insight', 'artifact', 'team_summary')),
    CONSTRAINT context_records_author_type_check
        CHECK (author_type IN ('user', 'agent', 'system'))
);

CREATE TABLE timeline_items (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id     UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    scope          TEXT NOT NULL DEFAULT 'channel',
    subject_type   TEXT,
    subject_id     UUID,
    item_kind      TEXT NOT NULL,
    item_type      TEXT NOT NULL,
    ref_id         UUID NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT timeline_items_scope_check
        CHECK (scope IN ('channel', 'team', 'thought', 'task')),
    CONSTRAINT timeline_items_subject_type_check
        CHECK (subject_type IS NULL OR subject_type IN ('channel', 'team', 'thought', 'thought_node', 'task')),
    CONSTRAINT timeline_items_kind_check
        CHECK (item_kind IN ('message', 'card', 'record'))
);

CREATE INDEX idx_interaction_cards_channel_created ON interaction_cards(channel_id, created_at DESC);
CREATE INDEX idx_interaction_cards_status ON interaction_cards(channel_id, status, created_at DESC);

CREATE INDEX idx_context_records_channel_created ON context_records(channel_id, created_at DESC);
CREATE INDEX idx_context_records_subject ON context_records(channel_id, scope, subject_type, subject_id, created_at DESC);
CREATE INDEX idx_context_records_type ON context_records(channel_id, record_type, created_at DESC);

CREATE INDEX idx_timeline_items_channel_created ON timeline_items(channel_id, created_at DESC);
CREATE INDEX idx_timeline_items_subject ON timeline_items(channel_id, scope, subject_type, subject_id, created_at DESC);
