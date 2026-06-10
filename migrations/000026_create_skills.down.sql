-- Drop in reverse dependency order: leaves (no incoming FKs) first, parent (skills) last.
-- All FKs use ON DELETE CASCADE for row-level cleanup; table-level DROP just needs the
-- referenced tables gone first. Indexes / UNIQUE / PK constraints are auto-dropped with
-- their owning tables.
DROP TABLE IF EXISTS agent_skills;
DROP TABLE IF EXISTS skill_files;
DROP TABLE IF EXISTS skills;
