-- backend/database/migrations/040_add_log_storage_settings.sql
-- UP Migration
-- Add log storage management settings with sensible defaults

INSERT OR IGNORE INTO system_settings (key, value) VALUES ('log_retention_days', '30');
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('max_db_size_gb', '10');
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('log_body_storage', 'file');
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('proxy_error_retention_days', '30');

-- DOWN Migration
DELETE FROM system_settings WHERE key IN ('log_retention_days', 'max_db_size_gb', 'log_body_storage', 'proxy_error_retention_days');
