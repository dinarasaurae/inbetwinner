ALTER TABLE vk_messages
    ADD COLUMN IF NOT EXISTS peer_id BIGINT;

-- For direct 1:1 dialogs peer_id is equal to the counterparty's VK user id.
-- Backfill existing rows so old drafts can still reply through the dialog id.
UPDATE vk_messages
   SET peer_id = from_vk_user_id
 WHERE peer_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_vk_messages_peer_id ON vk_messages(peer_id);
