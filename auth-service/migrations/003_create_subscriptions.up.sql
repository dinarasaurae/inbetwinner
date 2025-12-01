-- Создание таблицы подписок
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan VARCHAR(20) NOT NULL, -- 'free', 'pro', 'business'
    status VARCHAR(20) DEFAULT 'active', -- 'active', 'cancelled', 'expired', 'past_due'
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    payment_provider VARCHAR(50), -- 'stripe', 'yookassa', null для free плана
    external_subscription_id VARCHAR(255), -- ID подписки в платежной системе
    trial_ends_at TIMESTAMP WITH TIME ZONE, -- окончание trial периода
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Ограничения
    CONSTRAINT fk_subscriptions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_subscription_plan CHECK (plan IN ('free', 'pro', 'business')),
    CONSTRAINT chk_subscription_status CHECK (status IN ('active', 'cancelled', 'expired', 'past_due', 'trialing'))
);

-- Индексы
CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_expires_at ON subscriptions(expires_at);
CREATE INDEX idx_subscriptions_external_id ON subscriptions(external_subscription_id);

-- Триггер для обновления updated_at
CREATE TRIGGER update_subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();