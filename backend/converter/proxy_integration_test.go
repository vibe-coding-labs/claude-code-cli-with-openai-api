package converter

// Integration tests verifying the full Claude↔OpenAI protocol conversion chain.
// These tests exercise the real conversion functions with realistic data patterns
// that Claude Code CLI sends through the proxy.

import (
	
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ============================================================================
// Non-Streaming Integration Tests
// ============================================================================

func TestIntegration_NonStreaming_TextOnly(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: time.Now().Unix(),
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:    "assistant",
					Content: "Hello! How can I help you?",
				},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 8,
		},
	}

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Hi"}}},
		},
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)

	if claudeResp.ID != "chatcmpl-test" {
		t.Errorf("ID = %s, want chatcmpl-test", claudeResp.ID)
	}
	if claudeResp.Type != "message" {
		t.Errorf("Type = %s, want message", claudeResp.Type)
	}
	if claudeResp.Role != "assistant" {
		t.Errorf("Role = %s, want assistant", claudeResp.Role)
	}
	if claudeResp.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %s, want claude-sonnet-4-6", claudeResp.Model)
	}
	if len(claudeResp.Content) != 1 {
		t.Fatalf("Content blocks = %d, want 1", len(claudeResp.Content))
	}
	if claudeResp.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %s, want text", claudeResp.Content[0].Type)
	}
	if claudeResp.Content[0].Text != "Hello! How can I help you?" {
		t.Errorf("Content[0].Text = %s, want 'Hello! How can I help you?'", claudeResp.Content[0].Text)
	}
	if claudeResp.StopReason != "end_turn" {
		t.Errorf("StopReason = %s, want end_turn", claudeResp.StopReason)
	}
	if claudeResp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want 10", claudeResp.Usage.InputTokens)
	}
	if claudeResp.Usage.OutputTokens != 8 {
		t.Errorf("Usage.OutputTokens = %d, want 8", claudeResp.Usage.OutputTokens)
	}
}

func TestIntegration_NonStreaming_ToolCall(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:      "chatcmpl-tool",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: time.Now().Unix(),
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:    "assistant",
					Content: nil,
					ToolCalls: []models.OpenAIToolCall{
						{
							Index: 0,
							ID:    "call_abc123",
							Type:  "function",
							Function: models.OpenAIFunctionCall{
								Name:      "Read",
								Arguments: `{"file_path":"/etc/hosts"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     50,
			CompletionTokens: 20,
		},
	}

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read hosts file"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)

	if claudeResp.StopReason != "tool_use" {
		t.Errorf("StopReason = %s, want tool_use", claudeResp.StopReason)
	}

	toolBlocks := []models.ClaudeContentBlock{}
	for _, b := range claudeResp.Content {
		if b.Type == "tool_use" {
			toolBlocks = append(toolBlocks, b)
		}
	}
	if len(toolBlocks) != 1 {
		t.Fatalf("tool_use blocks = %d, want 1", len(toolBlocks))
	}

	tb := toolBlocks[0]
	if tb.Name != "Read" {
		t.Errorf("tool name = %s, want Read", tb.Name)
	}
	if tb.ID != "call_abc123" {
		t.Errorf("tool ID = %s, want call_abc123", tb.ID)
	}

	input := tb.Input
	if input["file_path"] != "/etc/hosts" {
		t.Errorf("tool input file_path = %v, want /etc/hosts", input["file_path"])
	}
}

func TestIntegration_NonStreaming_ReasoningThenText(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:      "chatcmpl-reason",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: time.Now().Unix(),
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:             "assistant",
					Content:          "The answer is 42.",
					ReasoningContent: "Let me think about this step by step...",
				},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     15,
			CompletionTokens: 25,
		},
	}

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "What is the answer?"}}},
		},
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)

	if len(claudeResp.Content) != 2 {
		t.Fatalf("Content blocks = %d, want 2", len(claudeResp.Content))
	}
	if claudeResp.Content[0].Type != "thinking" {
		t.Errorf("Content[0].Type = %s, want thinking", claudeResp.Content[0].Type)
	}
	if claudeResp.Content[0].Thinking != "Let me think about this step by step..." {
		t.Errorf("Content[0].Thinking = %s, wrong", claudeResp.Content[0].Thinking)
	}
	if claudeResp.Content[1].Type != "text" {
		t.Errorf("Content[1].Type = %s, want text", claudeResp.Content[1].Type)
	}
	if claudeResp.Content[1].Text != "The answer is 42." {
		t.Errorf("Content[1].Text = %s, want 'The answer is 42.'", claudeResp.Content[1].Text)
	}
}

func TestIntegration_NonStreaming_MultipleToolCalls(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:      "chatcmpl-multi",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: time.Now().Unix(),
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:    "assistant",
					Content: nil,
					ToolCalls: []models.OpenAIToolCall{
						{
							Index: 0,
							ID:    "call_read_1",
							Type:  "function",
							Function: models.OpenAIFunctionCall{
								Name:      "Read",
								Arguments: `{"file_path":"/a.txt"}`,
							},
						},
						{
							Index: 1,
							ID:    "call_write_1",
							Type:  "function",
							Function: models.OpenAIFunctionCall{
								Name:      "Write",
								Arguments: `{"file_path":"/b.txt","content":"hello"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     80,
			CompletionTokens: 40,
		},
	}

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read a and write b"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Write", Description: "Write a file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)

	if claudeResp.StopReason != "tool_use" {
		t.Errorf("StopReason = %s, want tool_use", claudeResp.StopReason)
	}

	toolBlocks := []models.ClaudeContentBlock{}
	for _, b := range claudeResp.Content {
		if b.Type == "tool_use" {
			toolBlocks = append(toolBlocks, b)
		}
	}
	if len(toolBlocks) != 2 {
		t.Fatalf("tool_use blocks = %d, want 2", len(toolBlocks))
	}

	if toolBlocks[0].Name != "Read" {
		t.Errorf("first tool = %s, want Read", toolBlocks[0].Name)
	}
	if toolBlocks[1].Name != "Write" {
		t.Errorf("second tool = %s, want Write", toolBlocks[1].Name)
	}

	input0 := toolBlocks[0].Input
	if input0["file_path"] != "/a.txt" {
		t.Errorf("Read input file_path = %v, want /a.txt", input0["file_path"])
	}

	input1 := toolBlocks[1].Input
	if input1["file_path"] != "/b.txt" {
		t.Errorf("Write input file_path = %v, want /b.txt", input1["file_path"])
	}
}

func TestIntegration_NonStreaming_CachedTokens(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:      "chatcmpl-cache",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: time.Now().Unix(),
		Choices: []models.OpenAIChoice{
			{
				Index:        0,
				Message:      models.OpenAIMessage{Role: "assistant", Content: "OK"},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     100,
			CompletionTokens: 5,
			PromptTokensDetails: &models.PromptTokensDetails{
				CachedTokens: 80,
			},
		},
	}

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "test"}}},
		},
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)

	if claudeResp.Usage.CacheReadInputTokens != 80 {
		t.Errorf("CacheReadInputTokens = %d, want 80", claudeResp.Usage.CacheReadInputTokens)
	}
}

