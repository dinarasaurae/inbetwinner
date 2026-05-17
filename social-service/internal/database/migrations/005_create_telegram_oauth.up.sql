-- Stores the result of Telegram Login Widget OAuth.
-- Created when a user clicks "Connect Telegram" and authenticates via the widget.
-- Separate from channel connection — a user authenticates once, then can connect
-- any channel they administer.

CREATE TABLE telegram_oauth (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID        NOT NULL,        -- platform user ID (from JWT)
    tg_user_id    BIGINT      NOT NULL,        -- Telegram numeric user ID
    tg_username   VARCHAR(255),               -- @handle, nullable
    tg_first_name TEXT,
    tg_last_name  TEXT,
    tg_photo_url  TEXT,
    linked_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One Telegram identity per platform user
    CONSTRAINT uq_telegram_oauth_user UNIQUE (user_id)
);

CREATE INDEX idx_telegram_oauth_user_id    ON telegram_oauth(user_id);
CREATE INDEX idx_telegram_oauth_tg_user_id ON telegram_oauth(tg_user_id);

CREATE TRIGGER trg_telegram_oauth_updated_at
    BEFORE UPDATE ON telegram_oauth
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
