-- Создание таблицы для OAuth провайдеров
CREATE TABLE oauth_providers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(20) NOT NULL, -- 'google', 'yandex'
    provider_user_id VARCHAR(255) NOT NULL, -- ID пользователя в OAuth провайдере
    access_token TEXT, -- токен доступа (может быть длинным)
    refresh_token TEXT, -- токен обновления
    token_expires_at TIMESTAMP WITH TIME ZONE, -- когда истекает access_token
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Ограничения
    CONSTRAINT fk_oauth_providers_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_oauth_provider CHECK (provider IN ('google', 'yandex')),
    CONSTRAINT uq_oauth_provider_user UNIQUE(provider, provider_user_id) -- один аккаунт провайдера = один пользователь
);

-- Индексы
CREATE INDEX idx_oauth_providers_user_id ON oauth_providers(user_id);
CREATE INDEX idx_oauth_providers_provider ON oauth_providers(provider);
CREATE INDEX idx_oauth_providers_provider_user_id ON oauth_providers(provider_user_id);

-- Триггер для обновления updated_at
CREATE TRIGGER update_oauth_providers_updated_at
    BEFORE UPDATE ON oauth_providers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();