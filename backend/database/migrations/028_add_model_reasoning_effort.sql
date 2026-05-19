-- Migration: add per-model reasoning effort fields
-- Description: Add reasoning_effort fields for each model type (big, middle, small)

-- Add reasoning effort fields for each model
ALTER TABLE api_configs ADD COLUMN big_model_reasoning_effort TEXT DEFAULT '';
ALTER TABLE api_configs ADD COLUMN middle_model_reasoning_effort TEXT DEFAULT '';
ALTER TABLE api_configs ADD COLUMN small_model_reasoning_effort TEXT DEFAULT '';

-- Create indexes for the new fields
CREATE INDEX IF NOT EXISTS idx_api_configs_big_model_reasoning_effort ON api_configs(big_model_reasoning_effort);
CREATE INDEX IF NOT EXISTS idx_api_configs_middle_model_reasoning_effort ON api_configs(middle_model_reasoning_effort);
CREATE INDEX IF NOT EXISTS idx_api_configs_small_model_reasoning_effort ON api_configs(small_model_reasoning_effort);
