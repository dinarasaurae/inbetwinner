-- New integrations should inherit the service-level orchestration mode unless
-- a per-integration override is explicitly set later.
ALTER TABLE vk_agent_settings
    ALTER COLUMN orchestration_mode SET DEFAULT '';
