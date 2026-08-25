ALTER TABLE channels
    ADD COLUMN project_source TEXT NOT NULL DEFAULT '',
    ADD COLUMN project_baseline TEXT NOT NULL DEFAULT '';

CREATE TABLE channel_project_mappings (
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    computer_id UUID NOT NULL REFERENCES computers(id) ON DELETE CASCADE,
    local_path TEXT NOT NULL CHECK (length(btrim(local_path)) > 0),
    version TEXT NOT NULL DEFAULT '',
    access_mode TEXT NOT NULL DEFAULT 'read_write' CHECK (access_mode IN ('read_only', 'read_write')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, user_id, computer_id)
);

CREATE INDEX idx_channel_project_mappings_user
    ON channel_project_mappings(user_id, channel_id);

INSERT INTO channel_project_mappings (channel_id, user_id, computer_id, local_path)
SELECT c.id, computer.owner_id, c.project_computer_id, c.project_path
  FROM channels c
  JOIN computers computer ON computer.id = c.project_computer_id
 WHERE c.project_computer_id IS NOT NULL
   AND c.project_path IS NOT NULL
   AND length(btrim(c.project_path)) > 0
   AND computer.owner_id IS NOT NULL
ON CONFLICT DO NOTHING;

ALTER TABLE agent_runs
    ADD COLUMN project_computer_id UUID REFERENCES computers(id) ON DELETE SET NULL,
    ADD COLUMN project_path TEXT,
    ADD COLUMN project_version TEXT NOT NULL DEFAULT '';
