package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Session represents a conversation session
type Session struct {
	ID             string                 `json:"id"`
	UserID         string                 `json:"user_id"`
	ConfigID       string                 `json:"config_id"`
	Title          string                 `json:"title"`
	Model          string                 `json:"model"`
	SystemPrompt   string                 `json:"system_prompt,omitempty"`
	MessageCount   int                    `json:"message_count"`
	TotalTokens    int                    `json:"total_tokens"`
	IsActive       bool                   `json:"is_active"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	LastMessageAt  *time.Time             `json:"last_message_at,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SessionMessage represents a message in a session
type SessionMessage struct {
	ID            int                    `json:"id"`
	SessionID     string                 `json:"session_id"`
	Role          string                 `json:"role"`
	Content       string                 `json:"content"`
	ToolCalls     []map[string]interface{} `json:"tool_calls,omitempty"`
	ToolCallID    string                 `json:"tool_call_id,omitempty"`
	InputTokens   int                    `json:"input_tokens"`
	OutputTokens  int                    `json:"output_tokens"`
	MessageIndex  int                    `json:"message_index"`
	CreatedAt     time.Time              `json:"created_at"`
}

// CreateSession creates a new session
func CreateSession(session *Session) error {
	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO sessions (id, user_id, config_id, title, model, system_prompt,
							  message_count, total_tokens, is_active, created_at, updated_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), ?)
	`

	_, err = DB.Exec(query,
		session.ID,
		session.UserID,
		session.ConfigID,
		session.Title,
		session.Model,
		session.SystemPrompt,
		session.MessageCount,
		session.TotalTokens,
		session.IsActive,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func GetSession(sessionID string) (*Session, error) {
	query := `
		SELECT id, user_id, config_id, title, model, system_prompt,
			   message_count, total_tokens, is_active, created_at, updated_at, last_message_at, metadata
		FROM sessions
		WHERE id = ?
	`

	var session Session
	var metadataJSON []byte
	var lastMessageAt sql.NullTime

	err := DB.QueryRow(query, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.ConfigID,
		&session.Title,
		&session.Model,
		&session.SystemPrompt,
		&session.MessageCount,
		&session.TotalTokens,
		&session.IsActive,
		&session.CreatedAt,
		&session.UpdatedAt,
		&lastMessageAt,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if lastMessageAt.Valid {
		session.LastMessageAt = &lastMessageAt.Time
	}

	if len(metadataJSON) > 0 {
		json.Unmarshal(metadataJSON, &session.Metadata)
	}

	return &session, nil
}

// GetSessionsByUser retrieves all sessions for a user
func GetSessionsByUser(userID string, limit, offset int) ([]*Session, error) {
	query := `
		SELECT id, user_id, config_id, title, model, system_prompt,
			   message_count, total_tokens, is_active, created_at, updated_at, last_message_at, metadata
		FROM sessions
		WHERE user_id = ?
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := DB.Query(query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}
	defer rows.Close()

	sessions := []*Session{}
	for rows.Next() {
		var session Session
		var metadataJSON []byte
		var lastMessageAt sql.NullTime

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.ConfigID,
			&session.Title,
			&session.Model,
			&session.SystemPrompt,
			&session.MessageCount,
			&session.TotalTokens,
			&session.IsActive,
			&session.CreatedAt,
			&session.UpdatedAt,
			&lastMessageAt,
			&metadataJSON,
		)
		if err != nil {
			continue
		}

		if lastMessageAt.Valid {
			session.LastMessageAt = &lastMessageAt.Time
		}
		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &session.Metadata)
		}

		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// UpdateSession updates a session
