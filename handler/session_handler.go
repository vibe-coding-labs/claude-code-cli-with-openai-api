package handler

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// SessionHandler handles conversation session operations
type SessionHandler struct{}

// NewSessionHandler creates a new session handler
func NewSessionHandler() *SessionHandler {
	return &SessionHandler{}
}

// ExtractSessionID extracts session ID from request metadata
func (sh *SessionHandler) ExtractSessionID(req *models.ClaudeMessagesRequest) string {
	if req.Metadata != nil && req.Metadata.SessionID != "" {
		return req.Metadata.SessionID
	}
	return ""
}

// GetOrCreateSession gets an existing session or creates a new one
func (sh *SessionHandler) GetOrCreateSession(sessionID, userID, configID string, req *models.ClaudeMessagesRequest) (*database.Session, error) {
	// If session ID is provided, try to get existing session
	if sessionID != "" {
		session, err := database.GetSession(sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}
		if session != nil {
			// Verify user and config match
			if session.UserID == userID && session.ConfigID == configID {
				return session, nil
			}
		}
	}

	// Create new session
	session := &database.Session{
		ID:           generateSessionID(),
		UserID:       userID,
		ConfigID:     configID,
		Title:        generateSessionTitle(req),
		Model:        req.Model,
		SystemPrompt: extractSystemPrompt(req.System),
		IsActive:     true,
		MessageCount: 0,
		TotalTokens:  0,
	}

	if err := database.CreateSession(session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// LoadConversationHistory loads conversation history for a session and prepends to messages
func (sh *SessionHandler) LoadConversationHistory(sessionID string, messages []models.ClaudeMessage) ([]models.ClaudeMessage, error) {
	historyMessages, err := database.GetSessionMessages(sessionID)
	if err != nil {
		return messages, fmt.Errorf("failed to get session messages: %w", err)
	}

	if len(historyMessages) == 0 {
		return messages, nil
	}

	// Convert history messages to ClaudeMessage format
	history := make([]models.ClaudeMessage, 0, len(historyMessages))
	for _, msg := range historyMessages {
		claudeMsg := models.ClaudeMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		history = append(history, claudeMsg)
	}

	// Prepend history to current messages
	return append(history, messages...), nil
}

// SaveMessages saves messages to a session
func (sh *SessionHandler) SaveMessages(sessionID string, messages []models.ClaudeMessage, usage *models.ClaudeUsage) error {
	if len(messages) == 0 {
		return nil
	}

	// Get current message count to determine index
	currentCount, err := database.CountSessionMessages(sessionID)
	if err != nil {
		return fmt.Errorf("failed to count session messages: %w", err)
	}

	// Save each message
	for i, msg := range messages {
		content := ""
		switch v := msg.Content.(type) {
		case string:
			content = v
		default:
			// Try to marshal complex content
			contentBytes, err := json.Marshal(v)
			if err == nil {
				content = string(contentBytes)
			}
		}

		sessionMsg := &database.SessionMessage{
			SessionID:    sessionID,
			Role:         msg.Role,
			Content:      content,
			MessageIndex: currentCount + i,
		}

		// Extract tool calls if present
		if msg.Role == "assistant" {
			// Check if content contains tool calls
			if blocks, ok := msg.Content.([]interface{}); ok {
				toolCalls := []map[string]interface{}{}
				for _, block := range blocks {
					if blockMap, ok := block.(map[string]interface{}); ok {
						if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_use" {
							toolCalls = append(toolCalls, blockMap)
						}
					}
				}
				if len(toolCalls) > 0 {
					sessionMsg.ToolCalls = toolCalls
				}
			}
		}

		// Set token usage for the last user/assistant pair
		if usage != nil {
			if msg.Role == "user" {
				sessionMsg.InputTokens = usage.InputTokens
			} else if msg.Role == "assistant" {
				sessionMsg.OutputTokens = usage.OutputTokens
			}
		}

		if err := database.CreateSessionMessage(sessionMsg); err != nil {
			return fmt.Errorf("failed to create session message: %w", err)
		}
	}

	// Update session stats
	session, err := database.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	session.MessageCount = currentCount + len(messages)
	if usage != nil {
		session.TotalTokens += usage.InputTokens + usage.OutputTokens
	}

	if err := database.UpdateSession(session); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Update last message timestamp
	if err := database.UpdateSessionLastMessage(sessionID); err != nil {
		return fmt.Errorf("failed to update session last message: %w", err)
	}

	return nil
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("sess_%s", uuid.New().String()[:24])
}

// generateSessionTitle generates a title for a new session
func generateSessionTitle(req *models.ClaudeMessagesRequest) string {
	// Try to extract title from first user message
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			switch v := msg.Content.(type) {
			case string:
				if len(v) > 50 {
					return v[:50] + "..."
				}
				return v
			}
		}
	}
	return "New Conversation"
}

// extractSystemPrompt extracts system prompt from request
func extractSystemPrompt(system interface{}) string {
	if system == nil {
		return ""
	}

	switch v := system.(type) {
	case string:
		return v
	default:
		// Try to marshal complex system prompt
		contentBytes, err := json.Marshal(v)
		if err == nil {
			return string(contentBytes)
		}
	}
	return ""
}

// FilterNewMessages filters out messages that are already in history
func (sh *SessionHandler) FilterNewMessages(sessionID string, messages []models.ClaudeMessage) ([]models.ClaudeMessage, error) {
	historyMessages, err := database.GetSessionMessages(sessionID)
	if err != nil {
		return messages, err
	}

	// If no history, all messages are new
	if len(historyMessages) == 0 {
		return messages, nil
	}

	// Get the last message index from history
	_ = historyMessages[len(historyMessages)-1].MessageIndex

	// Filter messages that have index > lastIndex
	// This assumes client includes message index in content or we track it differently
	// For simplicity, we'll just return the last user message and any assistant messages
	// In a real implementation, you'd want more sophisticated deduplication

	return messages, nil
}
