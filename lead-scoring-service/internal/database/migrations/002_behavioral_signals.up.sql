CREATE TABLE IF NOT EXISTS behavioral_signals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL,
    lead_id       TEXT NOT NULL,
    signal_type   TEXT NOT NULL,
    weight        INTEGER NOT NULL DEFAULT 0,
    message       TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_behavioral_signals_lead ON behavioral_signals (workspace_id, lead_id);
CREATE INDEX IF NOT EXISTS idx_behavioral_signals_type ON behavioral_signals (workspace_id, signal_type);
