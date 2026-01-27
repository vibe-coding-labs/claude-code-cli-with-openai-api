-- Migration: 024_add_composite_indexes
-- Description: Adds composite indexes for request_logs and token_stats
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_request_logs_user_created ON request_logs(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_token_stats_user_created ON token_stats(user_id, created_at);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- DROP INDEX IF EXISTS idx_token_stats_user_created;
-- DROP INDEX IF EXISTS idx_request_logs_user_created;
