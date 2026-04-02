-- Orchestration mode per integration (legacy | llm_service | hybrid).
-- Defaults to legacy so existing rows are unaffected on upgrade.
ALTER TABLE vk_agent_settings
    ADD COLUMN IF NOT EXISTS orchestration_mode TEXT NOT NULL DEFAULT 'legacy';

-- Observability columns on every draft row.
-- All nullable / defaulted so legacy writes don't have to set them.
ALTER TABLE vk_reply_drafts
    ADD COLUMN IF NOT EXISTS orchestration_source  TEXT    NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS llm_latency_ms        INTEGER,
    ADD COLUMN IF NOT EXISTS fallback_reason       TEXT,
    ADD COLUMN IF NOT EXISTS prompt_tokens         INTEGER,
    ADD COLUMN IF NOT EXISTS completion_tokens     INTEGER,
    ADD COLUMN IF NOT EXISTS used_agent_id         UUID,
    ADD COLUMN IF NOT EXISTS used_tools            JSONB   NOT NULL DEFAULT '[]'::jsonb;
