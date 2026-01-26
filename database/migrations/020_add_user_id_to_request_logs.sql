-- Migration: 020_add_user_id_to_request_logs
-- Description: Adds user_id to request_logs for per-user usage tracking
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

ALTER TABLE request_logs ADD COLUMN user_id INTEGER;
UPDATE request_logs
SET user_id = (SELECT id FROM users ORDER BY id LIMIT 1)
WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON request_logs(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate request_logs without user_id.
