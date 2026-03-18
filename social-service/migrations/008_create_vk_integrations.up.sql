-- Stores VK community (group) tokens per platform user.
-- Each row = one VK group connected by one inBeTwin user.
CREATE TABLE vk_integrations (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID        NOT NULL,
    group_id          BIGINT      NOT NULL,
    group_token_enc   BYTEA,                          -- AES-256-GCM ciphertext
    group_token_iv    BYTEA,                          -- GCM nonce (12 bytes)
    group_name        VARCHAR(255),
    group_screen_name VARCHAR(100),
    group_photo       TEXT,
    long_poll_ts      VARCHAR(20),                    -- last consumed Long Poll offset
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    connected_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, group_id)
);

CREATE INDEX idx_vk_integrations_user_id ON vk_integrations(user_id);
CREATE INDEX idx_vk_integrations_active  ON vk_integrations(is_active) WHERE is_active = TRUE;

CREATE OR REPLACE FUNCTION vk_integrations_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vk_integrations_updated_at
    BEFORE UPDATE ON vk_integrations
    FOR EACH ROW EXECUTE FUNCTION vk_integrations_set_updated_at();
