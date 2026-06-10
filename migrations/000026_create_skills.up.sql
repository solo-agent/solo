-- Migration 000026: Create skill system tables (skills / skill_files / agent_skills).
-- Phase1 of the skill system: disk scanning + DB persistence + agent-skill M:N binding.

CREATE TABLE skills (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100) NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    source_path     TEXT NOT NULL,
    source_kind     VARCHAR(50) NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    body_hash       CHAR(64) NOT NULL,
    discovered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE skill_files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_id    UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(skill_id, path)
);

CREATE TABLE agent_skills (
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    skill_id    UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, skill_id)
);

CREATE INDEX idx_skill_files_skill ON skill_files(skill_id);
CREATE INDEX idx_agent_skills_skill ON agent_skills(skill_id);
