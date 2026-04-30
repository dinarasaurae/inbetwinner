-- Add VK-specific enrichment columns to digital_twins:
-- vk_groups    — names of public VK communities the user is a member of
-- vk_wall_posts — last N wall post texts (for interest/intent inference)
ALTER TABLE digital_twins
    ADD COLUMN IF NOT EXISTS vk_groups     JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS vk_wall_posts JSONB NOT NULL DEFAULT '[]';
