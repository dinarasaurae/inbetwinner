-- Stores incoming private messages received via MTProto userbot.
-- Analogous to vk_messages — one row per inbound DM from a lead.
CREATE TABLE IF NOT EXISTS telegram_dm_messages (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID        NOT NULL,               -- inBeTwin user who owns the session
    tg_sender_id      BIGINT      NOT NULL,               -- Telegram user_id of the sender
    tg_sender_username VARCHAR(255),
    tg_message_id     INT         NOT NULL,               -- Telegram message ID (for dedup)
    text              TEXT        NOT NULL,
    -- Agent response
    reply_text        TEXT,
    reply_sent_at     TIMESTAMPTZ,
    reply_error       TEXT,
    -- Orchestration
    mode              VARCHAR(50),                        -- auto_reply | draft | escalate
    intent            VARCHAR(100),
    lead_score        INT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(workspace_id, tg_message_id)
);

CREATE INDEX idx_tg_dm_workspace ON telegram_dm_messages(workspace_id, created_at DESC);
CREATE INDEX idx_tg_dm_sender    ON telegram_dm_messages(workspace_id, tg_sender_id);
