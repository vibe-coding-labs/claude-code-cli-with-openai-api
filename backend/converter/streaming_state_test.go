package converter

import (
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestTranslateFinishReason(t *testing.T) {
	tests := []struct {
		reason   string
		expected string
	}{
		{"stop", models.StopEndTurn},
		{"length", models.StopMaxTokens},
		{"tool_calls", models.StopToolUse},
		{"function_call", models.StopToolUse},
		{models.StopSequence, models.StopEndTurn},
		{"content_filter", models.StopEndTurn},
		{"refusal", models.StopEndTurn},
		{"content_filtered", models.StopEndTurn},
		{"compaction", models.StopEndTurn},
		{"", models.StopEndTurn},
		{"some_unknown_reason", models.StopEndTurn},
	}
	for _, tt := range tests {
		if got := translateFinishReason(tt.reason); got != tt.expected {
			t.Errorf("translateFinishReason(%q) = %q, want %q", tt.reason, got, tt.expected)
		}
	}
}

func TestRestoreToolName(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	// No mapping at all: name passes through unchanged.
	if got := s.restoreToolName("Foo"); got != "Foo" {
		t.Errorf("expected passthrough with nil mapping, got %q", got)
	}
	// Empty name: passes through regardless of mapping.
	s2 := newStreamingState("gpt-test", map[string]string{"trunc": "OriginalName"})
	if got := s2.restoreToolName(""); got != "" {
		t.Errorf("expected empty string passthrough, got %q", got)
	}
	// Known truncated name: restored to original.
	if got := s2.restoreToolName("trunc"); got != "OriginalName" {
		t.Errorf("expected restored name %q, got %q", "OriginalName", got)
	}
	// Unknown name with a non-nil mapping: passes through unchanged.
	if got := s2.restoreToolName("NotInMapping"); got != "NotInMapping" {
		t.Errorf("expected passthrough for unmapped name, got %q", got)
	}
}

func TestShouldStartNewBlock_NotYetStarted(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	// sentContentBlockStart is false by construction: no transition regardless of input.
	shouldStart, blockType, _, _ := s.shouldStartNewBlock(&models.OpenAIChoice{
		Delta: &models.OpenAIMessage{Content: "hello"},
	})
	if shouldStart {
		t.Error("expected no transition before the first block has started")
	}
	if blockType != s.currentBlockType {
		t.Errorf("expected currentBlockType to be returned unchanged, got %v", blockType)
	}
}

func TestShouldStartNewBlock_NilChoiceAndNilDelta(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true

	if shouldStart, _, _, _ := s.shouldStartNewBlock(nil); shouldStart {
		t.Error("expected no transition for a nil choice")
	}
	if shouldStart, _, _, _ := s.shouldStartNewBlock(&models.OpenAIChoice{Delta: nil}); shouldStart {
		t.Error("expected no transition for a nil delta")
	}
}

func TestShouldStartNewBlock_TextToToolUse(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true
	s.currentBlockType = BlockText

	shouldStart, blockType, blockStart, idx := s.shouldStartNewBlock(&models.OpenAIChoice{
		Delta: &models.OpenAIMessage{
			ToolCalls: []models.OpenAIToolCall{
				{Index: 2, ID: "call_abc", Function: models.OpenAIFunctionCall{Name: "Read"}},
			},
		},
	})
	if !shouldStart || blockType != BlockToolUse {
		t.Fatalf("expected transition to tool_use, got shouldStart=%v blockType=%v", shouldStart, blockType)
	}
	if blockStart["type"] != "tool_use" || blockStart["name"] != "Read" {
		t.Errorf("unexpected blockStart contents: %v", blockStart)
	}
	if idx != 2 {
		t.Errorf("expected resolved tool_call index 2, got %d", idx)
	}
}

func TestShouldStartNewBlock_ToolCallWithEmptyNameDoesNotTransitionYet(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true
	s.currentBlockType = BlockText

	shouldStart, _, _, _ := s.shouldStartNewBlock(&models.OpenAIChoice{
		Delta: &models.OpenAIMessage{
			ToolCalls: []models.OpenAIToolCall{
				{Index: 0, ID: "call_abc", Function: models.OpenAIFunctionCall{Name: ""}},
			},
		},
	})
	if shouldStart {
		t.Error("expected no transition while the tool name is still unknown")
	}
}

func TestShouldStartNewBlock_FinishReasonShortCircuits(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true
	s.currentBlockType = BlockText

	shouldStart, _, _, _ := s.shouldStartNewBlock(&models.OpenAIChoice{
		FinishReason: "stop",
		Delta:        &models.OpenAIMessage{Content: ""},
	})
	if shouldStart {
		t.Error("expected no transition once finish_reason is set (and no tool_calls present)")
	}
}

func TestShouldStartNewBlock_TextToThinking(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true
	s.currentBlockType = BlockText

	shouldStart, blockType, blockStart, _ := s.shouldStartNewBlock(&models.OpenAIChoice{
		Delta: &models.OpenAIMessage{ReasoningContent: "thinking..."},
	})
	if !shouldStart || blockType != BlockThinking {
		t.Fatalf("expected transition to thinking, got shouldStart=%v blockType=%v", shouldStart, blockType)
	}
	if blockStart["type"] != "thinking" {
		t.Errorf("unexpected blockStart contents: %v", blockStart)
	}
}

func TestShouldStartNewBlock_ParallelToolCallsNewNamedBlock(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true
	s.currentBlockType = BlockToolUse
	s.currentBlockStart = map[string]interface{}{"type": "tool_use", "id": "toolu_1", "name": "First", "input": map[string]interface{}{}}

	shouldStart, blockType, blockStart, idx := s.shouldStartNewBlock(&models.OpenAIChoice{
		Delta: &models.OpenAIMessage{
			ToolCalls: []models.OpenAIToolCall{
				{Index: 1, ID: "call_2", Function: models.OpenAIFunctionCall{Name: "Second"}},
			},
		},
	})
	if !shouldStart || blockType != BlockToolUse {
		t.Fatalf("expected a new tool_use block for the second parallel tool call, got shouldStart=%v blockType=%v", shouldStart, blockType)
	}
	if blockStart["name"] != "Second" {
		t.Errorf("expected new block to be named Second, got %v", blockStart["name"])
	}
	if idx != 1 {
		t.Errorf("expected resolved index 1, got %d", idx)
	}
}

func TestShouldStartNewBlock_NoChangeWhenSameTypeContinues(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	s.sentContentBlockStart = true
	s.currentBlockType = BlockText
	s.currentBlockStart = map[string]interface{}{"type": "text", "text": ""}

	shouldStart, _, _, _ := s.shouldStartNewBlock(&models.OpenAIChoice{
		Delta: &models.OpenAIMessage{Content: "more text"},
	})
	if shouldStart {
		t.Error("expected no transition when text continues as text")
	}
}

func TestDetectBlockType_ToolCallMissingID(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	blockType, blockStart := s.detectBlockType(&models.OpenAIMessage{
		ToolCalls: []models.OpenAIToolCall{
			{Index: 0, ID: "", Function: models.OpenAIFunctionCall{Name: "Bash"}},
		},
	})
	if blockType != BlockToolUse {
		t.Fatalf("expected BlockToolUse, got %v", blockType)
	}
	id, _ := blockStart["id"].(string)
	if id == "" {
		t.Error("expected a generated fallback tool_use id when upstream omits one")
	}
}

func TestAccumulateToolCallDelta_ArgumentsBeforeName(t *testing.T) {
	s := newStreamingState("gpt-test", nil)
	// Arguments arrive before the name-revealing chunk — must not be dropped.
	s.accumulateToolCallDelta(models.OpenAIToolCall{
		Index:    0,
		ID:       "call_1",
		Function: models.OpenAIFunctionCall{Arguments: `{"path":`},
	})
	s.accumulateToolCallDelta(models.OpenAIToolCall{
		Index:    0,
		Function: models.OpenAIFunctionCall{Name: "Read", Arguments: `"/tmp/x"}`},
	})
	info := s.toolCalls[0]
	if info == nil {
		t.Fatal("expected tool call info to be tracked")
	}
	if info.name != "Read" {
		t.Errorf("expected name Read, got %q", info.name)
	}
	if info.argsBuffer != `{"path":"/tmp/x"}` {
		t.Errorf("expected argsBuffer to accumulate across chunks regardless of order, got %q", info.argsBuffer)
	}
}
