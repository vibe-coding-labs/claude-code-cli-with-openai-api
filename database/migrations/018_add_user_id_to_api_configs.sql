-- Migration: 018_add_user_id_to_api_configs
-- Description: Adds user_id ownership to api_configs
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

ALTER TABLE api_configs ADD COLUMN user_id INTEGER;
UPDATE api_configs
SET user_id = (SELECT id FROM users ORDER BY id LIMIT 1)
WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_api_configs_user_id ON api_configs(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate api_configs without user_id.
