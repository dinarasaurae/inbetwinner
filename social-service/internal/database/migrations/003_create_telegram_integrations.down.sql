DROP TRIGGER IF EXISTS trg_tg_integrations_updated_at ON telegram_integrations;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS telegram_integrations;
