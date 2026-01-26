-- Migration: 019_add_user_id_to_load_balancers
-- Description: Adds user_id ownership to load_balancers
-- Date: 2026-01-26

-- ============================================================================
-- UP Migration
-- ============================================================================

ALTER TABLE load_balancers ADD COLUMN user_id INTEGER;
UPDATE load_balancers
SET user_id = (SELECT id FROM users ORDER BY id LIMIT 1)
WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_load_balancers_user_id ON load_balancers(user_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- SQLite does not support DROP COLUMN directly.
-- To rollback, recreate load_balancers without user_id.
