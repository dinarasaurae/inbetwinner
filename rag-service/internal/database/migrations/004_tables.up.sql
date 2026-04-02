CREATE TABLE IF NOT EXISTS knowledge_tables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    namespace_id UUID NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    spreadsheet_id VARCHAR(200) NOT NULL DEFAULT '',
    sheet_name VARCHAR(200) NOT NULL DEFAULT '',
    row_count INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    last_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, spreadsheet_id, sheet_name)
);
CREATE INDEX IF NOT EXISTS idx_tables_workspace ON knowledge_tables(workspace_id);
