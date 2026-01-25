-- Create pricing_tiers table for billing engine
CREATE TABLE IF NOT EXISTS pricing_tiers (
    tenant_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    prompt_token_price REAL NOT NULL DEFAULT 0.01,
    completion_token_price REAL NOT NULL DEFAULT 0.03,
    request_price REAL NOT NULL DEFAULT 0.0001,
    volume_discount_enabled BOOLEAN DEFAULT FALSE,
    volume_discount_tiers TEXT, -- JSON array of discount tiers
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Create index on tenant_id
CREATE INDEX IF NOT EXISTS idx_pricing_tiers_tenant ON pricing_tiers(tenant_id);
