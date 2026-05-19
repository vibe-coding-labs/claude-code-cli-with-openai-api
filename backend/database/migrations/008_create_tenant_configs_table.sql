-- Migration: 008_create_tenant_configs_table
-- Description: Creates the tenant_configs table for tenant-specific configurations
-- Date: 2026-01-25

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenant_configs (
    tenant_id TEXT PRIMARY KEY,
    allowed_models TEXT, -- JSON array
    default_model TEXT,
    custom_rate_limits BOOLEAN DEFAULT FALSE,
    require_hmac BOOLEAN DEFAULT FALSE,
    webhook_url TEXT,
    alert_email TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create index for faster queries
CREATE INDEX IF NOT EXISTS idx_tenant_configs_tenant ON tenant_configs(tenant_id);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_tenant_configs_tenant;
-- DROP TABLE IF EXISTS tenant_configs;
