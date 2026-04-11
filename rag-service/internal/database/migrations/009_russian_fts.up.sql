-- Switch full-text search vectors from 'simple' to 'russian' stemming.
-- 'russian' config: "дозировке" ↔ "дозировка" → same stem "дозировк"
--                  "принимать" ↔ "принимаю"   → same stem "принима"
-- This allows natural-language queries to match stored forms correctly.

-- knowledge_table_rows
ALTER TABLE knowledge_table_rows DROP COLUMN search_vector;
ALTER TABLE knowledge_table_rows
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('russian', text)) STORED;
CREATE INDEX IF NOT EXISTS idx_table_rows_fts_ru ON knowledge_table_rows USING gin(search_vector);

-- knowledge_chunks (documents)
ALTER TABLE knowledge_chunks DROP COLUMN search_vector;
ALTER TABLE knowledge_chunks
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('russian', text)) STORED;
CREATE INDEX IF NOT EXISTS idx_chunks_fts_ru ON knowledge_chunks USING gin(search_vector);
