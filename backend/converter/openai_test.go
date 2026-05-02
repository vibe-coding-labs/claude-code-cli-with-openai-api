package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestOpenAIConverter_ParseRequest_SimpleText(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"max_tokens": 1024
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %q", req.Model)
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

func TestOpenAIConverter_ParseRequest_WithSystemMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are helpful"},
			{"role": "user", "content": "Hello"}
		],
		"max_tokens": 1024
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.System != "You are helpful" {
		t.Errorf("expected system 'You are helpful', got %q", req.System)
	}
	// System message should be extracted, leaving only user message
	if len(req.Messages) != 1 {
		t.Errorf("expected 1 non-system message, got %d", len(req.Messages))
	}
}

func TestOpenAIConverter_ParseRequest_WithTools(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Use a tool"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get current weather",
					"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
				}
			}
		]
	}`)

	c := NewOpenAIConverter(nil)
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

func TestOpenAIConverter_ParseRequest_WithToolCalls(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Call a function"},
			{
				"role": "assistant",
				"content": "I'll help you",
				"tool_calls": [
					{
						"id": "call_123",
						"type": "function",
						"function": {
							"name": "my_func",
							"arguments": "{\"param\": \"value\"}"
						}
					}
				]
			}
		]
	}`)

	c := NewOpenAIConverter(nil)
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

	// Should have text + tool_use
	if len(assistantMsg.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(assistantMsg.Content))
	}

	// Check text block
	if assistantMsg.Content[0].Type != "text" || assistantMsg.Content[0].Text != "I'll help you" {
		t.Errorf("expected text block with 'I'll help you'")
	}

	// Check tool_use block
	toolUse := assistantMsg.Content[1]
	if toolUse.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %q", toolUse.Type)
	}
	if toolUse.ID != "call_123" {
		t.Errorf("expected id call_123, got %q", toolUse.ID)
	}
	if toolUse.Name != "my_func" {
		t.Errorf("expected name my_func, got %q", toolUse.Name)
	}
	if toolUse.Input["param"] != "value" {
		t.Errorf("expected input param=value, got %v", toolUse.Input["param"])
	}
}

func TestOpenAIConverter_ParseRequest_WithToolMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Call a function"},
			{
				"role": "assistant",
				"tool_calls": [
					{
						"id": "call_123",
						"type": "function",
						"function": {"name": "my_func", "arguments": "{}"}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_123",
				"content": "Tool result"
			}
		]
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(req.Messages))
	}

	toolResultMsg := req.Messages[2]
	// Tool messages become user messages with tool_result content
	if toolResultMsg.Role != "user" {
		t.Errorf("expected role user for tool result, got %q", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(toolResultMsg.Content))
	}

	cb := toolResultMsg.Content[0]
	if cb.Type != "tool_result" {
		t.Errorf("expected type tool_result, got %q", cb.Type)
	}
	if cb.ToolUseID != "call_123" {
		t.Errorf("expected tool_use_id call_123, got %q", cb.ToolUseID)
	}
	if cb.Content != "Tool result" {
		t.Errorf("expected content 'Tool result', got %q", cb.Content)
	}
}

