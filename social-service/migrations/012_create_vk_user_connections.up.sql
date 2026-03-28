-- vk_user_connections stores the user-level OAuth token obtained after the user
-- authorises the inBeTwin app in VK.  This token is used to list the user's
-- admin groups and to fetch their public profile / subscriptions.
-- It is separate from the community (group) token that lives in vk_integrations.

CREATE TABLE IF NOT EXISTS vk_user_connections (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL,
    vk_user_id       BIGINT      NOT NULL,
    -- platform the user used when authorising: web | android | ios
    platform         VARCHAR(20) NOT NULL DEFAULT 'web',
    access_token_enc BYTEA       NOT NULL,
    access_token_iv  BYTEA       NOT NULL,
    scope            TEXT        NOT NULL DEFAULT '',
    connected_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- one VK account per inBeTwin user
    UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_vk_user_conn_user_id ON vk_user_connections (user_id);
