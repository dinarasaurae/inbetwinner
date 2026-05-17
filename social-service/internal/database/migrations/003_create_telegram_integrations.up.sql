CREATE TABLE telegram_integrations (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID        NOT NULL,
    consent_id       UUID        NOT NULL REFERENCES user_consents(id),

    channel_id       BIGINT      NOT NULL,
    channel_username VARCHAR(255),
    channel_title    TEXT        NOT NULL,

    -- AES-256-GCM encrypted platform bot token
    bot_token_enc    BYTEA       NOT NULL,
    bot_token_iv     BYTEA       NOT NULL,

    status           VARCHAR(20) NOT NULL DEFAULT 'active',
    last_synced_at   TIMESTAMPTZ,
    last_message_id  BIGINT,
    sync_error       TEXT,

    connected_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disconnected_at  TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_integration_status CHECK (status IN ('active', 'revoked', 'error')),
    CONSTRAINT uq_user_channel UNIQUE (user_id, channel_id)
);

CREATE INDEX idx_tg_integrations_user_id ON telegram_integrations(user_id);
CREATE INDEX idx_tg_integrations_channel ON telegram_integrations(channel_id);
CREATE INDEX idx_tg_integrations_status  ON telegram_integrations(status);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_tg_integrations_updated_at
    BEFORE UPDATE ON telegram_integrations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
