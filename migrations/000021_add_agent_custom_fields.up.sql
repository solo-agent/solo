-- Migration 000021: Add custom_env and custom_args fields to agents table
-- Allows per-agent environment variables and CLI arguments for multi-agent extension

ALTER TABLE agents
    ADD COLUMN custom_env  JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN custom_args JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN runtime_id  TEXT;
