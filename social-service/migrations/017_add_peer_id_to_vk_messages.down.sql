DROP INDEX IF EXISTS idx_vk_messages_peer_id;

ALTER TABLE vk_messages
    DROP COLUMN IF EXISTS peer_id;
