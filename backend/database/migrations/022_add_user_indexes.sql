-- Migration: 022_add_user_indexes
-- Description: Adds indexes for user role/status/created_at
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- DROP INDEX IF EXISTS idx_users_created_at;
-- DROP INDEX IF EXISTS idx_users_status;
-- DROP INDEX IF EXISTS idx_users_role;
