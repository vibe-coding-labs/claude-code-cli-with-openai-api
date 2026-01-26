-- Migration: 017_add_user_role_status
-- Description: Adds role and status fields to users table
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin' CHECK(role IN ('admin', 'user'));
ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'disabled'));

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, you would need to recreate the users table without role/status.
