CREATE TABLE IF NOT EXISTS zoho_integrations (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID        NOT NULL UNIQUE,
    -- api_domain returned by Zoho token endpoint (datacenter-specific, e.g. https://www.zohoapis.com)
    api_domain     TEXT        NOT NULL DEFAULT 'https://www.zohoapis.com',
    org_id         VARCHAR(100),
    org_name       VARCHAR(200),
    access_token   TEXT        NOT NULL,
    refresh_token  TEXT        NOT NULL,
    token_expiry   TIMESTAMPTZ NOT NULL,
    is_active      BOOLEAN     NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
