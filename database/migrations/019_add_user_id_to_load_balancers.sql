-- Migration: 019_add_user_id_to_load_balancers
-- Description: Adds user_id ownership to load_balancers
-- Date: 2026-01-26
-- Note: user_id column is already created in the initial table definition
-- Only need to create index (IF NOT EXISTS prevents errors if already exists)

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Index for user-based queries (safe to run even if already exists)
CREATE INDEX IF NOT EXISTS idx_load_balancers_user_id ON load_balancers(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate load_balancers without user_id.