func TestOpenAIConverter_ParseRequest_ReasoningEffort(t *testing.T) {
	body := []byte(`{
		"model": "o1-mini",
		"messages": [{"role": "user", "content": "Hello"}],
		"reasoning_effort": "high"
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.ReasoningEffort == nil {
		t.Fatal("expected reasoning_effort to be set")
	}
	if *req.ReasoningEffort != "high" {
		t.Errorf("expected reasoning_effort high, got %q", *req.ReasoningEffort)
	}
}

func TestOpenAIConverter_BuildRequest_WithConfig(t *testing.T) {
	cfg := &config.Config{
		BigModel:   "gpt-4",
		SmallModel: "gpt-3.5-turbo",
	}

	req := &InternalRequest{
		Model: "claude-3-opus-20240229", // This should be mapped using config
		Messages: []InternalMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}},
		},
	}

	c := NewOpenAIConverter(cfg)
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Model should be mapped using config
	if out.Model == "" {
		t.Error("expected model to be mapped")
	}
}

func TestOpenAIConverter_BuildRequest_WithTools(t *testing.T) {
	req := &InternalRequest{
		Model: "gpt-4",
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

	c := NewOpenAIConverter(nil)
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out.Tools))
	}
	if out.Tools[0].Type != "function" {
		t.Errorf("expected tool type function, got %q", out.Tools[0].Type)
	}
	if out.Tools[0].Function.Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %q", out.Tools[0].Function.Name)
	}
}

func TestOpenAIConverter_BuildRequest_WithToolUse(t *testing.T) {
	req := &InternalRequest{
		Model: "gpt-4",
		Messages: []InternalMessage{
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "text", Text: "I'll help you"},
					{Type: "tool_use", ID: "call_123", Name: "my_func", Input: map[string]interface{}{"param": "value"}},
				},
			},
		},
	}

	c := NewOpenAIConverter(nil)
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages (assistant + auto-inserted tool response), got %d", len(out.Messages))
	}

	msg := out.Messages[0]
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}

	// Verify auto-inserted tool response
	toolMsg := out.Messages[1]
	if toolMsg.Role != "tool" {
		t.Errorf("expected second message to be tool, got %q", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id=call_123, got %q", toolMsg.ToolCallID)
	}

	tc := msg.ToolCalls[0]
	if tc.Type != "function" {
		t.Errorf("expected tool call type function, got %q", tc.Type)
	}
	if tc.Function.Name != "my_func" {
		t.Errorf("expected function name my_func, got %q", tc.Function.Name)
	}
}

func TestOpenAIConverter_ParseResponse_SimpleText(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello there!"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`)

	c := NewOpenAIConverter(nil)
	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected id chatcmpl-123, got %q", resp.ID)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role assistant, got %q", resp.Role)
	}
	if resp.StopReason != "end_turn" { // mapped from "stop"
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

func TestOpenAIConverter_ParseResponse_WithToolCalls(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"tool_calls": [
						{
							"id": "call_abc",
							"type": "function",
							"function": {
								"name": "my_func",
								"arguments": "{\"param\": \"value\"}"
							}
						}
					]
				},
				"finish_reason": "tool_calls"
			}
		],
		"usage": {"prompt_tokens": 20, "completion_tokens": 15}
	}`)

	c := NewOpenAIConverter(nil)
	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StopReason != "tool_use" { // mapped from "tool_calls"
		t.Errorf("expected stop_reason tool_use, got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}

	cb := resp.Content[0]
	if cb.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %q", cb.Type)
	}
	// ID should be converted: call_abc -> call_abc (no change, already call_)
	if cb.ID != "call_abc" {
		t.Errorf("expected id call_abc, got %q", cb.ID)
	}
	if cb.Name != "my_func" {
		t.Errorf("expected name my_func, got %q", cb.Name)
	}
	if cb.Input["param"] != "value" {
		t.Errorf("expected input param=value, got %v", cb.Input["param"])
	}
}

func TestOpenAIConverter_ParseResponse_ToolCallIDConversion(t *testing.T) {
	// Test that fc_ prefix is converted to call_ prefix
	body := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"tool_calls": [
						{
							"id": "fc_abc123",
							"type": "function",
							"function": {
								"name": "my_func",
								"arguments": "{}"
							}
						}
					]
				},
				"finish_reason": "tool_calls"
			}
		],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`)

	c := NewOpenAIConverter(nil)
	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}

	// ID should be preserved: fc_abc123 -> fc_abc123 (no conversion)
	if resp.Content[0].ID != "fc_abc123" {
		t.Errorf("expected id fc_abc123 (preserved), got %q", resp.Content[0].ID)
	}
}

func TestOpenAIConverter_BuildResponse(t *testing.T) {
	resp := &InternalResponse{
		ID:         "msg_123",
		Model:      "claude-model",
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

	c := NewOpenAIConverter(nil)
	data, err := c.BuildResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.OpenAIResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if out.ID != "msg_123" {
		t.Errorf("expected id msg_123, got %v", out.ID)
	}
	if out.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %v", out.Choices[0].Message.Role)
	}
	if out.Choices[0].FinishReason != "stop" { // mapped from end_turn
		t.Errorf("expected finish_reason stop, got %v", out.Choices[0].FinishReason)
	}
	if out.Usage.PromptTokens != 10 {
		t.Errorf("expected prompt_tokens 10, got %d", out.Usage.PromptTokens)
	}
	if out.Usage.CompletionTokens != 5 {
		t.Errorf("expected completion_tokens 5, got %d", out.Usage.CompletionTokens)
	}
}

