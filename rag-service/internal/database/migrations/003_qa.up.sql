CREATE TABLE IF NOT EXISTS qa_pairs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    namespace_id UUID NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    tags JSONB NOT NULL DEFAULT '[]',
    is_strict BOOLEAN NOT NULL DEFAULT false,
    pinecone_id VARCHAR(200),
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('russian', question || ' ' || answer)) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_qa_workspace ON qa_pairs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_qa_namespace ON qa_pairs(namespace_id);
CREATE INDEX IF NOT EXISTS idx_qa_fts ON qa_pairs USING GIN(search_vector);
