-- Unified digital twin per (workspace, platform, chat_user_id).
-- Aggregates identity, occupation, and cross-platform social signals
-- so the LLM agent can personalise every response.
CREATE TABLE IF NOT EXISTS digital_twins (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID        NOT NULL,
    platform        VARCHAR(20) NOT NULL,   -- "vk" | "telegram" | "facebook" | etc.
    chat_user_id    VARCHAR(200) NOT NULL,  -- platform-specific user id (vk_user_id as string, telegram user_id, fb psid)

    -- Identity
    first_name      VARCHAR(200),
    last_name       VARCHAR(200),
    city            VARCHAR(200),
    country         VARCHAR(200),

    -- Occupation
    company         VARCHAR(500),
    job_title       VARCHAR(200),

    -- Bio / status
    bio             TEXT,
    status_text     TEXT,

    -- Social signals
    followers_count INT         NOT NULL DEFAULT 0,

    -- Pinterest enrichment
    pinterest_url   TEXT,
    pinterest_boards JSONB      NOT NULL DEFAULT '[]',  -- ["Скандинавский интерьер", "DIY"]

    -- Facebook Messenger enrichment
    facebook_name   VARCHAR(200),

    -- Behavioural (updated from lead-scoring-service)
    message_count   INT         NOT NULL DEFAULT 0,
    lead_score      INT         NOT NULL DEFAULT 0,

    last_enriched_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(workspace_id, platform, chat_user_id)
);

CREATE INDEX IF NOT EXISTS idx_digital_twins_workspace
    ON digital_twins(workspace_id);
CREATE INDEX IF NOT EXISTS idx_digital_twins_lookup
    ON digital_twins(workspace_id, platform, chat_user_id);
