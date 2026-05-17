CREATE TABLE user_consents (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID        NOT NULL,
    platform     VARCHAR(30) NOT NULL,
    scope        TEXT[]      NOT NULL,
    granted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at   TIMESTAMPTZ,
    consent_text TEXT        NOT NULL,
    ip_address   INET,
    user_agent   TEXT,

    CONSTRAINT chk_consent_platform CHECK (platform IN ('telegram'))
);

CREATE INDEX idx_user_consents_user_platform
    ON user_consents(user_id, platform);

-- One active consent per user per platform
CREATE UNIQUE INDEX uq_user_consent_active
    ON user_consents(user_id, platform)
    WHERE revoked_at IS NULL;
