package converter

import (
	"encoding/json"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestClaudeConverter_ParseRequest_SimpleText(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"max_tokens": 1024
	}`)

	c := NewClaudeConverter()
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %q", req.Model)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", req.MaxTokens)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Errorf("expected role user, got %q", req.Messages[0].Role)
	}
	if len(req.Messages[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(req.Messages[0].Content))
	}
	if req.Messages[0].Content[0].Text != "Hello" {
		t.Errorf("expected text Hello, got %q", req.Messages[0].Content[0].Text)
	}
}

func TestClaudeConverter_ParseRequest_WithSystem(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "Hi"}],
		"system": "You are a helpful assistant",
		"max_tokens": 512
	}`)

	c := NewClaudeConverter()
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.System != "You are a helpful assistant" {
		t.Errorf("expected system prompt, got %q", req.System)
	}
}

func TestClaudeConverter_ParseRequest_WithTools(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "Use a tool"}],
		"max_tokens": 1024,
		"tools": [
			{
				"name": "get_weather",
				"description": "Get current weather",
				"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
			}
		]
	}`)

	c := NewClaudeConverter()
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %q", req.Tools[0].Name)
	}
	if req.Tools[0].Description != "Get current weather" {
		t.Errorf("expected tool description, got %q", req.Tools[0].Description)
	}
}

func TestClaudeConverter_ParseRequest_WithToolUse(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [
			{"role": "user", "content": "Call my function"},
			{"role": "assistant", "content": [
				{"type": "tool_use", "id": "toolu_01", "name": "my_func", "input": {"param": "value"}}
			]}
		],
		"max_tokens": 1024
	}`)

	c := NewClaudeConverter()
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	assistantMsg := req.Messages[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("expected assistant role, got %q", assistantMsg.Role)
	}
	if len(assistantMsg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(assistantMsg.Content))
	}

	cb := assistantMsg.Content[0]
	if cb.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %q", cb.Type)
	}
	if cb.ID != "toolu_01" {
		t.Errorf("expected id toolu_01, got %q", cb.ID)
	}
	if cb.Name != "my_func" {
		t.Errorf("expected name my_func, got %q", cb.Name)
	}
	if cb.Input["param"] != "value" {
		t.Errorf("expected input param=value, got %v", cb.Input["param"])
	}
}

func TestClaudeConverter_ParseRequest_WithToolResult(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [
			{"role": "user", "content": "Call my function"},
			{"role": "assistant", "content": [
				{"type": "tool_use", "id": "toolu_01", "name": "my_func", "input": {"param": "value"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "toolu_01", "content": "Result data"}
			]}
		],
		"max_tokens": 1024
	}`)

	c := NewClaudeConverter()
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(req.Messages))
	}

	toolResultMsg := req.Messages[2]
	if toolResultMsg.Role != "user" {
		t.Errorf("expected user role for tool result, got %q", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(toolResultMsg.Content))
	}

	cb := toolResultMsg.Content[0]
	if cb.Type != "tool_result" {
		t.Errorf("expected type tool_result, got %q", cb.Type)
	}
	if cb.ToolUseID != "toolu_01" {
		t.Errorf("expected tool_use_id toolu_01, got %q", cb.ToolUseID)
	}
	if cb.Content != "Result data" {
		t.Errorf("expected content 'Result data', got %q", cb.Content)
	}
}

func TestClaudeConverter_ParseRequest_Streaming(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "Stream?"}],
		"max_tokens": 100,
		"stream": true
	}`)

	c := NewClaudeConverter()
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !req.Stream {
		t.Error("expected stream=true")
	}
}

func TestClaudeConverter_BuildRequest_SimpleText(t *testing.T) {
	req := &InternalRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	c := NewClaudeConverter()
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.ClaudeMessagesRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if out.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %v", out.Model)
	}
	if out.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %v", out.MaxTokens)
	}

	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message in output")
	}
	if out.Messages[0].Role != "user" {
		t.Errorf("expected role user, got %v", out.Messages[0].Role)
	}
	// Single text block should be serialized as a string
	if content, ok := out.Messages[0].Content.(string); ok {
		if content != "Hello" {
			t.Errorf("expected content Hello, got %v", content)
		}
	} else {
		t.Errorf("expected string content for single text block")
	}
}

func TestClaudeConverter_BuildRequest_WithSystem(t *testing.T) {
	req := &InternalRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 512,
		System:    "You are helpful",
		Messages: []InternalMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hi"}}},
		},
	}

	c := NewClaudeConverter()
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.ClaudeMessagesRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	systemText, ok := out.System.(string)
	if !ok || systemText != "You are helpful" {
		t.Errorf("expected system You are helpful, got %v", out.System)
	}
}

func TestClaudeConverter_BuildRequest_WithTools(t *testing.T) {
	req := &InternalRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []InternalMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Use a tool"}}},
		},
		Tools: []ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		},
	}

	c := NewClaudeConverter()
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.ClaudeMessagesRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out.Tools))
	}
	if out.Tools[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %v", out.Tools[0].Name)
	}
}

