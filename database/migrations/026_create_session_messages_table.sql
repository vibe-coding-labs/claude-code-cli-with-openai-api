-- Migration: Create session_messages table for conversation history
-- UP

CREATE TABLE IF NOT EXISTS session_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    tool_calls TEXT,
    tool_call_id TEXT,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    message_index INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_session_messages_session_id ON session_messages(session_id);
CREATE INDEX IF NOT EXISTS idx_session_messages_message_index ON session_messages(message_index);
CREATE INDEX IF NOT EXISTS idx_session_messages_created_at ON session_messages(created_at);

-- DOWN

DROP INDEX IF EXISTS idx_session_messages_session_id;
DROP INDEX IF EXISTS idx_session_messages_message_index;
DROP INDEX IF EXISTS idx_session_messages_created_at;
DROP TABLE IF EXISTS session_messages;
