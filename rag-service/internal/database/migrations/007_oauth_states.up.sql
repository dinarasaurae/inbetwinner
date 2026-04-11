-- OAuth pending states (replaces in-memory map, survives restarts)
CREATE TABLE IF NOT EXISTS google_oauth_states (
    state        VARCHAR(64)  PRIMARY KEY,
    workspace_id UUID         NOT NULL,
    expires_at   TIMESTAMPTZ  NOT NULL
);
