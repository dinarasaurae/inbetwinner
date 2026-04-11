ALTER TABLE knowledge_table_rows DROP COLUMN search_vector;
ALTER TABLE knowledge_table_rows
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED;

ALTER TABLE knowledge_chunks DROP COLUMN search_vector;
ALTER TABLE knowledge_chunks
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED;
