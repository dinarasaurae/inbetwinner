DROP TABLE IF EXISTS web_sources;
DROP TABLE IF EXISTS google_oauth_tokens;
ALTER TABLE knowledge_tables DROP COLUMN IF EXISTS google_connected;
