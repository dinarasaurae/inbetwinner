CREATE TABLE IF NOT EXISTS knowledge_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    namespace_id UUID NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    filename VARCHAR(500) NOT NULL,
    content_type VARCHAR(100) NOT NULL DEFAULT 'text/plain',
    content TEXT NOT NULL DEFAULT '',
    chunk_size INT NOT NULL DEFAULT 512,
    chunk_count INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    embed_model VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_docs_workspace ON knowledge_documents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_docs_namespace ON knowledge_documents(namespace_id);

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL,
    chunk_index INT NOT NULL,
    text TEXT NOT NULL,
    token_count INT NOT NULL DEFAULT 0,
    pinecone_id VARCHAR(200) NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('russian', text)) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_chunks_document ON knowledge_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_chunks_workspace ON knowledge_chunks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_chunks_fts ON knowledge_chunks USING GIN(search_vector);
