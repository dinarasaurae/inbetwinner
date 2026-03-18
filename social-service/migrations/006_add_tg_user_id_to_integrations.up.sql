-- Links a connected channel to the Telegram user who authorised it.
ALTER TABLE telegram_integrations
    ADD COLUMN tg_user_id BIGINT;

COMMENT ON COLUMN telegram_integrations.tg_user_id IS
    'Telegram user ID of the channel admin who connected this channel (from telegram_oauth)';
