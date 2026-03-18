-- Posts imported from the connected VK group's wall.
CREATE TABLE vk_posts (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id UUID        NOT NULL REFERENCES vk_integrations(id) ON DELETE CASCADE,
    user_id        UUID        NOT NULL,
    vk_post_id     BIGINT      NOT NULL,
    owner_id       BIGINT      NOT NULL,   -- negative = group
    text           TEXT,
    media_type     VARCHAR(50),            -- photo, video, doc, audio, link, poll, ...
    has_media      BOOLEAN     NOT NULL DEFAULT FALSE,
    likes_count    INT,
    reposts_count  INT,
    views_count    INT,
    comments_count INT,
    extracted_urls TEXT[],
    posted_at      TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (integration_id, vk_post_id)
);

CREATE INDEX idx_vk_posts_integration_id ON vk_posts(integration_id);
CREATE INDEX idx_vk_posts_posted_at      ON vk_posts(posted_at DESC);
