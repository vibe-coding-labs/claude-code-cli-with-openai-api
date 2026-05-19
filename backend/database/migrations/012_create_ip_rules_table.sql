-- Migration: 012_create_ip_rules_table
-- Description: Creates the ip_rules table for IP access control
-- Date: 2026-01-25

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS ip_rules (
    id TEXT PRIMARY KEY,
    tenant_id TEXT, -- NULL for global IP rules
    rule_type TEXT NOT NULL CHECK(rule_type IN ('whitelist', 'blacklist')),
    ip_address TEXT NOT NULL, -- Supports CIDR notation
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_ip_rules_tenant ON ip_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ip_rules_type ON ip_rules(rule_type);
CREATE INDEX IF NOT EXISTS idx_ip_rules_ip ON ip_rules(ip_address);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_ip_rules_ip;
-- DROP INDEX IF EXISTS idx_ip_rules_type;
-- DROP INDEX IF EXISTS idx_ip_rules_tenant;
-- DROP TABLE IF EXISTS ip_rules;
