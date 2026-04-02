-- Revert multilingual BM25 fix
ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS search_vector;
ALTER TABLE knowledge_chunks
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('russian', text)) STORED;
CREATE INDEX IF NOT EXISTS idx_chunks_fts ON knowledge_chunks USING GIN(search_vector);

ALTER TABLE qa_pairs DROP COLUMN IF EXISTS search_vector;
ALTER TABLE qa_pairs
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('russian', question || ' ' || answer)) STORED;
CREATE INDEX IF NOT EXISTS idx_qa_fts ON qa_pairs USING GIN(search_vector);

-- Remove storage columns
ALTER TABLE knowledge_documents
    DROP COLUMN IF EXISTS minio_key,
    DROP COLUMN IF EXISTS file_size;
