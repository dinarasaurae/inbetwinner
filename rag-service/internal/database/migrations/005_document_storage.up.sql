-- Add MinIO storage columns to knowledge_documents
ALTER TABLE knowledge_documents
    ADD COLUMN IF NOT EXISTS minio_key  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS file_size  BIGINT  NOT NULL DEFAULT 0;

-- ── Multilingual BM25 fix ────────────────────────────────────────────────────
-- Replace language-specific 'russian' tsvector config with 'simple' so that
-- documents and queries in any language (English, Russian, mixed) are indexed
-- and searched consistently. 'simple' lowercases tokens without stemming —
-- good enough for keyword search and avoids language-detection errors.

-- knowledge_chunks
ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS search_vector;
ALTER TABLE knowledge_chunks
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED;
CREATE INDEX IF NOT EXISTS idx_chunks_fts_simple ON knowledge_chunks USING GIN(search_vector);

-- qa_pairs
ALTER TABLE qa_pairs DROP COLUMN IF EXISTS search_vector;
ALTER TABLE qa_pairs
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', question || ' ' || answer)) STORED;
CREATE INDEX IF NOT EXISTS idx_qa_fts_simple ON qa_pairs USING GIN(search_vector);
