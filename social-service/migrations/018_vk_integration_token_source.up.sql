-- Track whether the group token was saved manually (VK panel) or via OAuth.
-- Manual tokens have 'messages' permission; OAuth tokens from VK ID apps do not.
ALTER TABLE vk_integrations
    ADD COLUMN IF NOT EXISTS token_source VARCHAR(20) NOT NULL DEFAULT 'oauth';

-- Mark existing integrations as 'oauth' (safe default).
UPDATE vk_integrations SET token_source = 'oauth' WHERE token_source IS NULL OR token_source = '';
