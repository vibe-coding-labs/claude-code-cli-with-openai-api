package converter

import (
	"sync"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// StreamingState holds the state for streaming conversion
type StreamingState struct {
	mu               sync.Mutex
	messageID        string
	textBlockIndex   int
	toolBlockCounter int
	currentToolCalls map[int]*ToolCallState
	finalStopReason  string
	usage            models.ClaudeUsage
}

// ToolCallState tracks the state of a tool call during streaming
type ToolCallState struct {
	ID          string
	Name        string
	ArgsBuffer  string
	JSONSent    bool
	ClaudeIndex int
	Started     bool
}
