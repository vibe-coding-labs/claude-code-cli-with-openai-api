-- UP Migration
-- Per-config model-name alias mapping. Stored as a JSON object mapping a
-- client-facing (vanity) model name to the real upstream model name, e.g.
-- {"glm-5.1": "astron-code-latest"}. Lets clients (Codex) send a model name
-- their built-in catalog recognizes, while the proxy forwards the real name
-- the upstream provider requires. Applied in the Responses handler.
ALTER TABLE api_configs ADD COLUMN model_mappings TEXT DEFAULT '';

-- DOWN Migration
-- ALTER TABLE api_configs DROP COLUMN model_mappings;
