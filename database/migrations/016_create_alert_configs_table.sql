-- Create alert_configs table for alert manager
CREATE TABLE IF NOT EXISTS alert_configs (
    tenant_id TEXT PRIMARY KEY,
    enable_quota_alerts BOOLEAN DEFAULT TRUE,
    warning_threshold REAL DEFAULT 0.80,
    critical_threshold REAL DEFAULT 0.95,
    webhook_url TEXT,
    email_address TEXT,
    duplicate_window INTEGER DEFAULT 3600, -- Seconds
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create index on tenant_id
CREATE INDEX IF NOT EXISTS idx_alert_configs_tenant ON alert_configs(tenant_id);
