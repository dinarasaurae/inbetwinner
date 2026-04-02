CREATE TABLE vk_agent_settings (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id      UUID        NOT NULL UNIQUE REFERENCES vk_integrations(id) ON DELETE CASCADE,
    draft_first         BOOLEAN     NOT NULL DEFAULT TRUE,
    auto_reply_enabled  BOOLEAN     NOT NULL DEFAULT FALSE,
    safe_intents        TEXT[]      NOT NULL DEFAULT ARRAY['faq','hours','basic_prices','qualification']::TEXT[],
    tone_of_voice       TEXT        NOT NULL DEFAULT '',
    forbidden_promises  TEXT[]      NOT NULL DEFAULT ARRAY[]::TEXT[],
    escalation_policy   TEXT        NOT NULL DEFAULT '',
    rag_enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION vk_agent_settings_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vk_agent_settings_updated_at
    BEFORE UPDATE ON vk_agent_settings
    FOR EACH ROW EXECUTE FUNCTION vk_agent_settings_set_updated_at();