// ============================================================================
// Streaming SSE Protocol Compliance Tests
// ============================================================================

func TestIntegration_Streaming_TextProtocolCompliance(t *testing.T) {
	sse := openAIChunk("chatcmpl-1", "gpt-4o", map[string]interface{}{
		"role":    "assistant",
		"content": "Hello",
	}, "") +
		openAIChunk("chatcmpl-1", "gpt-4o", map[string]interface{}{
			"content": " world",
		}, "") +
		openAIUsageChunk("chatcmpl-1", "gpt-4o", 10, 5, "stop") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Hi"}}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Verify protocol event sequence
	types := getEventTypes(events)

	// First event: message_start
	if types[0] != "message_start" {
		t.Errorf("first event = %s, want message_start", types[0])
	}

	// Second event: ping
	if types[1] != "ping" {
		t.Errorf("second event = %s, want ping", types[1])
	}

	// Last event: message_stop
	if types[len(types)-1] != "message_stop" {
		t.Errorf("last event = %s, want message_stop", types[len(types)-1])
	}

	// Verify message_start fields
	msgStart := findEventsByType(events, "message_start")[0]
	msg, ok := msgStart.Data["message"].(map[string]interface{})
	if !ok {
		t.Fatal("message_start missing 'message' field")
	}
	for _, field := range []string{"id", "type", "role", "model", "content", "stop_reason", "usage"} {
		if _, exists := msg[field]; !exists {
			t.Errorf("message_start.message missing field: %s", field)
		}
	}
	if msg["type"] != "message" {
		t.Errorf("message.type = %v, want 'message'", msg["type"])
	}
	if msg["role"] != "assistant" {
		t.Errorf("message.role = %v, want 'assistant'", msg["role"])
	}

	// Verify content_block_start/stop paired
	cbsCount := len(findEventsByType(events, "content_block_start"))
	cbeCount := len(findEventsByType(events, "content_block_stop"))
	if cbsCount != cbeCount {
		t.Errorf("content_block_start (%d) != content_block_stop (%d)", cbsCount, cbeCount)
	}

	// Verify text content
	deltaEvents := findEventsByType(events, "content_block_delta")
	var fullText string
	for _, ev := range deltaEvents {
		delta, _ := ev.Data["delta"].(map[string]interface{})
		if delta["type"] == "text_delta" {
			fullText += delta["text"].(string)
		}
	}
	if fullText != "Hello world" {
		t.Errorf("accumulated text = %q, want %q", fullText, "Hello world")
	}

	// Verify stop_reason in message_delta
	msgDelta := findEventsByType(events, "message_delta")[0]
	delta, _ := msgDelta.Data["delta"].(map[string]interface{})
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", delta["stop_reason"])
	}

	// Verify usage in message_delta
	usage, _ := msgDelta.Data["usage"].(map[string]interface{})
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("input_tokens = %v, want 10", usage["input_tokens"])
	}

	// Verify no duplicate message_start/stop
	if len(findEventsByType(events, "message_start")) != 1 {
		t.Error("duplicate message_start events")
	}
	if len(findEventsByType(events, "message_stop")) != 1 {
		t.Error("duplicate message_stop events")
	}

	// Verify StreamingResult
	if result.Content != "Hello world" {
		t.Errorf("result.Content = %q, want %q", result.Content, "Hello world")
	}
	if result.StopReason != "end_turn" {
		t.Errorf("result.StopReason = %s, want end_turn", result.StopReason)
	}
}

