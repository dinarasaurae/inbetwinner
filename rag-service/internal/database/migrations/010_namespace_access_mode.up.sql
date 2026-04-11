-- Add access_mode and content-type flags to namespaces.
-- access_mode controls whether the namespace is queried automatically (AUTO_QUERY)
-- or only when the agent explicitly calls a tool (TOOL_ONLY).

ALTER TABLE namespaces
    ADD COLUMN IF NOT EXISTS access_mode   VARCHAR(20)  NOT NULL DEFAULT 'AUTO_QUERY',
    ADD COLUMN IF NOT EXISTS has_documents BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS has_tables    BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS has_qa        BOOLEAN      NOT NULL DEFAULT false;

-- Back-fill existing rows based on the 'type' column
UPDATE namespaces SET has_documents = true WHERE type = 'doc';
UPDATE namespaces SET has_tables    = true WHERE type = 'table';
UPDATE namespaces SET has_qa        = true WHERE type = 'qa';
