-- Digital twin of each VK lead (enriched from their public profile + wall).
CREATE TABLE vk_lead_profiles (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id   UUID        NOT NULL REFERENCES vk_integrations(id) ON DELETE CASCADE,
    vk_user_id       BIGINT      NOT NULL,
    first_name       VARCHAR(100),
    last_name        VARCHAR(100),
    sex              SMALLINT,          -- 1=female, 2=male, 0=unknown
    bdate            VARCHAR(20),       -- "D.M" or "D.M.YYYY"
    city             VARCHAR(100),
    country          VARCHAR(100),
    about            TEXT,
    status           TEXT,
    domain           VARCHAR(100),
    photo_url        TEXT,
    followers_count  INT,
    occupation_type  VARCHAR(50),       -- work / university / school
    occupation_name  VARCHAR(255),
    last_enriched_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (integration_id, vk_user_id)
);

CREATE INDEX idx_vk_leads_integration ON vk_lead_profiles(integration_id);
CREATE INDEX idx_vk_leads_user_id     ON vk_lead_profiles(vk_user_id);
