-- Incoming DMs to the connected VK group + outgoing replies.
CREATE TABLE vk_messages (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id          UUID        NOT NULL REFERENCES vk_integrations(id) ON DELETE CASCADE,
    from_vk_user_id         BIGINT      NOT NULL,  -- VK user_id of the lead
    message_id              BIGINT      NOT NULL,
    conversation_message_id BIGINT,
    text                    TEXT,
    attachments             JSONB,
    is_incoming             BOOLEAN     NOT NULL DEFAULT TRUE,
    is_processed            BOOLEAN     NOT NULL DEFAULT FALSE,
    received_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (integration_id, message_id)
);

CREATE INDEX idx_vk_messages_integration ON vk_messages(integration_id);
CREATE INDEX idx_vk_messages_from_user   ON vk_messages(from_vk_user_id);
CREATE INDEX idx_vk_messages_unprocessed ON vk_messages(is_processed) WHERE NOT is_processed;
