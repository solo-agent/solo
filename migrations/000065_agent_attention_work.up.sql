ALTER TABLE agents ADD COLUMN attention_policy TEXT NOT NULL DEFAULT 'all'
    CHECK (attention_policy IN ('all','mentions','nothing'));
CREATE TABLE agent_channel_attention (
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    policy TEXT NOT NULL CHECK (policy IN ('all','mentions','nothing')),
    PRIMARY KEY(agent_id,channel_id)
);
CREATE TABLE thread_subscriptions (
    thread_id UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL,
    followed BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(thread_id,actor_id)
);
CREATE FUNCTION follow_thread_on_reply() RETURNS trigger AS $$
BEGIN
    IF NEW.thread_id IS NOT NULL AND NEW.sender_type IN ('agent','user') THEN
        INSERT INTO thread_subscriptions(thread_id,actor_id,followed) VALUES(NEW.thread_id,NEW.sender_id,true)
        ON CONFLICT(thread_id,actor_id) DO UPDATE SET followed=true,updated_at=now();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER message_follow AFTER INSERT ON messages FOR EACH ROW EXECUTE FUNCTION follow_thread_on_reply();

CREATE TABLE agent_work_marks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    source_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    description TEXT NOT NULL CHECK(length(description) BETWEEN 1 AND 8000),
    next_action TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved','cancelled')),
    resolution TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(agent_id,idempotency_key)
);
CREATE INDEX agent_work_marks_pending ON agent_work_marks(agent_id,created_at) WHERE status='open';
CREATE TABLE agent_held_drafts (
    run_id UUID PRIMARY KEY REFERENCES agent_runs(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES threads(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    based_on_seq BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agent_message_consumptions (
 agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
 message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
 run_id UUID NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
 PRIMARY KEY(agent_id,message_id)
);
