CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    name VARCHAR(200) NOT NULL,
    slug VARCHAR(200) NOT NULL,
    display_name VARCHAR(200) NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL DEFAULT '',
    model VARCHAR(100) NOT NULL DEFAULT 'gpt-4o-mini',
    temperature DECIMAL(3,2) NOT NULL DEFAULT 0.70,
    max_tokens INT NOT NULL DEFAULT 2000,
    language_code VARCHAR(10) NOT NULL DEFAULT 'ru',
    memory_history_size INT NOT NULL DEFAULT 20,
    rag_top_k INT NOT NULL DEFAULT 5,
    knowledge_namespaces JSONB NOT NULL DEFAULT '[]',
    use_summary BOOLEAN NOT NULL DEFAULT false,
    summarize_every INT NOT NULL DEFAULT 50,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    platform VARCHAR(50) NOT NULL DEFAULT 'all',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_agents_workspace ON agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
