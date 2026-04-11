-- Stores individual table row texts for BM25 full-text search.
-- Allows searching table knowledge even when Pinecone is not configured.
CREATE TABLE IF NOT EXISTS knowledge_table_rows (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    table_id     UUID         NOT NULL REFERENCES knowledge_tables(id) ON DELETE CASCADE,
    workspace_id UUID         NOT NULL,
    row_index    INT          NOT NULL,
    text         TEXT         NOT NULL,
    pinecone_id  VARCHAR(200) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED
);
CREATE INDEX IF NOT EXISTS idx_table_rows_workspace ON knowledge_table_rows(workspace_id);
CREATE INDEX IF NOT EXISTS idx_table_rows_table    ON knowledge_table_rows(table_id);
CREATE INDEX IF NOT EXISTS idx_table_rows_fts      ON knowledge_table_rows USING gin(search_vector);
