-- Google OAuth tokens (one per workspace)
CREATE TABLE IF NOT EXISTS google_oauth_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID       NOT NULL UNIQUE,
    access_token TEXT       NOT NULL,
    refresh_token TEXT      NOT NULL DEFAULT '',
    expiry      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scope       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Web / URL knowledge sources
CREATE TABLE IF NOT EXISTS web_sources (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID         NOT NULL,
    namespace_id UUID         NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    url          TEXT         NOT NULL,
    title        VARCHAR(500) NOT NULL DEFAULT '',
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
    error_message TEXT,
    chunk_count  INT          NOT NULL DEFAULT 0,
    last_crawl_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, url)
);
CREATE INDEX IF NOT EXISTS idx_web_sources_workspace ON web_sources(workspace_id);

-- Sheets: add google_connected column for OAuth flow tracking
ALTER TABLE knowledge_tables ADD COLUMN IF NOT EXISTS google_connected BOOLEAN NOT NULL DEFAULT FALSE;