func TestIntegration_Streaming_ToolCallProtocolCompliance(t *testing.T) {
	// Simulates how providers send tool calls: name/ID first, then arguments in chunks
	sse := openAIChunk("chatcmpl-tool", "gpt-4o", map[string]interface{}{
		"role": "assistant",
		"tool_calls": []interface{}{
			map[string]interface{}{
				"index": 0,
				"id":    "call_read_001",
				"type":  "function",
				"function": map[string]interface{}{
					"name":      "Read",
					"arguments": "",
				},
			},
		},
	}, "") +
		openAIChunk("chatcmpl-tool", "gpt-4o", map[string]interface{}{
			"tool_calls": []interface{}{
				map[string]interface{}{
					"index": 0,
					"function": map[string]interface{}{
						"arguments": `{"file`,
					},
				},
			},
		}, "") +
		openAIChunk("chatcmpl-tool", "gpt-4o", map[string]interface{}{
			"tool_calls": []interface{}{
				map[string]interface{}{
					"index": 0,
					"function": map[string]interface{}{
						"arguments": `_path":"/etc/hosts"}`,
					},
				},
			},
		}, "") +
		openAIUsageChunk("chatcmpl-tool", "gpt-4o", 50, 20, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read hosts file"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Verify tool_use block in content_block_start
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) != 1 {
		t.Fatalf("content_block_start count = %d, want 1", len(cbsEvents))
	}

	cb, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb["type"] != "tool_use" {
		t.Errorf("content_block type = %v, want tool_use", cb["type"])
	}
	if cb["name"] != "Read" {
		t.Errorf("tool name = %v, want Read", cb["name"])
	}
	toolID, _ := cb["id"].(string)
	if toolID == "" {
		t.Error("tool id is empty")
	}

	// Verify partial JSON deltas accumulate correctly
	deltaEvents := findEventsByType(events, "content_block_delta")
	var fullArgs string
	for _, ev := range deltaEvents {
		delta, _ := ev.Data["delta"].(map[string]interface{})
		if delta["type"] == "input_json_delta" {
			fullArgs += delta["partial_json"].(string)
		}
	}
	expectedArgs := `{"file_path":"/etc/hosts"}`
	if fullArgs != expectedArgs {
		t.Errorf("accumulated args = %q, want %q", fullArgs, expectedArgs)
	}

	// Verify stop_reason = tool_use
	msgDelta := findEventsByType(events, "message_delta")[0]
	delta, _ := msgDelta.Data["delta"].(map[string]interface{})
	if delta["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", delta["stop_reason"])
	}

	// Verify StreamingResult
	if result.StopReason != "tool_use" {
		t.Errorf("result.StopReason = %s, want tool_use", result.StopReason)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("result.ToolCalls = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0]["name"] != "Read" {
		t.Errorf("result.ToolCalls[0].name = %v, want Read", result.ToolCalls[0]["name"])
	}
	input, _ := result.ToolCalls[0]["input"].(map[string]interface{})
	if input["file_path"] != "/etc/hosts" {
		t.Errorf("result.ToolCalls[0].input.file_path = %v, want /etc/hosts", input["file_path"])
	}
}

