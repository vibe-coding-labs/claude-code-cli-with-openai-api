-- Migration: 005_create_alerts_table
-- Description: Creates the alerts table for load balancer alerting system
-- Date: 2024-01-15

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    load_balancer_id TEXT NOT NULL,
    level TEXT NOT NULL CHECK(level IN ('critical', 'warning', 'info')),
    type TEXT NOT NULL CHECK(type IN ('all_nodes_down', 'low_healthy_nodes', 'high_error_rate', 'circuit_breaker_open', 'node_health_change')),
    message TEXT NOT NULL,
    details TEXT,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_alerts_lb_id ON alerts(load_balancer_id);
CREATE INDEX IF NOT EXISTS idx_alerts_acknowledged ON alerts(acknowledged);
CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at);
CREATE INDEX IF NOT EXISTS idx_alerts_level ON alerts(level);
CREATE INDEX IF NOT EXISTS idx_alerts_type ON alerts(type);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_alerts_type;
-- DROP INDEX IF EXISTS idx_alerts_level;
-- DROP INDEX IF EXISTS idx_alerts_created_at;
-- DROP INDEX IF EXISTS idx_alerts_acknowledged;
-- DROP INDEX IF EXISTS idx_alerts_lb_id;
-- DROP TABLE IF EXISTS alerts;
