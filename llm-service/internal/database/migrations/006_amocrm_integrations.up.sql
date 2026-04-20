CREATE TABLE IF NOT EXISTS amocrm_integrations (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID        NOT NULL UNIQUE,
    subdomain      VARCHAR(100) NOT NULL,
    access_token   TEXT        NOT NULL,
    refresh_token  TEXT        NOT NULL,
    token_expiry   TIMESTAMPTZ NOT NULL,
    account_id     BIGINT,
    email          VARCHAR(200),
    is_active      BOOLEAN     NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