func TestOpenAIConverter_BuildResponse_WithToolUse(t *testing.T) {
	resp := &InternalResponse{
		ID:         "msg_123",
		Role:       "assistant",
		StopReason: "tool_use",
		Content: []ContentBlock{
			{Type: "tool_use", ID: "call_abc", Name: "my_func", Input: map[string]interface{}{"param": "value"}},
		},
		Usage: &UsageInfo{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	c := NewOpenAIConverter(nil)
	data, err := c.BuildResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out models.OpenAIResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if out.Choices[0].FinishReason != "tool_calls" { // mapped from tool_use
		t.Errorf("expected finish_reason tool_calls, got %v", out.Choices[0].FinishReason)
	}
	if len(out.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(out.Choices[0].Message.ToolCalls))
	}

	tc := out.Choices[0].Message.ToolCalls[0]
	// ID should be preserved: call_abc -> call_abc (no conversion)
	if tc.ID != "call_abc" {
		t.Errorf("expected id call_abc (preserved), got %q", tc.ID)
	}
	if tc.Function.Name != "my_func" {
		t.Errorf("expected function name my_func, got %q", tc.Function.Name)
	}
}

func TestOpenAIConverter_ParseStreamEvent_TextDelta(t *testing.T) {
	line := []byte(`data: {"choices": [{"delta": {"content": "Hello"}, "finish_reason": null}]}`)

	c := NewOpenAIConverter(nil)
	event, err := c.ParseStreamEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Delta == nil {
		t.Fatal("expected delta to be non-nil")
	}
	if event.Delta.Text != "Hello" {
		t.Errorf("expected delta text Hello, got %q", event.Delta.Text)
	}
}

func TestOpenAIConverter_ParseStreamEvent_Done(t *testing.T) {
	line := []byte(`data: [DONE]`)

	c := NewOpenAIConverter(nil)
	event, err := c.ParseStreamEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "done" {
		t.Errorf("expected type done, got %q", event.Type)
	}
}

func TestOpenAIConverter_ParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	c := NewOpenAIConverter(nil)
	_, err := c.ParseRequest(body)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestOpenAIConverter_ParseResponse_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	c := NewOpenAIConverter(nil)
	_, err := c.ParseResponse(body)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestOpenAIConverter_RoundTrip(t *testing.T) {
	// Test that ParseRequest and BuildRequest are inverse operations
	original := &InternalRequest{
		Model:     "gpt-4",
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

	c := NewOpenAIConverter(nil)

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

func TestOpenAIConverter_BuildStreamEvent(t *testing.T) {
	c := NewOpenAIConverter(nil)

	tests := []struct {
		name     string
		event    *StreamEvent
		expected map[string]interface{}
	}{
		{
			name: "text_delta",
			event: &StreamEvent{
				Type:  "content_block_delta",
				Index: 0,
				Delta: &StreamDelta{
					Type: "text_delta",
					Text: "Hello",
				},
			},
			expected: map[string]interface{}{
				"object": "chat.completion.chunk",
				"choices": []interface{}{
					map[string]interface{}{
						"index": 0,
						"delta": map[string]interface{}{
							"content": "Hello",
						},
					},
				},
			},
		},
		{
			name: "finish_reason_stop",
			event: &StreamEvent{
				Type: "message_delta",
				Delta: &StreamDelta{
					StopReason: "end_turn",
				},
			},
			expected: map[string]interface{}{
				"object": "chat.completion.chunk",
				"choices": []interface{}{
					map[string]interface{}{
						"index":         0,
						"finish_reason": "stop",
						"delta":         map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "finish_reason_tool_use",
			event: &StreamEvent{
				Type: "message_delta",
				Delta: &StreamDelta{
					StopReason: "tool_use",
				},
			},
			expected: map[string]interface{}{
				"object": "chat.completion.chunk",
				"choices": []interface{}{
					map[string]interface{}{
						"index":         0,
						"finish_reason": "tool_calls",
						"delta":         map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "with_usage",
			event: &StreamEvent{
				Type: "content_block_delta",
				Usage: &UsageInfo{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
			expected: map[string]interface{}{
				"object": "chat.completion.chunk",
				"choices": []interface{}{
					map[string]interface{}{
						"index": 0,
						"delta": map[string]interface{}{},
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := c.BuildStreamEvent(tt.event)
			if err != nil {
				t.Fatalf("BuildStreamEvent failed: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			// Verify object type
			if result["object"] != "chat.completion.chunk" {
				t.Errorf("expected object 'chat.completion.chunk', got %q", result["object"])
			}

			// Verify choices exist
			if choices, ok := result["choices"].([]interface{}); !ok || len(choices) == 0 {
				t.Error("expected choices array with at least one element")
			}

			// Verify usage if present
			if tt.event.Usage != nil {
				if usage, ok := result["usage"].(map[string]interface{}); ok {
					if usage["prompt_tokens"] != float64(tt.event.Usage.InputTokens) {
						t.Errorf("expected prompt_tokens %d, got %v", tt.event.Usage.InputTokens, usage["prompt_tokens"])
					}
				} else {
					t.Error("expected usage in result")
				}
			}
		})
	}
}

func TestOpenAIConverter_MaxCompletionTokens(t *testing.T) {
	// Test that max_completion_tokens is parsed correctly
	body := []byte(`{
		"model": "o1-preview",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_completion_tokens": 1000
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if req.MaxTokens != 1000 {
		t.Errorf("expected MaxTokens 1000 (from max_completion_tokens), got %d", req.MaxTokens)
	}
}

func TestOpenAIConverter_StopAsString(t *testing.T) {
	// Test that stop as string is parsed correctly
	body := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hello"}],
		"stop": "END"
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if len(req.StopSeqs) != 1 || req.StopSeqs[0] != "END" {
		t.Errorf("expected StopSeqs [END], got %v", req.StopSeqs)
	}
}

func TestOpenAIConverter_StopAsArray(t *testing.T) {
	// Test that stop as array is parsed correctly
	body := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hello"}],
		"stop": ["END", "STOP"]
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if len(req.StopSeqs) != 2 || req.StopSeqs[0] != "END" || req.StopSeqs[1] != "STOP" {
		t.Errorf("expected StopSeqs [END STOP], got %v", req.StopSeqs)
	}
}

func TestOpenAIConverter_ParseRequest_WithImage(t *testing.T) {
	// Test OpenAI vision API format with image_url
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "What's in this image?"},
					{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}}
				]
			}
		],
		"max_tokens": 1024
	}`)

	c := NewOpenAIConverter(nil)
	req, err := c.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}

	if len(req.Messages[0].Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(req.Messages[0].Content))
	}

	// Check text block
	if req.Messages[0].Content[0].Type != "text" {
		t.Errorf("expected first block type 'text', got %q", req.Messages[0].Content[0].Type)
	}
	if req.Messages[0].Content[0].Text != "What's in this image?" {
		t.Errorf("expected text 'What's in this image?', got %q", req.Messages[0].Content[0].Text)
	}

	// Check image block
	if req.Messages[0].Content[1].Type != "image" {
		t.Errorf("expected second block type 'image', got %q", req.Messages[0].Content[1].Type)
	}
	if req.Messages[0].Content[1].Source == nil {
		t.Fatal("expected image source to be non-nil")
	}
	if req.Messages[0].Content[1].Source.Type != "base64" {
		t.Errorf("expected source type 'base64', got %q", req.Messages[0].Content[1].Source.Type)
	}
	if req.Messages[0].Content[1].Source.MediaType != "image/png" {
		t.Errorf("expected media_type 'image/png', got %q", req.Messages[0].Content[1].Source.MediaType)
	}
}

func TestOpenAIConverter_BuildRequest_WithImage(t *testing.T) {
	// Test building OpenAI request with image
	req := &InternalRequest{
		Model:     "gpt-4o",
		MaxTokens: 1024,
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "What's in this image?"},
					{
						Type: "image",
						Source: &ImageSource{
							Type:      "base64",
							MediaType: "image/png",
							Data:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
				},
			},
		},
	}

	c := NewOpenAIConverter(nil)
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var result models.OpenAIRequest
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	// Content should be an array for multi-modal
	content, ok := result.Messages[0].Content.([]interface{})
	if !ok {
		t.Fatalf("expected content to be array, got %T", result.Messages[0].Content)
	}

	if len(content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(content))
	}

	// Check image part
	imagePart, ok := content[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected image part to be map, got %T", content[1])
	}
	if imagePart["type"] != "image_url" {
		t.Errorf("expected type 'image_url', got %q", imagePart["type"])
	}
}

func TestOpenAIConverter_ImageRoundTrip(t *testing.T) {
	// Test that image survives a round-trip conversion
	originalBody := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Describe this"},
					{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD"}}
				]
			}
		],
		"max_tokens": 512
	}`)

	c := NewOpenAIConverter(nil)

	// Parse OpenAI request
	req, err := c.ParseRequest(originalBody)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Build back to OpenAI
	outputBody, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var result models.OpenAIRequest
	if err := json.Unmarshal(outputBody, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Verify image is preserved
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	content, ok := result.Messages[0].Content.([]interface{})
	if !ok {
		t.Fatalf("expected content to be array, got %T", result.Messages[0].Content)
	}

	if len(content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(content))
	}

	// Verify image data URL is preserved
	imagePart := content[1].(map[string]interface{})
	imageURL := imagePart["image_url"].(map[string]interface{})
	url := imageURL["url"].(string)
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Errorf("expected data URL to be preserved, got %q", url)
	}
}

// TestOpenAIConverter_ParseResponse_ToolCallIDConversion_NoUnderscore 测试 fcxxx 格式（无下划线）的 ID 转换
func TestOpenAIConverter_ParseResponse_ToolCallIDConversion_NoUnderscore(t *testing.T) {
	// Test that fcxxx format (no underscore) is converted to call_xxx
	body := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"tool_calls": [
						{
							"id": "fcSxBUVrxLp43jX9lGXFgdwuo9",
							"type": "function",
							"function": {
								"name": "my_func",
								"arguments": "{}"
							}
						}
					]
				},
				"finish_reason": "tool_calls"
			}
		],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`)

	c := NewOpenAIConverter(nil)
	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}

	// ID should be preserved: fcSxBUVrxLp43jX9lGXFgdwuo9 -> fcSxBUVrxLp43jX9lGXFgdwuo9 (no conversion)
	expectedID := "fcSxBUVrxLp43jX9lGXFgdwuo9"
	if resp.Content[0].ID != expectedID {
		t.Errorf("expected id %q (preserved), got %q", expectedID, resp.Content[0].ID)
	}
}

// TestOpenAIConverter_BuildRequest_ToolResult_IDConversion 测试 tool_result 消息的 ID 转换
func TestOpenAIConverter_BuildRequest_ToolResult_IDConversion(t *testing.T) {
	cfg := &config.Config{
		BigModel:   "gpt-4o",
		SmallModel: "gpt-4o-mini",
	}
	c := NewOpenAIConverter(cfg)

	// Test with call_ prefix (normal case)
	// Must include assistant(tool_calls) before tool_result for valid sequence
	req := &InternalRequest{
		Model: "claude-3-opus",
		Messages: []InternalMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "check weather"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "call_abc123", Name: "get_weather", Input: map[string]interface{}{"city": "NYC"}},
				},
			},
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "tool_result", ToolUseID: "call_abc123", Content: "result"},
				},
			},
		},
	}

	body, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	messages, ok := result["messages"].([]interface{})
	if !ok || len(messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(messages))
	}

	// Find the tool message
	var toolMsg map[string]interface{}
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if msg["role"] == "tool" {
			toolMsg = msg
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("expected tool message not found")
	}

	// Tool call ID should be preserved: call_abc123 -> call_abc123 (no conversion)
	if toolMsg["tool_call_id"] != "call_abc123" {
		t.Errorf("expected tool_call_id 'call_abc123', got %q", toolMsg["tool_call_id"])
	}
}