func TestIntegration_Streaming_ParallelToolCalls(t *testing.T) {
	// Two tool calls: Read and Write in the same response
	sse := openAIChunk("chatcmpl-par", "gpt-4o", map[string]interface{}{
		"role": "assistant",
		"tool_calls": []interface{}{
			map[string]interface{}{
				"index": 0,
				"id":    "call_r1",
				"type":  "function",
				"function": map[string]interface{}{
					"name":      "Read",
					"arguments": `{"file_path":"/a.txt"}`,
				},
			},
		},
	}, "") +
		openAIChunk("chatcmpl-par", "gpt-4o", map[string]interface{}{
			"tool_calls": []interface{}{
				map[string]interface{}{
					"index": 1,
					"id":    "call_w1",
					"type":  "function",
					"function": map[string]interface{}{
						"name":      "Write",
						"arguments": `{"file_path":"/b.txt","content":"hello"}`,
					},
				},
			},
		}, "") +
		openAIUsageChunk("chatcmpl-par", "gpt-4o", 80, 40, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read a and write b"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Write", Description: "Write", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Should have 2 tool_use blocks
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) != 2 {
		t.Fatalf("content_block_start count = %d, want 2", len(cbsEvents))
	}

	// Verify block indices sequential
	idx0, _ := cbsEvents[0].Data["index"].(float64)
	idx1, _ := cbsEvents[1].Data["index"].(float64)
	if int(idx0) != 0 || int(idx1) != 1 {
		t.Errorf("block indices = [%v, %v], want [0, 1]", idx0, idx1)
	}

	// Verify first tool: Read
	cb0, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb0["name"] != "Read" {
		t.Errorf("first tool name = %v, want Read", cb0["name"])
	}

	// Verify second tool: Write
	cb1, _ := cbsEvents[1].Data["content_block"].(map[string]interface{})
	if cb1["name"] != "Write" {
		t.Errorf("second tool name = %v, want Write", cb1["name"])
	}

	// Verify content_block_stop paired
	cbeEvents := findEventsByType(events, "content_block_stop")
	if len(cbeEvents) != 2 {
		t.Errorf("content_block_stop count = %d, want 2", len(cbeEvents))
	}

	// Verify result
	if len(result.ToolCalls) != 2 {
		t.Fatalf("result.ToolCalls = %d, want 2", len(result.ToolCalls))
	}
	if result.ToolCalls[0]["name"] != "Read" {
		t.Errorf("first tool = %v, want Read", result.ToolCalls[0]["name"])
	}
	if result.ToolCalls[1]["name"] != "Write" {
		t.Errorf("second tool = %v, want Write", result.ToolCalls[1]["name"])
	}
}

func TestIntegration_Streaming_ThinkingTextToolSequence(t *testing.T) {
	// thinking → text → tool_use (common extended thinking pattern)
	sse := openAIChunk("chatcmpl-seq", "gpt-4o", map[string]interface{}{
		"role":              "assistant",
		"reasoning_content": "I need to read the file first...",
	}, "") +
		openAIChunk("chatcmpl-seq", "gpt-4o", map[string]interface{}{
			"content": "Let me read that file.",
		}, "") +
		openAIChunk("chatcmpl-seq", "gpt-4o", map[string]interface{}{
			"tool_calls": []interface{}{
				map[string]interface{}{
					"index": 0,
					"id":    "call_t1",
					"type":  "function",
					"function": map[string]interface{}{
						"name":      "Read",
						"arguments": `{"file_path":"/test.txt"}`,
					},
				},
			},
		}, "") +
		openAIUsageChunk("chatcmpl-seq", "gpt-4o", 60, 35, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read test.txt"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Should have 3 content blocks: thinking(0), text(1), tool_use(2)
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) != 3 {
		t.Fatalf("content_block_start count = %d, want 3", len(cbsEvents))
	}

	cb0, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	cb1, _ := cbsEvents[1].Data["content_block"].(map[string]interface{})
	cb2, _ := cbsEvents[2].Data["content_block"].(map[string]interface{})

	if cb0["type"] != "thinking" {
		t.Errorf("block[0] type = %v, want thinking", cb0["type"])
	}
	if cb1["type"] != "text" {
		t.Errorf("block[1] type = %v, want text", cb1["type"])
	}
	if cb2["type"] != "tool_use" {
		t.Errorf("block[2] type = %v, want tool_use", cb2["type"])
	}
	if cb2["name"] != "Read" {
		t.Errorf("block[2] name = %v, want Read", cb2["name"])
	}

	// Verify block indices
	for i, ev := range cbsEvents {
		idx, _ := ev.Data["index"].(float64)
		if int(idx) != i {
			t.Errorf("block[%d] index = %v, want %d", i, idx, i)
		}
	}

	// Verify thinking delta content
	thinkingDeltas := findEventsByType(events, "content_block_delta")
	var thinkingContent string
	for _, ev := range thinkingDeltas {
		delta, _ := ev.Data["delta"].(map[string]interface{})
		if delta["type"] == "thinking_delta" {
			thinkingContent += delta["thinking"].(string)
		}
	}
	if thinkingContent != "I need to read the file first..." {
		t.Errorf("thinking content = %q, wrong", thinkingContent)
	}

	// Verify result
	if result.StopReason != "tool_use" {
		t.Errorf("result.StopReason = %s, want tool_use", result.StopReason)
	}
}

// ============================================================================
// Claude Code CLI Real-World Pattern Tests
// ============================================================================