func UpdateSession(session *Session) error {
	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE sessions
		SET title = ?, model = ?, system_prompt = ?, message_count = ?, total_tokens = ?,
			is_active = ?, updated_at = datetime('now'), metadata = ?
		WHERE id = ?
	`

	_, err = DB.Exec(query,
		session.Title,
		session.Model,
		session.SystemPrompt,
		session.MessageCount,
		session.TotalTokens,
		session.IsActive,
		metadataJSON,
		session.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// UpdateSessionLastMessage updates the last message timestamp
func UpdateSessionLastMessage(sessionID string) error {
	query := `
		UPDATE sessions
		SET last_message_at = datetime('now'), updated_at = datetime('now')
		WHERE id = ?
	`

	_, err := DB.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session last message: %w", err)
	}

	return nil
}

// DeleteSession deletes a session and all its messages
func DeleteSession(sessionID string) error {
	query := `DELETE FROM sessions WHERE id = ?`

	_, err := DB.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// CreateSessionMessage creates a new session message
func CreateSessionMessage(message *SessionMessage) error {
	toolCallsJSON, err := json.Marshal(message.ToolCalls)
	if err != nil {
		return fmt.Errorf("failed to marshal tool calls: %w", err)
	}

	query := `
		INSERT INTO session_messages (session_id, role, content, tool_calls, tool_call_id,
									  input_tokens, output_tokens, message_index, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	result, err := DB.Exec(query,
		message.SessionID,
		message.Role,
		message.Content,
		toolCallsJSON,
		message.ToolCallID,
		message.InputTokens,
		message.OutputTokens,
		message.MessageIndex,
	)

	if err != nil {
		return fmt.Errorf("failed to create session message: %w", err)
	}

	id, _ := result.LastInsertId()
	message.ID = int(id)

	return nil
}

// GetSessionMessages retrieves all messages for a session
func GetSessionMessages(sessionID string) ([]*SessionMessage, error) {
	query := `
		SELECT id, session_id, role, content, tool_calls, tool_call_id,
			   input_tokens, output_tokens, message_index, created_at
		FROM session_messages
		WHERE session_id = ?
		ORDER BY message_index ASC, created_at ASC
	`

	rows, err := DB.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}
	defer rows.Close()

	messages := []*SessionMessage{}
	for rows.Next() {
		var message SessionMessage
		var toolCallsJSON []byte

		err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.Role,
			&message.Content,
			&toolCallsJSON,
			&message.ToolCallID,
			&message.InputTokens,
			&message.OutputTokens,
			&message.MessageIndex,
			&message.CreatedAt,
		)
		if err != nil {
			continue
		}

		if len(toolCallsJSON) > 0 {
			json.Unmarshal(toolCallsJSON, &message.ToolCalls)
		}

		messages = append(messages, &message)
	}

	return messages, nil
}

// GetSessionMessagesPaginated retrieves paginated messages for a session
func GetSessionMessagesPaginated(sessionID string, limit, offset int) ([]*SessionMessage, error) {
	query := `
		SELECT id, session_id, role, content, tool_calls, tool_call_id,
			   input_tokens, output_tokens, message_index, created_at
		FROM session_messages
		WHERE session_id = ?
		ORDER BY message_index ASC, created_at ASC
		LIMIT ? OFFSET ?
	`

	rows, err := DB.Query(query, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}
	defer rows.Close()

	messages := []*SessionMessage{}
	for rows.Next() {
		var message SessionMessage
		var toolCallsJSON []byte

		err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.Role,
			&message.Content,
			&toolCallsJSON,
			&message.ToolCallID,
			&message.InputTokens,
			&message.OutputTokens,
			&message.MessageIndex,
			&message.CreatedAt,
		)
		if err != nil {
			continue
		}

		if len(toolCallsJSON) > 0 {
			json.Unmarshal(toolCallsJSON, &message.ToolCalls)
		}

		messages = append(messages, &message)
	}

	return messages, nil
}

// DeleteSessionMessages deletes all messages for a session
func DeleteSessionMessages(sessionID string) error {
	query := `DELETE FROM session_messages WHERE session_id = ?`

	_, err := DB.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session messages: %w", err)
	}

	return nil
}

// CountSessionMessages counts the number of messages in a session
func CountSessionMessages(sessionID string) (int, error) {
	query := `SELECT COUNT(*) FROM session_messages WHERE session_id = ?`

	var count int
	err := DB.QueryRow(query, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count session messages: %w", err)
	}

	return count, nil
}
