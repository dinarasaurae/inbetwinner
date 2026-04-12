-- Stores FCM device tokens for push notifications.
-- One user can have multiple devices (phone + tablet, Android + iOS).
CREATE TABLE IF NOT EXISTS device_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT        NOT NULL,
    platform    VARCHAR(10) NOT NULL DEFAULT 'android', -- 'android' | 'ios'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, token)
);

CREATE INDEX IF NOT EXISTS idx_device_tokens_user_id ON device_tokens(user_id);
