package models

// Claude API Models

type ClaudeMessagesRequest struct {
	Model                  string                 `json:"model"`
	Messages               []ClaudeMessage        `json:"messages"`
	System                 interface{}            `json:"system,omitempty"`
	MaxTokens              int                    `json:"max_tokens"`
	Temperature            float64                `json:"temperature,omitempty"`
	TopP                   *float64               `json:"top_p,omitempty"`
	TopK                   *int                   `json:"top_k,omitempty"`
	Stream                 bool                   `json:"stream,omitempty"`
	StopSequences          []string               `json:"stop_sequences,omitempty"`
	Metadata               *ClaudeMetadata        `json:"metadata,omitempty"`
	Tools                  []ClaudeTool           `json:"tools,omitempty"`
	ToolChoice             map[string]interface{} `json:"tool_choice,omitempty"`
	DisableParallelToolUse *bool                  `json:"disable_parallel_tool_use,omitempty"` // Disable parallel tool calls
	ContextManagement      interface{}            `json:"context_management,omitempty"`        // Claude Code 2.x sends this, but we ignore it
	Thinking               *ClaudeThinking        `json:"thinking,omitempty"`                  // Beta feature for extended thinking
}

type ClaudeMetadata struct {
	UserID    string `json:"user_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type ClaudeThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // thinking token budget
}

type ClaudeMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type ClaudeContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"` // For thinking content blocks
	Source    interface{}            `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
}

type ClaudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type ClaudeResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Model        string               `json:"model"`
	Content      []ClaudeContentBlock `json:"content"`
	StopReason   string               `json:"stop_reason"`
	StopSequence *string              `json:"stop_sequence,omitempty"`
	Usage        ClaudeUsage          `json:"usage"`
}

type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type ClaudeTokenCountRequest struct {
	Model    string          `json:"model"`
	Messages []ClaudeMessage `json:"messages"`
	System   interface{}     `json:"system,omitempty"`
}