func TestIntegration_ClaudeCode_ReadFileWithArgs(t *testing.T) {
	// Simulates provider sending tool name + args in same chunk (e.g., Gemini, xAI)
	sse := openAIToolCallChunk("chatcmpl-rf", "gpt-4o", 0, "toolu_01ABC", "Read", `{"file_path":"/tmp/test.txt"}`) +
		openAIUsageChunk("chatcmpl-rf", "gpt-4o", 100, 30, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		System:    "You are Claude Code, an AI assistant.",
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read /tmp/test.txt"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Write", Description: "Write", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Bash", Description: "Bash", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Verify tool_use block
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) < 1 {
		t.Fatal("no content_block_start events")
	}
	cb, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb["type"] != "tool_use" {
		t.Errorf("type = %v, want tool_use", cb["type"])
	}
	if cb["name"] != "Read" {
		t.Errorf("name = %v, want Read", cb["name"])
	}

	// Verify result tool call has correct input
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc["name"] != "Read" {
		t.Errorf("name = %v, want Read", tc["name"])
	}
	input, _ := tc["input"].(map[string]interface{})
	if input["file_path"] != "/tmp/test.txt" {
		t.Errorf("file_path = %v, want /tmp/test.txt", input["file_path"])
	}
}

func TestIntegration_ClaudeCode_EditTool(t *testing.T) {
	// Claude Code Edit tool with complex input
	sse := openAIToolCallChunk("chatcmpl-edit", "gpt-4o", 0, "call_edit_1", "Edit", `{"file_path":"/main.go","old_string":"fmt.Println","new_string":"log.Println"}`) +
		openAIUsageChunk("chatcmpl-edit", "gpt-4o", 200, 50, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Replace Println"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Edit", Description: "Edit", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	input, _ := tc["input"].(map[string]interface{})
	if input["file_path"] != "/main.go" {
		t.Errorf("file_path = %v, want /main.go", input["file_path"])
	}
	if input["old_string"] != "fmt.Println" {
		t.Errorf("old_string = %v", input["old_string"])
	}
	if input["new_string"] != "log.Println" {
		t.Errorf("new_string = %v", input["new_string"])
	}
}

func TestIntegration_ClaudeCode_BashWithTimeout(t *testing.T) {
	// Bash tool with timeout parameter
	sse := openAIToolCallChunk("chatcmpl-bash", "gpt-4o", 0, "call_bash_1", "Bash", `{"command":"go test ./...","timeout":120000}`) +
		openAIUsageChunk("chatcmpl-bash", "gpt-4o", 150, 30, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Run tests"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Bash", Description: "Bash", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	tc := result.ToolCalls[0]
	input, _ := tc["input"].(map[string]interface{})
	if input["command"] != "go test ./..." {
		t.Errorf("command = %v", input["command"])
	}
	if input["timeout"].(float64) != 120000 {
		t.Errorf("timeout = %v, want 120000", input["timeout"])
	}
}

func TestIntegration_ClaudeCode_ToolNameRestoration(t *testing.T) {
	longName := "mcp_server_very_long_tool_name_that_exceeds_the_openai_limit_of_64_characters_for_function_names"
	truncated := truncateToolName(longName)

	if len(truncated) > 64 {
		t.Errorf("truncated name length = %d, want <= 64", len(truncated))
	}
	if truncated == longName {
		t.Error("name was not truncated")
	}

	mapping := map[string]string{truncated: longName}

	sse := fmt.Sprintf("data: {\"id\":\"chatcmpl-long\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"call_long\",\"type\":\"function\",\"function\":{\"name\":\"%s\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n", truncated) +
		openAIUsageChunk("chatcmpl-long", "gpt-4o", 10, 5, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "test"}}},
		},
	}

	result := ConvertOpenAIStreamingToClaudeWithMapping(c, strings.NewReader(sse), req, context.Background(), mapping)
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) < 1 {
		t.Fatal("no content_block_start events")
	}
	cb, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	restoredName := cb["name"].(string)

	if restoredName != longName {
		t.Errorf("restored name = %q, want %q", restoredName, longName)
	}

	if len(result.ToolCalls) > 0 && result.ToolCalls[0]["name"] != longName {
		t.Errorf("result tool name = %v, want %v", result.ToolCalls[0]["name"], longName)
	}
}

