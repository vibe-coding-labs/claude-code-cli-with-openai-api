-- Migration: 020_add_user_id_to_request_logs
-- Description: Adds user_id to request_logs for per-user usage tracking
-- Date: 2026-01-26
-- Note: user_id column is already created in the initial table definition
-- Only need to create index (IF NOT EXISTS prevents errors if already exists)

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Index for user-based queries (safe to run even if already exists)
CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON request_logs(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate request_logs without user_id.
