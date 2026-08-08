ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ;
UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL;

CREATE TABLE auth_email_challenges (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) NOT NULL,
    purpose         VARCHAR(24) NOT NULL CHECK (purpose IN ('register', 'password_reset')),
    code_hash       VARCHAR(64) NOT NULL,
    display_name    VARCHAR(100),
    password_hash   VARCHAR(255),
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_email_challenges_lookup
    ON auth_email_challenges (email, purpose, created_at DESC);
CREATE INDEX idx_auth_email_challenges_expiry
    ON auth_email_challenges (expires_at);
