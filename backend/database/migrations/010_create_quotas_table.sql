-- Migration: 010_create_quotas_table
-- Description: Creates the quotas table for tenant quota management
-- Date: 2026-01-25

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS quotas (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    quota_type TEXT NOT NULL CHECK(quota_type IN ('requests', 'tokens', 'cost')),
    period TEXT NOT NULL CHECK(period IN ('daily', 'monthly')),
    [limit] INTEGER NOT NULL,
    current_usage INTEGER NOT NULL DEFAULT 0,
    reset_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE(tenant_id, quota_type, period)
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_quotas_tenant ON quotas(tenant_id);
CREATE INDEX IF NOT EXISTS idx_quotas_reset ON quotas(reset_at);
CREATE INDEX IF NOT EXISTS idx_quotas_tenant_type ON quotas(tenant_id, quota_type);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_quotas_tenant_type;
-- DROP INDEX IF EXISTS idx_quotas_reset;
-- DROP INDEX IF EXISTS idx_quotas_tenant;
-- DROP TABLE IF EXISTS quotas;
