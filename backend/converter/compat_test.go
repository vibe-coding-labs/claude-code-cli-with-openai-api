package converter

import (
	"encoding/json"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// --- Tool Call ID Auto-generation ---

func TestToolUseBlockWithoutID(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "", Name: "test_tool", Input: map[string]interface{}{"key": "value"}},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	for _, msg := range openAIReq.Messages {
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.ID == "" {
					t.Error("Tool call ID should not be empty, expected auto-generated ID")
				}
				if tc.Function.Name != "test_tool" {
					t.Errorf("Tool name = %q, want %q", tc.Function.Name, "test_tool")
				}
			}
			return
		}
	}
	t.Error("No tool_calls found in output messages")
}

func TestToolUseBlockWithExistingID(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "toolu_abc123", Name: "my_tool", Input: nil},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	for _, msg := range openAIReq.Messages {
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "toolu_abc123" {
					t.Errorf("Tool call ID = %q, want %q (should preserve original)", tc.ID, "toolu_abc123")
				}
			}
			return
		}
	}
}

// --- Tool Name Truncation ---

func TestTruncateToolName_Short(t *testing.T) {
	name := "short_tool"
	if got := truncateToolName(name); got != name {
		t.Errorf("truncateToolName(%q) = %q, want %q", name, got, name)
	}
}

func TestTruncateToolName_Exactly64(t *testing.T) {
	name := ""
	for i := 0; i < 64; i++ {
		name += "x"
	}
	if got := truncateToolName(name); got != name {
		t.Errorf("truncateToolName(64 chars) = %q (len=%d), want original", got, len(got))
	}
}

func TestTruncateToolName_Over64(t *testing.T) {
	name := ""
	for i := 0; i < 100; i++ {
		name += "x"
	}
	got := truncateToolName(name)
	if len(got) > 64 {
		t.Errorf("truncateToolName(100 chars) length = %d, want <= 64", len(got))
	}
	got2 := truncateToolName(name)
	if got != got2 {
		t.Errorf("truncateToolName is not deterministic: %q vs %q", got, got2)
	}
}

func TestTruncateToolName_DifferentLongNames(t *testing.T) {
	name1 := ""
	name2 := ""
	for i := 0; i < 100; i++ {
		name1 += "a"
		name2 += "b"
	}
	got1 := truncateToolName(name1)
	got2 := truncateToolName(name2)
	if got1 == got2 {
		t.Errorf("Different long names should produce different truncated results: %q == %q", got1, got2)
	}
}

// --- Empty Response Handling ---

func TestLegacyConvertOpenAIToClaude_NilResponse(t *testing.T) {
	originalReq := &models.ClaudeMessagesRequest{Model: "test-model"}
	result := legacyConvertOpenAIToClaude(nil, originalReq)
	if result == nil {
		t.Fatal("Expected non-nil response for nil input")
	}
	if result.Type != "message" {
		t.Errorf("Type = %q, want %q", result.Type, "message")
	}
	if len(result.Content) == 0 {
		t.Error("Expected at least one content block")
	}
	if result.Content[0].Type != "text" {
		t.Errorf("Content type = %q, want %q", result.Content[0].Type, "text")
	}
}

func TestLegacyConvertOpenAIToClaude_EmptyChoices(t *testing.T) {
	originalReq := &models.ClaudeMessagesRequest{Model: "test-model"}
	openAIResp := &models.OpenAIResponse{
		ID:      "chatcmpl-test",
		Choices: []models.OpenAIChoice{},
		Usage:   models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}
	result := legacyConvertOpenAIToClaude(openAIResp, originalReq)
	if result == nil {
		t.Fatal("Expected non-nil response for empty choices")
	}
	if len(result.Content) == 0 {
		t.Error("Expected at least one content block for empty choices")
	}
}

// --- Tool Result Content Array Format ---

func TestToolMessageWithArrayContent(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	msg := &models.OpenAIMessage{
		Role:       "tool",
		ToolCallID: "call_123",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": "result text"},
		},
	}

	internalMsg := conv.convertOpenAIMessageToInternal(msg)
	if internalMsg.Role != "user" {
		t.Errorf("Role = %q, want %q", internalMsg.Role, "user")
	}
	if len(internalMsg.Content) != 1 {
		t.Fatalf("Content blocks = %d, want 1", len(internalMsg.Content))
	}
	if internalMsg.Content[0].Type != "tool_result" {
		t.Errorf("Content type = %q, want %q", internalMsg.Content[0].Type, "tool_result")
	}
	if internalMsg.Content[0].Content != "result text" {
		t.Errorf("Content = %q, want %q", internalMsg.Content[0].Content, "result text")
	}
}

func TestToolMessageWithEmptyContent(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	msg := &models.OpenAIMessage{
		Role:       "tool",
		ToolCallID: "call_456",
		Content:    nil,
	}

	internalMsg := conv.convertOpenAIMessageToInternal(msg)
	if internalMsg.Content[0].Content != "" {
		t.Errorf("Content = %q, want empty string for nil content", internalMsg.Content[0].Content)
	}
}

func TestThinkingOnlyAssistantMessage(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "hello"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "thinking", Text: "Let me think about this..."},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	var assistantMsg *models.OpenAIMessage
	for i := range openAIReq.Messages {
		if openAIReq.Messages[i].Role == "assistant" {
			assistantMsg = &openAIReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("No assistant message found")
	}
	if assistantMsg.Content == nil {
		t.Error("Assistant content is nil, should have content for strict providers")
	}
	if assistantMsg.Content == "" {
		t.Error("Assistant content is empty string, strict providers will reject this")
	}
	if assistantMsg.ReasoningContent == "" {
		t.Error("ReasoningContent should be set for thinking blocks")
	}
}

func TestThinkingWithTextAssistantMessage(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "hello"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "thinking", Text: "thinking..."},
					{Type: "text", Text: "Hello!"},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	var assistantMsg *models.OpenAIMessage
	for i := range openAIReq.Messages {
		if openAIReq.Messages[i].Role == "assistant" {
			assistantMsg = &openAIReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("No assistant message found")
	}
	if assistantMsg.Content != "Hello!" {
		t.Errorf("Content = %v, want 'Hello!'", assistantMsg.Content)
	}
}

func TestRedactedThinkingOnlyAssistantMessage(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "hello"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "redacted_thinking", Text: ""},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	var assistantMsg *models.OpenAIMessage
	for i := range openAIReq.Messages {
		if openAIReq.Messages[i].Role == "assistant" {
			assistantMsg = &openAIReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("No assistant message found")
	}
	if assistantMsg.Content == nil {
		t.Error("Assistant content should not be nil even for redacted_thinking only")
	}
}

func TestToolCallSequenceMismatch(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "read files"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "toolu_01", Name: "Read", Input: map[string]interface{}{"path": "a.txt"}},
					{Type: "tool_use", ID: "toolu_02", Name: "Read", Input: map[string]interface{}{"path": "b.txt"}},
				},
			},
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "tool_result", ToolUseID: "toolu_01", Content: "content of a"},
					{Type: "text", Text: "what about b?"},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	toolMsgCount := 0
	for _, m := range openAIReq.Messages {
		if m.Role == "tool" {
			toolMsgCount++
		}
	}

	if toolMsgCount != 2 {
		t.Errorf("Expected 2 tool messages, got %d", toolMsgCount)
		for i, m := range openAIReq.Messages {
			t.Logf("  [%d] role=%s tool_call_id=%s", i, m.Role, m.ToolCallID)
		}
	}
}
