-- Migration 000023: Add system info columns to computers table.
-- Stores OS, hostname, and IP reported by the daemon on register/heartbeat.

ALTER TABLE computers ADD COLUMN os TEXT DEFAULT '';
ALTER TABLE computers ADD COLUMN hostname TEXT DEFAULT '';
ALTER TABLE computers ADD COLUMN ip TEXT DEFAULT '';