func TestIntegration_ClaudeCode_CachedTokensInStreaming(t *testing.T) {
	sse := openAIChunk("chatcmpl-cache", "gpt-4o", map[string]interface{}{
		"role":    "assistant",
		"content": "cached response",
	}, "")

	// Usage chunk with cached tokens
	usageChunk := `{"id":"chatcmpl-cache","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":80}}}`
	sse += "data: " + usageChunk + "\n\n" +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "test"}}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	// Check cache tokens in usage
	msgStartEvents := findEventsByType(parseClaudeSSEEvents(w.Body.String()), "message_start")
	if len(msgStartEvents) > 0 {
		msg, _ := msgStartEvents[0].Data["message"].(map[string]interface{})
		usage, _ := msg["usage"].(map[string]interface{})
		// Initial usage should have cache fields (even if 0)
		if _, ok := usage["cache_creation_input_tokens"]; !ok {
			t.Error("initial usage missing cache_creation_input_tokens field")
		}
		if _, ok := usage["cache_read_input_tokens"]; !ok {
			t.Error("initial usage missing cache_read_input_tokens field")
		}
	}

	// Check cache tokens in result
	if result.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", result.InputTokens)
	}
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestIntegration_Edge_EmptyToolCallArgs(t *testing.T) {
	// Tool call with empty args (some providers do this)
	sse := openAIToolCallChunk("chatcmpl-empty", "gpt-4o", 0, "call_empty", "ListFiles", "") +
		openAIUsageChunk("chatcmpl-empty", "gpt-4o", 10, 5, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "List files"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "ListFiles", Description: "List", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc["name"] != "ListFiles" {
		t.Errorf("name = %v, want ListFiles", tc["name"])
	}
	// Empty args should still produce a valid input map
	input, _ := tc["input"].(map[string]interface{})
	// Should be empty map, not nil
	if len(input) != 0 {
		t.Errorf("input = %v, want empty map", input)
	}
}

func TestIntegration_Edge_ToolCallNoIDFirst(t *testing.T) {
	// Provider sends tool call without ID in first chunk, ID in second chunk
	sse := openAIChunk("chatcmpl-noid", "gpt-4o", map[string]interface{}{
		"role": "assistant",
		"tool_calls": []interface{}{
			map[string]interface{}{
				"index": 0,
				"type":  "function",
				"function": map[string]interface{}{
					"name":      "Read",
					"arguments": "",
				},
			},
		},
	}, "") +
		openAIChunk("chatcmpl-noid", "gpt-4o", map[string]interface{}{
			"tool_calls": []interface{}{
				map[string]interface{}{
					"index": 0,
					"id":    "call_delayed_id",
					"function": map[string]interface{}{
						"arguments": `{"file_path":"/test"}`,
					},
				},
			},
		}, "") +
		openAIUsageChunk("chatcmpl-noid", "gpt-4o", 20, 10, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) < 1 {
		t.Fatal("no content_block_start")
	}
	cb, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb["name"] != "Read" {
		t.Errorf("name = %v, want Read", cb["name"])
	}
	// Should have a generated ID even though first chunk had none
	toolID, _ := cb["id"].(string)
	if toolID == "" {
		t.Error("tool id should not be empty even when ID arrives late")
	}

	// Result should have the delayed ID
	if len(result.ToolCalls) > 0 {
		if result.ToolCalls[0]["id"] == "" {
			t.Error("result tool call id is empty")
		}
	}
}

func TestIntegration_Edge_ToolCallWithMalformedJSON(t *testing.T) {
	// Tool call with malformed JSON arguments
	sse := openAIToolCallChunk("chatcmpl-mal", "gpt-4o", 0, "call_mal", "Read", `{invalid json`) +
		openAIUsageChunk("chatcmpl-mal", "gpt-4o", 15, 8, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	// Should not crash, should still produce tool call
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc["name"] != "Read" {
		t.Errorf("name = %v, want Read", tc["name"])
	}
	// Malformed JSON should still be handled gracefully
	input, _ := tc["input"].(map[string]interface{})
	// Should have raw_arguments fallback or empty map
	_ = input
}

func TestIntegration_Edge_ThreeParallelToolCalls(t *testing.T) {
	// Three tool calls in sequence
	sse := openAIToolCallChunk("chatcmpl-3t", "gpt-4o", 0, "call_a", "Read", `{"file_path":"/a"}`) +
		openAIToolCallChunk("chatcmpl-3t", "gpt-4o", 1, "call_b", "Write", `{"file_path":"/b","content":"x"}`) +
		openAIToolCallChunk("chatcmpl-3t", "gpt-4o", 2, "call_c", "Bash", `{"command":"ls"}`) +
		openAIUsageChunk("chatcmpl-3t", "gpt-4o", 100, 60, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Do everything"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Write", Description: "Write", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Bash", Description: "Bash", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Should have 3 content blocks
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) != 3 {
		t.Fatalf("content_block_start count = %d, want 3", len(cbsEvents))
	}

	expectedNames := []string{"Read", "Write", "Bash"}
	for i, expected := range expectedNames {
		cb, _ := cbsEvents[i].Data["content_block"].(map[string]interface{})
		name, _ := cb["name"].(string)
		if name != expected {
			t.Errorf("block[%d] name = %q, want %q", i, name, expected)
		}
		idx, _ := cbsEvents[i].Data["index"].(float64)
		if int(idx) != i {
			t.Errorf("block[%d] index = %v, want %d", i, idx, i)
		}
	}

	// Verify result
	if len(result.ToolCalls) != 3 {
		t.Fatalf("result.ToolCalls = %d, want 3", len(result.ToolCalls))
	}
	for i, expected := range expectedNames {
		if result.ToolCalls[i]["name"] != expected {
			t.Errorf("result.ToolCalls[%d].name = %v, want %v", i, result.ToolCalls[i]["name"], expected)
		}
	}
}

func TestIntegration_Edge_TextThenToolThenMoreText(t *testing.T) {
	// Text → tool_use (model sends text first, then tool call)
	sse := openAIChunk("chatcmpl-tt", "gpt-4o", map[string]interface{}{
		"role":    "assistant",
		"content": "I'll read the file for you. ",
	}, "") +
		openAIChunk("chatcmpl-tt", "gpt-4o", map[string]interface{}{
			"content": nil,
			"tool_calls": []interface{}{
				map[string]interface{}{
					"index": 0,
					"id":    "call_read_1",
					"type":  "function",
					"function": map[string]interface{}{
						"name":      "Read",
						"arguments": `{"file_path":"/test"}`,
					},
				},
			},
		}, "") +
		openAIUsageChunk("chatcmpl-tt", "gpt-4o", 30, 20, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read /test"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events := parseClaudeSSEEvents(w.Body.String())

	// Should have 2 content blocks: text(0), tool_use(1)
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) != 2 {
		t.Fatalf("content_block_start count = %d, want 2", len(cbsEvents))
	}

	cb0, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	cb1, _ := cbsEvents[1].Data["content_block"].(map[string]interface{})

	if cb0["type"] != "text" {
		t.Errorf("block[0] type = %v, want text", cb0["type"])
	}
	if cb1["type"] != "tool_use" {
		t.Errorf("block[1] type = %v, want tool_use", cb1["type"])
	}
	if cb1["name"] != "Read" {
		t.Errorf("block[1] name = %v, want Read", cb1["name"])
	}

	// Verify text content was accumulated
	if !strings.Contains(result.Content, "I'll read the file for you") {
		t.Errorf("result.Content = %q, should contain intro text", result.Content)
	}
}

func TestIntegration_Edge_SSEFormatWithComments(t *testing.T) {
	// Provider sends SSE comments (lines starting with :)
	sse := ": this is a comment\n\n" +
		openAIChunk("chatcmpl-cmt", "gpt-4o", map[string]interface{}{
			"role":    "assistant",
			"content": "Hello",
		}, "") +
		": another comment\n" +
		openAIUsageChunk("chatcmpl-cmt", "gpt-4o", 5, 2, "stop") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Hi"}}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	if result.Content != "Hello" {
		t.Errorf("Content = %q, want 'Hello'", result.Content)
	}
}

func TestIntegration_Edge_NilContentField(t *testing.T) {
	// Provider sends content: null
	sse := openAIChunk("chatcmpl-nil", "gpt-4o", map[string]interface{}{
		"role":    "assistant",
		"content": nil,
	}, "") +
		openAIChunk("chatcmpl-nil", "gpt-4o", map[string]interface{}{
			"content": "text after null",
		}, "") +
		openAIUsageChunk("chatcmpl-nil", "gpt-4o", 5, 5, "stop") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Hi"}}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	if result.Content != "text after null" {
		t.Errorf("Content = %q, want 'text after null'", result.Content)
	}
}

