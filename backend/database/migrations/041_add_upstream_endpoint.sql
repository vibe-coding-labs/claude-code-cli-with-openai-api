-- UP Migration
-- Per-config upstream endpoint override. Empty means use the global default
-- (UPSTREAM_ENDPOINT env, "chat/completions"). Set to "responses" for upstreams
-- that only implement the OpenAI Responses API (e.g. opencode.ai/zen/go/v1),
-- without forcing every other channel off chat/completions.
ALTER TABLE api_configs ADD COLUMN upstream_endpoint TEXT DEFAULT '';

-- DOWN Migration
-- ALTER TABLE api_configs DROP COLUMN upstream_endpoint;
