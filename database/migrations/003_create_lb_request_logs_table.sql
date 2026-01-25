-- Migration: 003_create_lb_request_logs_table
-- Description: Creates the load_balancer_request_logs table for tracking load balancer requests
-- Date: 2024-01-15

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS load_balancer_request_logs (
    id TEXT PRIMARY KEY,
    load_balancer_id TEXT NOT NULL,
    selected_config_id TEXT NOT NULL,
    request_time DATETIME NOT NULL,
    response_time DATETIME NOT NULL,
    duration_ms INTEGER NOT NULL,
    status_code INTEGER NOT NULL,
    success BOOLEAN NOT NULL,
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    request_summary TEXT,
    response_preview TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_lb_request_logs_lb_id ON load_balancer_request_logs(load_balancer_id);
CREATE INDEX IF NOT EXISTS idx_lb_request_logs_time ON load_balancer_request_logs(request_time);
CREATE INDEX IF NOT EXISTS idx_lb_request_logs_config_id ON load_balancer_request_logs(selected_config_id);
CREATE INDEX IF NOT EXISTS idx_lb_request_logs_success ON load_balancer_request_logs(success);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_lb_request_logs_success;
-- DROP INDEX IF EXISTS idx_lb_request_logs_config_id;
-- DROP INDEX IF EXISTS idx_lb_request_logs_time;
-- DROP INDEX IF EXISTS idx_lb_request_logs_lb_id;
-- DROP TABLE IF EXISTS load_balancer_request_logs;
