-- Migration: 018_add_user_id_to_api_configs
-- Description: Adds user_id ownership to api_configs
-- Date: 2026-01-26
-- Note: user_id column is already created in the initial table definition
-- Only need to create index (IF NOT EXISTS prevents errors if already exists)

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Index for user-based queries (safe to run even if already exists)
CREATE INDEX IF NOT EXISTS idx_api_configs_user_id ON api_configs(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate api_configs without user_id.
