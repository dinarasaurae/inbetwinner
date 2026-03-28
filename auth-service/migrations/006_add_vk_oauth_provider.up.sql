ALTER TABLE oauth_providers DROP CONSTRAINT chk_oauth_provider;
ALTER TABLE oauth_providers ADD CONSTRAINT chk_oauth_provider
  CHECK (provider IN ('google', 'yandex', 'vk'));
