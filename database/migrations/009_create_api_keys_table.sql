-- Migration: 009_create_api_keys_table
-- Description: Creates the api_keys table for API key management
-- Date: 2026-01-25

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    key_hash TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    name TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'expired', 'revoked')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    hmac_secret TEXT,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_status ON api_keys(status);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_expires_at ON api_keys(expires_at);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_api_keys_expires_at;
-- DROP INDEX IF EXISTS idx_api_keys_key_hash;
-- DROP INDEX IF EXISTS idx_api_keys_status;
-- DROP INDEX IF EXISTS idx_api_keys_tenant;
-- DROP TABLE IF EXISTS api_keys;