// ============================================================================
// Full Protocol Sequence Verification
// ============================================================================

func TestIntegration_FullProtocolSequence_Text(t *testing.T) {
	// Verify exact event sequence for a text-only response
	sse := openAIChunk("chatcmpl-full", "gpt-4o", map[string]interface{}{
		"role":    "assistant",
		"content": "Hi",
	}, "") +
		openAIUsageChunk("chatcmpl-full", "gpt-4o", 10, 3, "stop") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Hello"}}},
		},
	}

	ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())

	events := parseClaudeSSEEvents(w.Body.String())
	types := getEventTypes(events)

	// Expected sequence:
	// 0: message_start
	// 1: ping
	// 2: content_block_start (text, index 0)
	// 3: content_block_delta (text_delta, "Hi")
	// 4: content_block_stop (index 0)
	// 5: message_delta (stop_reason: end_turn)
	// 6: message_stop
	expected := []string{
		"message_start",
		"ping",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}

	if len(types) != len(expected) {
		t.Fatalf("event count = %d, want %d\ngot: %v\nwant: %v", len(types), len(expected), types, expected)
	}

	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d] = %s, want %s", i, types[i], want)
		}
	}
}

func TestIntegration_FullProtocolSequence_ToolCall(t *testing.T) {
	// Verify exact event sequence for a tool call response
	sse := openAIToolCallChunk("chatcmpl-ft", "gpt-4o", 0, "call_ft1", "Read", `{"file_path":"/x"}`) +
		openAIUsageChunk("chatcmpl-ft", "gpt-4o", 30, 15, "tool_calls") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())

	events := parseClaudeSSEEvents(w.Body.String())
	types := getEventTypes(events)

	// Expected sequence:
	// 0: message_start
	// 1: ping
	// 2: content_block_start (tool_use, index 0)
	// 3: content_block_delta (input_json_delta)
	// 4: content_block_stop (index 0)
	// 5: message_delta (stop_reason: tool_use)
	// 6: message_stop
	expected := []string{
		"message_start",
		"ping",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}

	if len(types) != len(expected) {
		t.Fatalf("event count = %d, want %d\ngot: %v\nwant: %v", len(types), len(expected), types, expected)
	}

	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d] = %s, want %s", i, types[i], want)
		}
	}

	// Verify tool_use block data
	cbsEvent := findEventsByType(events, "content_block_start")[0]
	cb, _ := cbsEvent.Data["content_block"].(map[string]interface{})
	if cb["type"] != "tool_use" {
		t.Errorf("content_block type = %v, want tool_use", cb["type"])
	}
	if cb["name"] != "Read" {
		t.Errorf("name = %v, want Read", cb["name"])
	}
	if _, ok := cb["id"]; !ok {
		t.Error("tool_use block missing id")
	}
	if _, ok := cb["input"]; !ok {
		t.Error("tool_use block missing input")
	}
}