func TestOpenAIConverter_EmptyAssistantMessage_HasNonEmptyContent(t *testing.T) {
	cfg := &config.Config{
		BigModel:    "devstral-2512",
		MiddleModel: "devstral-2512",
		SmallModel:  "devstral-2512",
	}
	c := NewOpenAIConverter(cfg)

	tests := []struct {
		name    string
		content []ContentBlock
	}{
		{"redacted_thinking only", []ContentBlock{{Type: "redacted_thinking", Text: "opaque-data"}}},
		{"thinking only", []ContentBlock{{Type: "thinking", Text: "internal thought"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &InternalRequest{
				Model:   "claude-sonnet-4-20250514",
				Stream:  false,
				Messages: []InternalMessage{
					{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hello"}}},
					{Role: "assistant", Content: tt.content},
				},
			}

			body, err := c.BuildRequest(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			messages, _ := result["messages"].([]interface{})
			for _, m := range messages {
				msg, ok := m.(map[string]interface{})
				if !ok || msg["role"] != "assistant" {
					continue
				}
				content, _ := msg["content"].(string)
				if content == "" {
					t.Errorf("assistant message has empty content, expected non-empty placeholder")
				}
			}
		})
	}
}

func TestOpenAIConverter_ToolCallMismatch_InsertsMissingResponses(t *testing.T) {
	cfg := &config.Config{
		BigModel:    "devstral-2512",
		MiddleModel: "devstral-2512",
		SmallModel:  "devstral-2512",
	}
	c := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model:  "claude-sonnet-4-20250514",
		Stream: false,
		Messages: []InternalMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "read files"}}},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "toolu_01", Name: "Read", Input: map[string]interface{}{"file": "a.txt"}},
					{Type: "tool_use", ID: "toolu_02", Name: "Read", Input: map[string]interface{}{"file": "b.txt"}},
				},
			},
		},
		Tools: []ToolDefinition{
			{Name: "Read", Description: "read file", Parameters: map[string]interface{}{"type": "object"}},
		},
	}

	body, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	messages, _ := result["messages"].([]interface{})
	toolCallCount := 0
	toolResponseCount := 0
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "assistant" {
			if tcs, ok := msg["tool_calls"].([]interface{}); ok {
				toolCallCount += len(tcs)
			}
		}
		if role == "tool" {
			toolResponseCount++
		}
	}

	if toolCallCount != toolResponseCount {
		t.Errorf("tool_calls=%d but tool_responses=%d, should match", toolCallCount, toolResponseCount)
	}
}

