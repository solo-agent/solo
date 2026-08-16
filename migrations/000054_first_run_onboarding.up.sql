ALTER TABLE users ADD COLUMN onboarding_completed_at TIMESTAMPTZ;

UPDATE users SET onboarding_completed_at = now();
