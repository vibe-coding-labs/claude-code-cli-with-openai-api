-- backend/database/migrations/040_add_log_storage_settings.sql
-- UP Migration
-- Add log storage management settings with sensible defaults

-- Log retention period in days (default: 30, range: 1-365)
-- Note: log_retention_days is also seeded by migration 036; this INSERT OR IGNORE preserves user modifications.
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('log_retention_days', '30');

-- Maximum database size in GB before triggering cleanup (default: 10, range: 1-1000)
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('max_db_size_gb', '10');

-- Body storage mode: 'file' (store on disk, recommended) or 'inline' (store in DB, legacy)
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('log_body_storage', 'file');

-- Proxy error retention period in days (default: 30)
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('proxy_error_retention_days', '30');

-- DOWN Migration
DELETE FROM system_settings WHERE key IN ('max_db_size_gb', 'log_body_storage', 'proxy_error_retention_days');
