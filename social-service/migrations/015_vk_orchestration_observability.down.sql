ALTER TABLE vk_reply_drafts
    DROP COLUMN IF EXISTS orchestration_source,
    DROP COLUMN IF EXISTS llm_latency_ms,
    DROP COLUMN IF EXISTS fallback_reason,
    DROP COLUMN IF EXISTS prompt_tokens,
    DROP COLUMN IF EXISTS completion_tokens,
    DROP COLUMN IF EXISTS used_agent_id,
    DROP COLUMN IF EXISTS used_tools;

ALTER TABLE vk_agent_settings
    DROP COLUMN IF EXISTS orchestration_mode;
