-- Migration: 023_add_username_unique_index
-- Description: Adds unique index for users.username
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- DROP INDEX IF EXISTS idx_users_username;
