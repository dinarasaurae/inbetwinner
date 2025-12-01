-- Откат миграции - удаление таблицы OAuth провайдеров
DROP TRIGGER IF EXISTS update_oauth_providers_updated_at ON oauth_providers;
DROP TABLE IF EXISTS oauth_providers;