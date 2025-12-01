-- Создание таблицы для хранения refresh токенов
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    device_info TEXT, -- информация об устройстве (браузер, мобильное приложение и т.д.)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Ограничения
    CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Индексы для быстрого поиска
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

-- Автоматическое удаление истекших токенов (опционально, можно запускать по cron)
-- CREATE INDEX idx_refresh_tokens_expired ON refresh_tokens(expires_at) WHERE expires_at < NOW();