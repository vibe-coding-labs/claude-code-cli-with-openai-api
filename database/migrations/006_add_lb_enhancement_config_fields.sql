-- Migration: 006_add_lb_enhancement_config_fields
-- Description: Adds enhancement configuration fields to load_balancers table
-- Date: 2024-01-15

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Note: SQLite doesn't support IF NOT EXISTS for ALTER TABLE ADD COLUMN
-- This migration will fail if columns already exist, which is expected behavior
-- If you need to re-run this migration, check if columns exist first

-- Health check configuration fields
ALTER TABLE load_balancers ADD COLUMN health_check_enabled BOOLEAN DEFAULT 1;
ALTER TABLE load_balancers ADD COLUMN health_check_interval INTEGER DEFAULT 30;
ALTER TABLE load_balancers ADD COLUMN failure_threshold INTEGER DEFAULT 3;
ALTER TABLE load_balancers ADD COLUMN recovery_threshold INTEGER DEFAULT 2;
ALTER TABLE load_balancers ADD COLUMN health_check_timeout INTEGER DEFAULT 5;

-- Retry configuration fields
ALTER TABLE load_balancers ADD COLUMN max_retries INTEGER DEFAULT 3;
ALTER TABLE load_balancers ADD COLUMN initial_retry_delay INTEGER DEFAULT 100;
ALTER TABLE load_balancers ADD COLUMN max_retry_delay INTEGER DEFAULT 5000;

-- Circuit breaker configuration fields
ALTER TABLE load_balancers ADD COLUMN circuit_breaker_enabled BOOLEAN DEFAULT 1;
ALTER TABLE load_balancers ADD COLUMN error_rate_threshold REAL DEFAULT 0.5;
ALTER TABLE load_balancers ADD COLUMN circuit_breaker_window INTEGER DEFAULT 60;
ALTER TABLE load_balancers ADD COLUMN circuit_breaker_timeout INTEGER DEFAULT 30;
ALTER TABLE load_balancers ADD COLUMN half_open_requests INTEGER DEFAULT 3;

-- Dynamic weight configuration fields
ALTER TABLE load_balancers ADD COLUMN dynamic_weight_enabled BOOLEAN DEFAULT 0;
ALTER TABLE load_balancers ADD COLUMN weight_update_interval INTEGER DEFAULT 300;

-- Log configuration field
ALTER TABLE load_balancers ADD COLUMN log_level TEXT DEFAULT 'standard' CHECK(log_level IN ('minimal', 'standard', 'detailed'));

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- Note: SQLite does not support DROP COLUMN directly.
-- To rollback, you would need to:
-- 1. Create a new table without these columns
-- 2. Copy data from old table to new table
-- 3. Drop old table
-- 4. Rename new table to old name
--
-- Example rollback (commented out):
--
-- CREATE TABLE load_balancers_backup AS 
-- SELECT id, name, description, strategy, config_nodes, enabled, 
--        anthropic_api_key, created_at, updated_at
-- FROM load_balancers;
--
-- DROP TABLE load_balancers;
--
-- ALTER TABLE load_balancers_backup RENAME TO load_balancers;
