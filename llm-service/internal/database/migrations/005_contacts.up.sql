CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    chat_user_id VARCHAR(200) NOT NULL,
    platform VARCHAR(50) NOT NULL DEFAULT 'api',
    name VARCHAR(200) NOT NULL DEFAULT '',
    email VARCHAR(200) NOT NULL DEFAULT '',
    phone VARCHAR(50) NOT NULL DEFAULT '',
    company VARCHAR(200) NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, chat_user_id)
);
CREATE INDEX IF NOT EXISTS idx_contacts_workspace ON contacts(workspace_id);
