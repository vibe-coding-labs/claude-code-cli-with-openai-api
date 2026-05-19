-- Migration: add retry backoff configuration fields
-- Description: Add retry_backoff_base and retry_backoff_max for exponential backoff configuration

ALTER TABLE api_configs ADD COLUMN retry_backoff_base REAL DEFAULT 1.0;
ALTER TABLE api_configs ADD COLUMN retry_backoff_max INTEGER DEFAULT 60;

-- Create indexes for the new fields
CREATE INDEX IF NOT EXISTS idx_api_configs_retry_backoff_base ON api_configs(retry_backoff_base);
CREATE INDEX IF NOT EXISTS idx_api_configs_retry_backoff_max ON api_configs(retry_backoff_max);
