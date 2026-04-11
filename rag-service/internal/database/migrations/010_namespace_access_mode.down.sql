ALTER TABLE namespaces
    DROP COLUMN IF EXISTS access_mode,
    DROP COLUMN IF EXISTS has_documents,
    DROP COLUMN IF EXISTS has_tables,
    DROP COLUMN IF EXISTS has_qa;