func TestClaudeConverter_ParseResponse_SimpleText(t *testing.T) {
	body := []byte(`{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [{"type": "text", "text": "Hello there!"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	c := NewClaudeConverter()
	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "msg_123" {
		t.Errorf("expected id msg_123, got %q", resp.ID)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role assistant, got %q", resp.Role)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello there!" {
		t.Errorf("expected text Hello there!, got %q", resp.Content[0].Text)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens 5, got %d", resp.Usage.OutputTokens)
	}
}

func TestClaudeConverter_ParseResponse_WithToolUse(t *testing.T) {
	body := []byte(`{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{"type": "tool_use", "id": "toolu_01", "name": "my_func", "input": {"param": "value"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 20, "output_tokens": 15}
	}`)

	c := NewClaudeConverter()
	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason tool_use, got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}

	cb := resp.Content[0]
	if cb.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %q", cb.Type)
	}
	if cb.ID != "toolu_01" {
		t.Errorf("expected id toolu_01, got %q", cb.ID)
	}
	if cb.Name != "my_func" {
		t.Errorf("expected name my_func, got %q", cb.Name)
	}
}

func TestClaudeConverter_BuildResponse(t *testing.T) {
	resp := &InternalResponse{
		ID:         "msg_123",
		Model:      "claude-sonnet-4-6",
		Role:       "assistant",
		StopReason: "end_turn",
		Content: []ContentBlock{
			{Type: "text", Text: "Hello there!"},
		},
		Usage: &UsageInfo{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	c := NewClaudeConverter()
	data, err := c.BuildResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.ClaudeResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if out.ID != "msg_123" {
		t.Errorf("expected id msg_123, got %v", out.ID)
	}
	if out.Role != "assistant" {
		t.Errorf("expected role assistant, got %v", out.Role)
	}
	if out.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %v", out.StopReason)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	if out.Content[0].Text != "Hello there!" {
		t.Errorf("expected text Hello there!, got %v", out.Content[0].Text)
	}
}

func TestClaudeConverter_ParseStreamEvent_ContentBlockDelta(t *testing.T) {
	line := []byte(`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}`)

	c := NewClaudeConverter()
	event, err := c.ParseStreamEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "content_block_delta" {
		t.Errorf("expected type content_block_delta, got %q", event.Type)
	}
	if event.Index != 0 {
		t.Errorf("expected index 0, got %d", event.Index)
	}
	if event.Delta == nil {
		t.Fatal("expected delta to be non-nil")
	}
	if event.Delta.Text != "Hello" {
		t.Errorf("expected delta text Hello, got %q", event.Delta.Text)
	}
}

func TestClaudeConverter_ParseStreamEvent_MessageDelta(t *testing.T) {
	line := []byte(`{"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {"output_tokens": 10}}`)

	c := NewClaudeConverter()
	event, err := c.ParseStreamEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "message_delta" {
		t.Errorf("expected type message_delta, got %q", event.Type)
	}
	if event.Delta == nil {
		t.Fatal("expected delta to be non-nil")
	}
	if event.Delta.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %q", event.Delta.StopReason)
	}
	if event.Usage == nil {
		t.Fatal("expected usage to be non-nil")
	}
	if event.Usage.OutputTokens != 10 {
		t.Errorf("expected output_tokens 10, got %d", event.Usage.OutputTokens)
	}
}

func TestClaudeConverter_ParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	c := NewClaudeConverter()
	_, err := c.ParseRequest(body)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestClaudeConverter_ParseResponse_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	c := NewClaudeConverter()
	_, err := c.ParseResponse(body)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestClaudeConverter_RoundTrip(t *testing.T) {
	// Test that ParseRequest and BuildRequest are inverse operations
	original := &InternalRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		System:    "You are helpful",
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "text", Text: "Hi there!"},
				},
			},
		},
		Tools: []ToolDefinition{
			{
				Name:        "test_tool",
				Description: "A test tool",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		},
	}

	c := NewClaudeConverter()

	// Build request
	data, err := c.BuildRequest(original)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Parse it back
	parsed, err := c.ParseRequest(data)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify key fields
	if parsed.Model != original.Model {
		t.Errorf("model mismatch: expected %q, got %q", original.Model, parsed.Model)
	}
	if parsed.MaxTokens != original.MaxTokens {
		t.Errorf("max_tokens mismatch: expected %d, got %d", original.MaxTokens, parsed.MaxTokens)
	}
	if parsed.System != original.System {
		t.Errorf("system mismatch: expected %q, got %q", original.System, parsed.System)
	}
	if len(parsed.Messages) != len(original.Messages) {
		t.Errorf("messages length mismatch: expected %d, got %d", len(original.Messages), len(parsed.Messages))
	}
	if len(parsed.Tools) != len(original.Tools) {
		t.Errorf("tools length mismatch: expected %d, got %d", len(original.Tools), len(parsed.Tools))
	}
}

func TestClaudeConverter_BuildStreamEvent(t *testing.T) {
	c := NewClaudeConverter()

	event := &StreamEvent{
		Type:  "content_block_delta",
		Index: 0,
		Delta: &StreamDelta{
			Type: "text_delta",
			Text: "Hello world",
		},
	}

	data, err := c.BuildStreamEvent(event)
	if err != nil {
		t.Fatalf("BuildStreamEvent failed: %v", err)
	}

	// Verify it can be unmarshalled back
	var result StreamEvent
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.Type != "content_block_delta" {
		t.Errorf("expected type 'content_block_delta', got %q", result.Type)
	}
	if result.Index != 0 {
		t.Errorf("expected index 0, got %d", result.Index)
	}
	if result.Delta == nil {
		t.Fatal("expected delta to be non-nil")
	}
	if result.Delta.Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %q", result.Delta.Text)
	}
}
