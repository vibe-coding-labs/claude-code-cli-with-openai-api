-- Migration: 021_add_user_id_to_token_stats
-- Description: Adds user_id to token_stats for per-user aggregation
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

ALTER TABLE token_stats ADD COLUMN user_id INTEGER;
UPDATE token_stats
SET user_id = (SELECT id FROM users ORDER BY id LIMIT 1)
WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_token_stats_user_id ON token_stats(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate token_stats without user_id.
