CREATE TABLE channel_bindings (
    channel_id   UUID PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    repo_url     TEXT NOT NULL,
    repo_branch  VARCHAR(200) NOT NULL DEFAULT 'main',
    bind_path    TEXT NOT NULL,
    bound_by     UUID NOT NULL REFERENCES users(id),
    bound_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
