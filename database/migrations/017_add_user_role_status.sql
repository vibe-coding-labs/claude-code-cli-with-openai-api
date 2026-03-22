-- Migration: 017_add_user_role_status
-- Description: Adds role and status fields to users table
-- Date: 2026-01-26
-- Note: These columns are already created in CreateUserTable function

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, you would need to recreate the users table without role/status.
