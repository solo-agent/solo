CREATE TABLE IF NOT EXISTS computer_members (
    computer_id UUID NOT NULL REFERENCES computers(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        VARCHAR(20) NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (computer_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_computer_members_user ON computer_members(user_id);

INSERT INTO computer_members (computer_id, user_id, role, joined_at)
SELECT id, owner_id, 'owner', created_at
FROM computers
WHERE owner_id IS NOT NULL
ON CONFLICT DO NOTHING;