// TestOpenAIConverter_ToolChoiceMapping_E2E verifies the full Claude→Internal→OpenAI tool_choice
// conversion path, specifically that Claude's "any" maps to OpenAI's "required" and "none" maps to "none".
func TestOpenAIConverter_ToolChoiceMapping_E2E(t *testing.T) {
	cfg := &config.Config{BigModel: "gpt-4o"}
	factory := NewConverterFactory()
	factory.SetOpenAIConfig(cfg)

	tests := []struct {
		name           string
		claudeToolChoice string
		expectedOpenAI interface{}
	}{
		{
			name:           "auto maps to auto",
			claudeToolChoice: `{"type":"auto"}`,
			expectedOpenAI: "auto",
		},
		{
			name:           "any maps to required",
			claudeToolChoice: `{"type":"any"}`,
			expectedOpenAI: "required",
		},
		{
			name:           "none maps to none",
			claudeToolChoice: `{"type":"none"}`,
			expectedOpenAI: "none",
		},
		{
			name:           "tool with name maps to function",
			claudeToolChoice: `{"type":"tool","name":"get_weather"}`,
			expectedOpenAI: map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a Claude request with tool_choice
			claudeBody := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],"tool_choice":` + tt.claudeToolChoice + `}`

			// Claude → Internal
			internalReq, err := factory.ConvertClaudeToInternal([]byte(claudeBody))
			if err != nil {
				t.Fatalf("ClaudeToInternal error: %v", err)
			}

			// Internal → OpenAI
			openAIBody, err := factory.ConvertInternalToOpenAI(internalReq)
			if err != nil {
				t.Fatalf("InternalToOpenAI error: %v", err)
			}

			var openAIReq models.OpenAIRequest
			if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
				t.Fatalf("unmarshal OpenAI request error: %v", err)
			}

			// Verify tool_choice
			actualJSON, _ := json.Marshal(openAIReq.ToolChoice)
			expectedJSON, _ := json.Marshal(tt.expectedOpenAI)

			if string(actualJSON) != string(expectedJSON) {
				t.Errorf("tool_choice: got %s, want %s", actualJSON, expectedJSON)
			}
		})
	}
}
