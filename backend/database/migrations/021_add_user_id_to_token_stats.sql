-- Migration: 021_add_user_id_to_token_stats
-- Description: Adds user_id to token_stats for per-user aggregation
-- Date: 2026-01-26
-- Note: user_id column is already created in the initial table definition
-- Only need to create index (IF NOT EXISTS prevents errors if already exists)

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Index for user-based queries (safe to run even if already exists)
CREATE INDEX IF NOT EXISTS idx_token_stats_user_id ON token_stats(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate token_stats without user_id.
