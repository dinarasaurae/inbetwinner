CREATE TABLE IF NOT EXISTS tool_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_name VARCHAR(100) NOT NULL,
    workspace_id UUID NOT NULL,
    chat_user_id VARCHAR(200) NOT NULL,
    arguments JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result JSONB,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    duration_ms INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_te_workspace ON tool_executions(workspace_id);
CREATE INDEX IF NOT EXISTS idx_te_tool ON tool_executions(tool_name);
CREATE INDEX IF NOT EXISTS idx_te_started ON tool_executions(started_at DESC);
