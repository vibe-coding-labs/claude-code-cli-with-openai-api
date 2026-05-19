-- Migration: 001_create_health_statuses_table
-- Description: Creates the health_statuses table for tracking configuration node health
-- Date: 2024-01-15

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS health_statuses (
    config_id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK(status IN ('healthy', 'unhealthy', 'unknown')),
    last_check_time DATETIME NOT NULL,
    consecutive_successes INTEGER DEFAULT 0,
    consecutive_failures INTEGER DEFAULT 0,
    last_error TEXT,
    response_time_ms INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
);

-- Create index for faster queries
CREATE INDEX IF NOT EXISTS idx_health_statuses_status ON health_statuses(status);
CREATE INDEX IF NOT EXISTS idx_health_statuses_last_check ON health_statuses(last_check_time);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_health_statuses_last_check;
-- DROP INDEX IF EXISTS idx_health_statuses_status;
-- DROP TABLE IF EXISTS health_statuses;
