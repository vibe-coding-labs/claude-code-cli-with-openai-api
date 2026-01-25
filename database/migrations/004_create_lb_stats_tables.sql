-- Migration: 004_create_lb_stats_tables
-- Description: Creates load_balancer_stats and node_stats tables for aggregated statistics
-- Date: 2024-01-15

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Load balancer statistics table (aggregated by time bucket)
CREATE TABLE IF NOT EXISTS load_balancer_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    load_balancer_id TEXT NOT NULL,
    time_bucket DATETIME NOT NULL,
    total_requests INTEGER DEFAULT 0,
    success_requests INTEGER DEFAULT 0,
    failed_requests INTEGER DEFAULT 0,
    total_duration_ms INTEGER DEFAULT 0,
    min_duration_ms INTEGER DEFAULT 0,
    max_duration_ms INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE
);

-- Node statistics table (per-node aggregated by time bucket)
CREATE TABLE IF NOT EXISTS node_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    load_balancer_id TEXT NOT NULL,
    config_id TEXT NOT NULL,
    time_bucket DATETIME NOT NULL,
    request_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    total_duration_ms INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE,
    FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_lb_stats_lb_id_time ON load_balancer_stats(load_balancer_id, time_bucket);
CREATE INDEX IF NOT EXISTS idx_lb_stats_time_bucket ON load_balancer_stats(time_bucket);
CREATE INDEX IF NOT EXISTS idx_node_stats_lb_id_time ON node_stats(load_balancer_id, time_bucket);
CREATE INDEX IF NOT EXISTS idx_node_stats_config_id ON node_stats(config_id);
CREATE INDEX IF NOT EXISTS idx_node_stats_time_bucket ON node_stats(time_bucket);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_node_stats_time_bucket;
-- DROP INDEX IF EXISTS idx_node_stats_config_id;
-- DROP INDEX IF EXISTS idx_node_stats_lb_id_time;
-- DROP INDEX IF EXISTS idx_lb_stats_time_bucket;
-- DROP INDEX IF EXISTS idx_lb_stats_lb_id_time;
-- DROP TABLE IF EXISTS node_stats;
-- DROP TABLE IF EXISTS load_balancer_stats;
