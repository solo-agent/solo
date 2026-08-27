CREATE TABLE lark_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL UNIQUE REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL UNIQUE REFERENCES channels(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    platform VARCHAR(16) NOT NULL CHECK (platform IN ('feishu', 'lark')),
    app_id VARCHAR(128) NOT NULL,
    app_secret_encrypted TEXT NOT NULL,
    verification_token_hash VARCHAR(64) NOT NULL,
    external_chat_id VARCHAR(128),
    external_chat_type VARCHAR(24),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (platform, app_id)
);

CREATE TABLE lark_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    binding_id UUID NOT NULL REFERENCES lark_bindings(id) ON DELETE CASCADE,
    direction VARCHAR(12) NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    external_message_id VARCHAR(160),
    solo_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX lark_deliveries_inbound_unique
    ON lark_deliveries(binding_id, external_message_id)
    WHERE direction = 'inbound' AND external_message_id IS NOT NULL;
CREATE UNIQUE INDEX lark_deliveries_outbound_unique
    ON lark_deliveries(binding_id, solo_message_id)
    WHERE direction = 'outbound' AND solo_message_id IS NOT NULL;
