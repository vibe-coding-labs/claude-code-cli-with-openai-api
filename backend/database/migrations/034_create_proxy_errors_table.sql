-- Migration: Create proxy_errors table for structured error logging
-- UP Migration

CREATE TABLE IF NOT EXISTS proxy_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_id TEXT NOT NULL,
    config_name TEXT NOT NULL,
    session_id TEXT,
    request_id TEXT,
    model TEXT NOT NULL,
    upstream_model TEXT,
    error_type TEXT NOT NULL,
    error_category TEXT NOT NULL,
    error_message TEXT NOT NULL,
    upstream_status_code INTEGER,
    upstream_error_body TEXT,
    request_stage TEXT NOT NULL,
    retry_attempt INTEGER DEFAULT 0,
    request_duration_ms INTEGER,
    request_preview TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxy_errors_config_id ON proxy_errors(config_id);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_error_type ON proxy_errors(error_type);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_error_category ON proxy_errors(error_category);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_created_at ON proxy_errors(created_at);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_model ON proxy_errors(model);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_request_stage ON proxy_errors(request_stage);

-- DOWN Migration

DROP INDEX IF EXISTS idx_proxy_errors_config_id;
DROP INDEX IF EXISTS idx_proxy_errors_error_type;
DROP INDEX IF EXISTS idx_proxy_errors_error_category;
DROP INDEX IF EXISTS idx_proxy_errors_created_at;
DROP INDEX IF EXISTS idx_proxy_errors_model;
DROP INDEX IF EXISTS idx_proxy_errors_request_stage;
DROP TABLE IF EXISTS proxy_errors;
