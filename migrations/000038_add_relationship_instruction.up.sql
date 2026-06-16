-- 000038: Add instruction column to agent_relationships.
-- Stores free-text delegation criteria (DELEGATE when / Report back with pattern
-- from alook). Previously dropped in 000036, now re-added for agent collaboration.
ALTER TABLE agent_relationships ADD COLUMN IF NOT EXISTS instruction TEXT NOT NULL DEFAULT '';
