-- Migration: 014_create_audit_logs_table
-- Description: Creates the audit_logs table for security audit logging
-- Date: 2026-01-25

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT, -- NULL for system-level events
    event_type TEXT NOT NULL,
    actor TEXT NOT NULL, -- User or system component
    resource TEXT NOT NULL, -- What was affected
    action TEXT NOT NULL, -- What happened
    result TEXT NOT NULL CHECK(result IN ('success', 'failure')),
    details TEXT, -- JSON blob
    ip_address TEXT,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON audit_logs(tenant_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_result ON audit_logs(result);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_audit_actor;
-- DROP INDEX IF EXISTS idx_audit_result;
-- DROP INDEX IF EXISTS idx_audit_timestamp;
-- DROP INDEX IF EXISTS idx_audit_event_type;
-- DROP INDEX IF EXISTS idx_audit_tenant_time;
-- DROP TABLE IF EXISTS audit_logs;
