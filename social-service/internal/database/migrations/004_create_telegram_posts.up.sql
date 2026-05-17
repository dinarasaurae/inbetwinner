CREATE TABLE telegram_posts (
    id             UUID     PRIMARY KEY DEFAULT uuid_generate_v4(),
    integration_id UUID     NOT NULL REFERENCES telegram_integrations(id) ON DELETE CASCADE,
    user_id        UUID     NOT NULL,

    message_id     BIGINT   NOT NULL,
    channel_id     BIGINT   NOT NULL,

    text           TEXT,
    media_type     VARCHAR(20),
    has_media      BOOLEAN  NOT NULL DEFAULT FALSE,
    link_count     SMALLINT NOT NULL DEFAULT 0,
    extracted_urls TEXT[],

    posted_at      TIMESTAMPTZ NOT NULL,
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    forward_count  INT,
    view_count     INT,

    CONSTRAINT uq_msg_channel UNIQUE (channel_id, message_id)
);

CREATE INDEX idx_tg_posts_integration ON telegram_posts(integration_id);
CREATE INDEX idx_tg_posts_user_id     ON telegram_posts(user_id);
CREATE INDEX idx_tg_posts_posted_at   ON telegram_posts(posted_at DESC);
CREATE INDEX idx_tg_posts_media_type  ON telegram_posts(media_type);
