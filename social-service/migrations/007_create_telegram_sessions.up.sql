-- Stores the encrypted MTProto session for each user.
-- A session gives the platform permission to act AS the user:
-- posting to their channels, reading history, replying to comments.
--
-- Security notes:
--   • session_enc is AES-256-GCM encrypted (ENCRYPTION_KEY env var)
--   • session_iv  is the 12-byte GCM nonce
--   • phone numbers are NEVER stored (only used transiently during auth)
--   • revoked_at triggers log-out on Telegram's side + row deletion

CREATE TABLE telegram_sessions (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID        NOT NULL,
    tg_user_id    BIGINT      NOT NULL,
    tg_username   VARCHAR(255),
    tg_first_name TEXT,
    tg_last_name  TEXT,
    session_enc   BYTEA       NOT NULL,
    session_iv    BYTEA       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at    TIMESTAMPTZ,

    CONSTRAINT uq_telegram_session_user UNIQUE (user_id)
);

CREATE INDEX idx_telegram_sessions_user_id ON telegram_sessions(user_id);

-- Add access_hash to telegram_integrations so we can make MTProto calls
-- to the channel without re-resolving the peer on every request.
ALTER TABLE telegram_integrations
    ADD COLUMN IF NOT EXISTS access_hash BIGINT;
