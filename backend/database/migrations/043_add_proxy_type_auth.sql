-- UP Migration
-- Per-config intermediate proxy type + optional auth.
-- proxy_type: 'auto' (infer from proxy_url scheme; empty/http/https -> HTTP,
--             socks5/socks5h -> SOCKS5) / 'http' / 'https' / 'socks5' / 'socks5h'
-- proxy_username/proxy_password_encrypted: optional credentials (SOCKS5
--   username/password; HTTP proxies get Proxy-Authorization). The password is
--   stored encrypted via database/encryption.go (same mechanism as
--   openai_api_key_encrypted).
ALTER TABLE api_configs ADD COLUMN proxy_type TEXT DEFAULT 'auto';
ALTER TABLE api_configs ADD COLUMN proxy_username TEXT DEFAULT '';
ALTER TABLE api_configs ADD COLUMN proxy_password_encrypted TEXT DEFAULT '';

-- DOWN Migration
-- ALTER TABLE api_configs DROP COLUMN proxy_type;
-- ALTER TABLE api_configs DROP COLUMN proxy_username;
-- ALTER TABLE api_configs DROP COLUMN proxy_password_encrypted;