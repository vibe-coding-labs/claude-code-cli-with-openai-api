-- Migration: add reasoning_effort to api_configs
-- Description: Add reasoning_effort field for o1/o3 model reasoning level configuration

ALTER TABLE api_configs ADD COLUMN reasoning_effort TEXT DEFAULT '';

-- Create index for reasoning_effort queries
CREATE INDEX IF NOT EXISTS idx_api_configs_reasoning_effort ON api_configs(reasoning_effort);
