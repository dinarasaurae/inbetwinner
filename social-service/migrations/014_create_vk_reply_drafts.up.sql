CREATE TABLE vk_reply_drafts (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id      UUID         NOT NULL REFERENCES vk_integrations(id) ON DELETE CASCADE,
    inbound_message_id  UUID         NOT NULL UNIQUE REFERENCES vk_messages(id) ON DELETE CASCADE,
    from_vk_user_id     BIGINT       NOT NULL,
    intent              VARCHAR(64)  NOT NULL DEFAULT 'unknown',
    confidence          DOUBLE PRECISION NOT NULL DEFAULT 0,
    safe_intent         BOOLEAN      NOT NULL DEFAULT FALSE,
    status              VARCHAR(32)  NOT NULL DEFAULT 'pending',
    source              VARCHAR(32)  NOT NULL DEFAULT 'rules',
    draft_text          TEXT         NOT NULL,
    rationale           TEXT         NOT NULL DEFAULT '',
    knowledge_snippets  JSONB        NOT NULL DEFAULT '[]'::jsonb,
    sent_message_id     BIGINT,
    approved_by         UUID,
    generated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    approved_at         TIMESTAMPTZ,
    sent_at             TIMESTAMPTZ
);

CREATE INDEX idx_vk_reply_drafts_integration_status ON vk_reply_drafts(integration_id, status);
CREATE INDEX idx_vk_reply_drafts_from_user ON vk_reply_drafts(from_vk_user_id);
