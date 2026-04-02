CREATE TABLE IF NOT EXISTS lead_scores (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL,
    lead_id       TEXT NOT NULL,
    platform      TEXT NOT NULL DEFAULT 'unknown',
    score         INTEGER NOT NULL DEFAULT 0 CHECK (score >= 0 AND score <= 100),
    score_label   TEXT NOT NULL DEFAULT 'cold',
    last_message  TEXT NOT NULL DEFAULT '',
    message_count INTEGER NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, lead_id)
);

CREATE INDEX IF NOT EXISTS idx_lead_scores_workspace ON lead_scores (workspace_id);
CREATE INDEX IF NOT EXISTS idx_lead_scores_score ON lead_scores (workspace_id, score DESC);