func TestIntegration_FullProtocolSequence_ThinkingText(t *testing.T) {
	// Verify exact event sequence for thinking + text response
	sse := openAIChunk("chatcmpl-tt", "gpt-4o", map[string]interface{}{
		"role":              "assistant",
		"reasoning_content": "thinking...",
	}, "") +
		openAIChunk("chatcmpl-tt", "gpt-4o", map[string]interface{}{
			"content": "answer",
		}, "") +
		openAIUsageChunk("chatcmpl-tt", "gpt-4o", 20, 10, "stop") +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Think"}}},
		},
	}

	ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())

	events := parseClaudeSSEEvents(w.Body.String())
	types := getEventTypes(events)

	// Expected: message_start, ping, thinking block, text block, message_delta, message_stop
	expected := []string{
		"message_start",
		"ping",
		"content_block_start", // thinking, index 0
		"content_block_delta", // thinking_delta
		"content_block_stop",  // index 0
		"content_block_start", // text, index 1
		"content_block_delta", // text_delta
		"content_block_stop",  // index 1
		"message_delta",
		"message_stop",
	}

	if len(types) != len(expected) {
		t.Fatalf("event count = %d, want %d\ngot: %v\nwant: %v", len(types), len(expected), types, expected)
	}

	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d] = %s, want %s", i, types[i], want)
		}
	}

	// Verify block types
	cbsEvents := findEventsByType(events, "content_block_start")
	cb0, _ := cbsEvents[0].Data["content_block"].(map[string]interface{})
	cb1, _ := cbsEvents[1].Data["content_block"].(map[string]interface{})

	if cb0["type"] != "thinking" {
		t.Errorf("block[0] = %v, want thinking", cb0["type"])
	}
	if cb1["type"] != "text" {
		t.Errorf("block[1] = %v, want text", cb1["type"])
	}
}

// ============================================================================
// Request Conversion Integration Tests
// ============================================================================

func TestIntegration_RequestConversion_ToolResult(t *testing.T) {
	// Verify tool_result message is correctly converted to OpenAI format
	claudeReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Messages: []models.ClaudeMessage{
			{
				Role: "user",
				Content: []models.ClaudeContentBlock{
					{Type: "text", Text: "Read the file"},
				},
			},
			{
				Role: "assistant",
				Content: []models.ClaudeContentBlock{
					{
						Type:  "tool_use",
						ID:    "toolu_01ABC",
						Name:  "Read",
						Input: map[string]interface{}{"file_path": "/test.txt"},
					},
				},
			},
			{
				Role: "user",
				Content: []models.ClaudeContentBlock{
					{
						Type:      "tool_result",
						ToolUseID: "toolu_01ABC",
						Text:      "file contents here",
					},
				},
			},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	// Convert via the factory
	reqBody, err := json.Marshal(claudeReq)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	factory := GlobalFactory
	openAIBody, _, err := factory.ConvertClaudeToOpenAI(reqBody, nil)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify the OpenAI request has correct message structure
	if len(openAIReq.Messages) < 3 {
		t.Fatalf("messages count = %d, want >= 3", len(openAIReq.Messages))
	}

	// First message: user text
	if openAIReq.Messages[0].Role != "user" {
		t.Errorf("msg[0].role = %s, want user", openAIReq.Messages[0].Role)
	}

	// Second message: assistant with tool call
	if openAIReq.Messages[1].Role != "assistant" {
		t.Errorf("msg[1].role = %s, want assistant", openAIReq.Messages[1].Role)
	}
	if len(openAIReq.Messages[1].ToolCalls) != 1 {
		t.Errorf("msg[1].tool_calls count = %d, want 1", len(openAIReq.Messages[1].ToolCalls))
	}

	// Third message: tool result
	if openAIReq.Messages[2].Role != "tool" {
		t.Errorf("msg[2].role = %s, want tool", openAIReq.Messages[2].Role)
	}
	if openAIReq.Messages[2].ToolCallID != "toolu_01ABC" {
		t.Errorf("msg[2].tool_call_id = %s, want toolu_01ABC", openAIReq.Messages[2].ToolCallID)
	}
}


func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
