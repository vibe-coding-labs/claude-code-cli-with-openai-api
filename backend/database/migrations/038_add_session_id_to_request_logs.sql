-- backend/database/migrations/038_add_session_id_to_request_logs.sql
-- Add session_id to request_logs for request-session correlation

-- Add session_id column (nullable, existing records will have NULL)
ALTER TABLE request_logs ADD COLUMN session_id TEXT;

-- Create index for efficient session-based queries
CREATE INDEX IF NOT EXISTS idx_request_logs_session_id ON request_logs(session_id);

-- Create composite index for common query patterns
CREATE INDEX IF NOT EXISTS idx_request_logs_config_session ON request_logs(config_id, session_id);
