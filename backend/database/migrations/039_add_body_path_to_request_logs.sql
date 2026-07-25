-- backend/database/migrations/039_add_body_path_to_request_logs.sql
-- UP Migration
-- Add body_path columns for file-based body storage.
-- Existing rows keep their inline request_body/response_body;
-- new rows will write bodies to files and store the path here.

ALTER TABLE request_logs ADD COLUMN request_body_path TEXT;
ALTER TABLE request_logs ADD COLUMN response_body_path TEXT;

-- Index for finding rows that still have inline bodies (for migration)
CREATE INDEX IF NOT EXISTS idx_request_logs_has_inline_body
    ON request_logs(id) WHERE request_body IS NOT NULL AND request_body_path IS NULL;

-- DOWN Migration
-- ALTER TABLE does not support DROP COLUMN in SQLite < 3.35.0
-- For safety, we leave the columns in place on downgrade.
