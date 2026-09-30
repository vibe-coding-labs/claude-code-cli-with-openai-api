package converter

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
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
	// OpenAI tool_call index that owns the currently open block, valid only
	// when currentBlockType == BlockToolUse. Lets flushToolCallArgs target
	// the right entry in toolCalls without re-deriving it from the delta.
	currentToolCallIndex int

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
	// emittedLen is how many bytes of argsBuffer have already been sent to
	// the client as content_block_delta events. Needed because argsBuffer
	// accumulates unconditionally (see accumulateToolCallDelta) — including
	// fragments that arrived before this tool call's block was even open —
	// while emission can only happen once the block is open, so the two can
	// legitimately be out of sync and must be flushed by the difference.
	emittedLen int
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

// accumulateToolCallDelta unconditionally folds a single OpenAI tool-call
// delta fragment into internal state, keyed by its OpenAI tool_call index —
// regardless of whether the visible content block has transitioned to
// tool_use yet. detectBlockType/shouldStartNewBlock deliberately withhold
// that transition until a non-empty name arrives (to avoid emitting a
// nameless tool_use block), but some providers stream argument fragments in
// chunks that arrive BEFORE the id/name-revealing chunk (observed for real
// tool calls, e.g. "TaskUpdate" and "Read"). If argument accumulation waited
// for the same signal as the visible transition, those fragments would be
// silently and permanently dropped, and the client would receive a tool_use
// block whose "input" is truncated or empty instead of an error — which is
// exactly why this bug went unnoticed as anything other than "the tool call
// after conversion is wrong".
func (s *StreamingState) accumulateToolCallDelta(tc models.OpenAIToolCall) {
	info := s.toolCalls[tc.Index]
	if info == nil {
		info = &toolCallInfo{}
		s.toolCalls[tc.Index] = info
	}
	if tc.ID != "" {
		info.id = NormalizeToolCallID(tc.ID)
	}
	// Capture Gemini's thought_signature (extra_content.google), keyed by
	// the ID Claude Code will echo back later.
	rememberThoughtSignature(info.id, tc.ExtraContent)
	if name := s.restoreToolName(tc.Function.Name); name != "" {
		info.name = name
	}
	if tc.Function.Arguments != "" {
		info.argsBuffer += tc.Function.Arguments
	}
}

// shouldStartNewBlock detects if the current OpenAI streaming chunk indicates
// a content block type change. Ported from litellm's _should_start_new_content_block.
// The fourth return value is the OpenAI tool_call index that resolved the
// transition; only meaningful when the returned type is BlockToolUse.
func (s *StreamingState) shouldStartNewBlock(choice *models.OpenAIChoice) (bool, ContentBlockType, map[string]interface{}, int) {
	// No block transitions if we haven't started any block yet (lazy start)
	if !s.sentContentBlockStart {
		return false, s.currentBlockType, nil, 0
	}

	if choice == nil {
		return false, s.currentBlockType, nil, 0
	}

	delta := choice.Delta
	if delta == nil {
		return false, s.currentBlockType, nil, 0
	}

	// IMPORTANT: Check for tool_calls BEFORE checking finish_reason
	// Some providers send tool_calls and finish_reason in the same chunk
	// We need to detect the tool_use block type before the stream ends
	if len(delta.ToolCalls) > 0 {
		tc := delta.ToolCalls[0]
		toolID := NormalizeToolCallID(tc.ID)
		if toolID == "" {
			toolID = "toolu_" + generateShortID()
		}
		toolName := s.restoreToolName(tc.Function.Name)
		if toolName != "" {
			blockStart := map[string]interface{}{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolName,
				"input": map[string]interface{}{}, // Empty object, not empty string (Claude CLI requirement)
			}
			// If current block is text, we need to transition to tool_use
			if s.currentBlockType == BlockText {
				return true, BlockToolUse, blockStart, tc.Index
			}
		}
	}

	// After processing tool_calls, check finish_reason
	if choice.FinishReason != "" {
		return false, s.currentBlockType, nil, 0
	}

	// Detect block type from raw chunk
	blockType, blockStart := s.detectBlockType(delta)

	// Check if type changed
	if blockType != s.currentBlockType {
		idx := 0
		if blockType == BlockToolUse && len(delta.ToolCalls) > 0 {
			idx = delta.ToolCalls[0].Index
		}
		return true, blockType, blockStart, idx
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
					"input": map[string]interface{}{}, // Empty object, not empty string (Claude CLI requirement)
				}
				return true, blockType, blockStart, tc.Index
			}
		}
	}

	return false, s.currentBlockType, nil, 0
}

// detectBlockType determines what type of content block an OpenAI delta implies.
// Ported from litellm's _translate_streaming_openai_chunk_to_anthropic_content_block.
func (s *StreamingState) detectBlockType(delta *models.OpenAIMessage) (ContentBlockType, map[string]interface{}) {
	// Debug: log incoming delta
	utils.GetLogger().Info("[detectBlockType] delta: content=%v tool_calls_len=%d reasoning_len=%d", delta.Content, len(delta.ToolCalls), len(delta.ReasoningContent))
	// Tool calls: detect whenever tool_calls array is non-empty (litellm pattern)
	// litellm checks: choice.delta.tool_calls is not None and len > 0 and function is not None
	if len(delta.ToolCalls) > 0 {
		tc := delta.ToolCalls[0]
		toolID := NormalizeToolCallID(tc.ID)
		if toolID == "" {
			toolID = "toolu_" + generateShortID()
		}
		toolName := s.restoreToolName(tc.Function.Name)
		// Debug: log tool call detection
		utils.GetLogger().Info("[detectBlockType] tool_call detected: id=%q name=%q args_len=%d", tc.ID, tc.Function.Name, len(tc.Function.Arguments))
		// Don't switch to tool_use if name is empty — wait for the name chunk.
		// Some providers send name on a separate chunk from the initial tool_call detection.
		// Emitting a tool_use block with empty name causes Claude Code CLI parse failures.
		if toolName == "" {
			utils.GetLogger().Warn("[detectBlockType] tool_call name is empty, waiting for name chunk: id=%q", tc.ID)
			return s.currentBlockType, s.currentBlockStart
		}
		return BlockToolUse, map[string]interface{}{
			"type":  "tool_use",
			"id":    toolID,
			"name":  toolName,
			"input": map[string]interface{}{}, // Empty object, not empty string (Claude CLI requirement)
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
