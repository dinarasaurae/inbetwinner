CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    chat_user_id VARCHAR(200) NOT NULL,
    platform VARCHAR(50) NOT NULL DEFAULT 'api',
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    tool_name VARCHAR(100),
    tool_call_id VARCHAR(200),
    total_tokens INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cm_workspace_user ON chat_messages(workspace_id, chat_user_id);
CREATE INDEX IF NOT EXISTS idx_cm_created ON chat_messages(created_at DESC);
