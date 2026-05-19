-- Migration: 031_add_cache_tokens_to_logs
-- Description: Add cache token fields to request_logs and token_stats tables
-- Date: 2026-03-22

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Add cache token columns to request_logs table
ALTER TABLE request_logs ADD COLUMN cache_read_tokens INTEGER DEFAULT 0;
ALTER TABLE request_logs ADD COLUMN cache_write_tokens INTEGER DEFAULT 0;

-- Add cache token columns to token_stats table
ALTER TABLE token_stats ADD COLUMN cache_read_tokens INTEGER DEFAULT 0;
ALTER TABLE token_stats ADD COLUMN cache_write_tokens INTEGER DEFAULT 0;

-- Create indexes for faster queries on cache tokens
CREATE INDEX IF NOT EXISTS idx_request_logs_cache_read ON request_logs(cache_read_tokens);
CREATE INDEX IF NOT EXISTS idx_token_stats_cache_read ON token_stats(cache_read_tokens);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_token_stats_cache_read;
-- DROP INDEX IF EXISTS idx_request_logs_cache_read;
-- ALTER TABLE token_stats DROP COLUMN cache_write_tokens;
-- ALTER TABLE token_stats DROP COLUMN cache_read_tokens;
-- ALTER TABLE request_logs DROP COLUMN cache_write_tokens;
-- ALTER TABLE request_logs DROP COLUMN cache_read_tokens;
