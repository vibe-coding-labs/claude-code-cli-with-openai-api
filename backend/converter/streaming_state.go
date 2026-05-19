package converter

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ContentBlockType represents the type of content block being streamed.
// Ported from litellm's AnthropicStreamWrapper.
type ContentBlockType string

const (
	BlockText             ContentBlockType = "text"
	BlockToolUse          ContentBlockType = "tool_use"
	BlockThinking         ContentBlockType = "thinking"
	BlockRedactedThinking ContentBlockType = "redacted_thinking"
	BlockCompaction       ContentBlockType = "compaction"
)

// StreamingState holds the state for streaming conversion.
// Ported from litellm's AnthropicStreamWrapper state machine.
type StreamingState struct {
	mu sync.Mutex

	messageID string
	model     string

	// Tool name mapping (truncated → original) for restoring names in response (litellm pattern)
	ToolNameMapping map[string]string

	// State machine flags (litellm pattern)
	sentFirstChunk         bool
	sentContentBlockStart  bool
	sentContentBlockFinish bool
	sentLastMessage        bool

	// Current content block tracking
	currentBlockType  ContentBlockType
	currentBlockIndex int
	currentBlockStart map[string]interface{}

	// Usage tracking
	usage           models.ClaudeUsage
	finalStopReason string

	// Track whether message_delta was already emitted
	emittedMessageDelta bool

	// Conversion error tracking
	chunkErrors int

	// Tool tracking for OpenAI's streaming tool call deltas
	// Maps OpenAI tool_call index to accumulated state
	toolCalls map[int]*toolCallInfo
}

// toolCallInfo tracks accumulated state for a tool call during streaming
type toolCallInfo struct {
	id         string
	name       string
	argsBuffer string
}

// generateMessageID creates a message ID in the format "msg_<uuid-prefix>".
func generateMessageID() string {
	return fmt.Sprintf("msg_%s", uuid.New().String()[:24])
}

// newStreamingState creates a new StreamingState initialized for the given model.
func newStreamingState(model string, toolNameMapping map[string]string) *StreamingState {
	return &StreamingState{
		messageID:       generateMessageID(),
		model:           model,
		ToolNameMapping: toolNameMapping,
		// Start with text block type (litellm starts text eagerly)
		currentBlockType:  BlockText,
		currentBlockIndex: 0,
		currentBlockStart: map[string]interface{}{"type": "text", "text": ""},
		toolCalls:         make(map[int]*toolCallInfo),
	}
}

// restoreToolName restores the original tool name if it was truncated (litellm pattern).
func (s *StreamingState) restoreToolName(name string) string {
	if s.ToolNameMapping == nil || name == "" {
		return name
	}
	if original, ok := s.ToolNameMapping[name]; ok {
		return original
	}
	return name
}

// shouldStartNewBlock detects if the current OpenAI streaming chunk indicates
// a content block type change. Ported from litellm's _should_start_new_content_block.
func (s *StreamingState) shouldStartNewBlock(choice *models.OpenAIChoice) (bool, ContentBlockType, map[string]interface{}) {
	if choice == nil || choice.FinishReason != "" {
		return false, s.currentBlockType, nil
	}
	// No block transitions if we haven't started any block yet (lazy start)
	if !s.sentContentBlockStart {
		return false, s.currentBlockType, nil
	}

	delta := choice.Delta
	if delta == nil {
		return false, s.currentBlockType, nil
	}

	// Detect block type from raw chunk
	blockType, blockStart := s.detectBlockType(delta)

	// Check if type changed
	if blockType != s.currentBlockType {
		return true, blockType, blockStart
	}

	// For parallel tool calls: a new tool_use with a name means a new block (litellm pattern)
	if blockType == BlockToolUse && len(delta.ToolCalls) > 0 {
		for _, tc := range delta.ToolCalls {
			if tc.Function.Name != "" {
				toolID := NormalizeToolCallID(tc.ID)
				if toolID == "" {
					toolID = "toolu_" + generateShortID()
				}
				toolName := s.restoreToolName(tc.Function.Name)
				blockStart = map[string]interface{}{
					"type":  "tool_use",
					"id":    toolID,
					"name":  toolName,
					"input": "",
				}
				return true, blockType, blockStart
			}
		}
	}

	return false, s.currentBlockType, nil
}

// detectBlockType determines what type of content block an OpenAI delta implies.
// Ported from litellm's _translate_streaming_openai_chunk_to_anthropic_content_block.
func (s *StreamingState) detectBlockType(delta *models.OpenAIMessage) (ContentBlockType, map[string]interface{}) {
	// Tool calls: detect whenever tool_calls array is non-empty (litellm pattern)
	// litellm checks: choice.delta.tool_calls is not None and len > 0 and function is not None
	if len(delta.ToolCalls) > 0 {
		tc := delta.ToolCalls[0]
		toolID := NormalizeToolCallID(tc.ID)
		if toolID == "" {
			toolID = "toolu_" + generateShortID()
		}
		toolName := s.restoreToolName(tc.Function.Name)
		// Don't switch to tool_use if name is empty — wait for the name chunk.
		// Some providers send name on a separate chunk from the initial tool_call detection.
		// Emitting a tool_use block with empty name causes Claude Code CLI parse failures.
		if toolName == "" {
			return s.currentBlockType, s.currentBlockStart
		}
		return BlockToolUse, map[string]interface{}{
			"type":  "tool_use",
			"id":    toolID,
			"name":  toolName,
			"input": "",
		}
	}

	// Reasoning/thinking content
	if delta.ReasoningContent != "" {
		return BlockThinking, map[string]interface{}{
			"type":     "thinking",
			"thinking": "",
		}
	}

	// Regular text content (only if non-empty)
	if delta.Content != nil {
		if textContent, ok := delta.Content.(string); ok && textContent != "" {
			return BlockText, map[string]interface{}{"type": "text", "text": ""}
		}
	}

	// Default: no change
	return s.currentBlockType, s.currentBlockStart
}

// updateUsage extracts usage data from an OpenAI streaming chunk.
func (s *StreamingState) updateUsage(chunk *models.OpenAIResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		s.usage.InputTokens = chunk.Usage.PromptTokens
		s.usage.OutputTokens = chunk.Usage.CompletionTokens
		if chunk.Usage.PromptTokensDetails != nil {
			s.usage.CacheReadInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
	}
}

// translateFinishReason maps OpenAI finish_reason to Anthropic stop_reason.
// Ported from litellm's _FINISH_REASON_MAP and one-api's stopReasonClaude2OpenAI.
func translateFinishReason(reason string) string {
	switch reason {
	case "stop":
		return models.StopEndTurn
	case "length":
		return models.StopMaxTokens
	case "tool_calls", "function_call":
		return models.StopToolUse
	case models.StopSequence:
		return models.StopEndTurn
	case "content_filter", "refusal", "content_filtered":
		return models.StopEndTurn
	case "compaction":
		return models.StopEndTurn
	default:
		return models.StopEndTurn
	}
}
