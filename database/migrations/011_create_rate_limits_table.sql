-- Migration: 011_create_rate_limits_table
-- Description: Creates the rate_limits table for rate limiting configuration
-- Date: 2026-01-25

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS rate_limits (
    id TEXT PRIMARY KEY,
    tenant_id TEXT, -- NULL for global rate limits
    dimension TEXT NOT NULL CHECK(dimension IN ('api_key', 'ip', 'tenant')),
    algorithm TEXT NOT NULL CHECK(algorithm IN ('fixed_window', 'sliding_window', 'token_bucket')),
    [limit] INTEGER NOT NULL,
    window INTEGER NOT NULL, -- Seconds
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_rate_limits_tenant ON rate_limits(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rate_limits_dimension ON rate_limits(dimension);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_rate_limits_dimension;
-- DROP INDEX IF EXISTS idx_rate_limits_tenant;
-- DROP TABLE IF EXISTS rate_limits;
