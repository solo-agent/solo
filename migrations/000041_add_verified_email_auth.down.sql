DROP TABLE IF EXISTS auth_email_challenges;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified_at;
