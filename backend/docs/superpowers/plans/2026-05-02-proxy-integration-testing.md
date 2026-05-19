# Proxy Integration Testing Plan — HTTP-Level Protocol Verification

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 通过直接向代理服务器发送 Claude 格式的 HTTP 请求，验证完整的请求/响应转换链路正确工作，覆盖 Claude Code CLI 实际使用的所有通信模式。

**Architecture:** 测试脚本 → HTTP POST to proxy /v1/messages (Anthropic format) → handler 解析 → converter 转换为 OpenAI → 发送到上游 API → converter 转回 Claude SSE → 验证 SSE 事件序列。测试分两层：(1) 不依赖真实 API key 的 handler 级测试（mock 上游响应），(2) 使用真实 API key 的 E2E 测试。

**Tech Stack:** Go 1.22, net/http/httptest, Gin test mode, Anthropic Messages API SSE protocol

**Risks:**
- 真实 API 测试需要有效的 API key → 缓解：测试函数检查 API key，无则 skip
- Handler 依赖数据库 → 缓解：使用 handler 的 test helpers 或直接 HTTP 级测试

---

### Task 1: 创建 HTTP 级别的代理集成测试框架

**Depends on:** None
**Files:**
- Create: `converter/proxy_integration_test.go`

测试框架基础设施，支持发送 Claude 格式请求到 httptest 服务器并解析 SSE 响应。

- [ ] **Step 1: 创建 proxy_integration_test.go 测试文件 — 建立基础设施**

```go
package converter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// --- SSE Event Parsing ---

// SSEEvent represents a parsed Server-Sent Event
type SSEEvent struct {
	Event string
	Data  map[string]interface{}
	Raw   string
}

// ParseSSEStream parses an SSE response body into events
func ParseSSEStream(body io.Reader) ([]SSEEvent, error) {
	var events []SSEEvent
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentEvent string
	var currentData strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			currentData.WriteString(strings.TrimPrefix(line, "data: "))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			currentData.WriteString(strings.TrimPrefix(line, "data:"))
			continue
		}

		// Empty line = event boundary
		if strings.TrimSpace(line) == "" && currentEvent != "" {
			raw := currentData.String()
			var data map[string]interface{}
			_ = json.Unmarshal([]byte(raw), &data)
			events = append(events, SSEEvent{
				Event: currentEvent,
				Data:  data,
				Raw:   raw,
			})
			currentEvent = ""
			currentData.Reset()
		}
	}

	return events, scanner.Err()
}

// FindEvents filters events by type
func FindEvents(events []SSEEvent, eventType string) []SSEEvent {
	var result []SSEEvent
	for _, e := range events {
		if e.Event == eventType {
			result = append(result, e)
		}
	}
	return result
}

// --- Request Building Helpers ---

// BuildClaudeRequest creates a Claude Messages API request body
func BuildClaudeRequest(model string, messages []map[string]interface{}, opts ...func(map[string]interface{})) map[string]interface{} {
	req := map[string]interface{}{
		"model":      model,
		"max_tokens": 4096,
		"stream":     true,
		"messages":   messages,
	}
	for _, opt := range opts {
		opt(req)
	}
	return req
}

// WithSystem adds a system prompt to the request
func WithSystem(system string) func(map[string]interface{}) {
	return func(req map[string]interface{}) {
		req["system"] = system
	}
}

// WithTools adds tools to the request
func WithTools(tools []map[string]interface{}) func(map[string]interface{}) {
	return func(req map[string]interface{}) {
		req["tools"] = tools
	}
}

// ClaudeTextMessage creates a text user message
func ClaudeTextMessage(text string) map[string]interface{} {
	return map[string]interface{}{
		"role": "user",
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	}
}

// ClaudeToolResultMessage creates a tool_result user message
func ClaudeToolResultMessage(toolUseID, content string) map[string]interface{} {
	return map[string]interface{}{
		"role": "user",
		"content": []map[string]interface{}{
			{
				"type":       "tool_result",
				"tool_use_id": toolUseID,
				"content":    content,
			},
		},
	}
}

// ClaudeAssistantToolUse creates an assistant message with tool_use
func ClaudeAssistantToolUse(toolID, toolName, input map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolName,
				"input": input,
			},
		},
	}
}

// ReadFileTool returns a Claude tool definition for Read
func ReadFileTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        "Read",
		"description": "Read a file from the filesystem",
		"input_schema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to read",
				},
			},
			"required": []string{"file_path"},
		},
	}
}

// WriteFileTool returns a Claude tool definition for Write
func WriteFileTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        "Write",
		"description": "Write content to a file",
		"input_schema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write",
				},
			},
			"required": []string{"file_path", "content"},
		},
	}
}

// BashTool returns a Claude tool definition for Bash
func BashTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        "Bash",
		"description": "Execute a bash command",
		"input_schema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The command to execute",
				},
			},
			"required": []string{"command"},
		},
	}
}

// --- Mock HTTP Server ---

// MockUpstreamServer creates a test server that returns OpenAI-format streaming responses
func MockUpstreamServer(responses []string) *httptest.Server {
	gin.SetMode(gin.TestMode)
	mux := http.NewServeMux()
	responseIdx := 0

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if responseIdx >= len(responses) {
			w.WriteHeader(500)
			return
		}
		resp := responses[responseIdx]
		responseIdx++

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		w.Write([]byte(resp))
	})

	return httptest.NewServer(mux)
}

// --- Claude Protocol Assertion Helpers ---

// AssertEventSequence verifies the SSE event sequence follows Claude protocol rules
func AssertEventSequence(t *testing.T, events []SSEEvent) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("no SSE events received")
	}

	// First event must be message_start
	if events[0].Event != models.EventMessageStart {
		t.Errorf("first event = %s, want %s", events[0].Event, models.EventMessageStart)
	}

	// Second event should be ping
	if len(events) > 1 && events[1].Event != models.EventPing {
		t.Errorf("second event = %s, want %s", events[1].Event, models.EventPing)
	}

	// Last event must be message_stop
	if events[len(events)-1].Event != models.EventMessageStop {
		t.Errorf("last event = %s, want %s", events[len(events)-1].Event, models.EventMessageStop)
	}

	// Verify content_block_start/stop are paired
	cbsCount := len(FindEvents(events, models.EventContentBlockStart))
	cbeCount := len(FindEvents(events, models.EventContentBlockStop))
	if cbsCount != cbeCount {
		t.Errorf("content_block_start count (%d) != content_block_stop count (%d)", cbsCount, cbeCount)
	}

	// Verify indices are sequential
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	for i, ev := range cbsEvents {
		idx, _ := ev.Data["index"].(float64)
		if int(idx) != i {
			t.Errorf("content_block_start[%d].index = %v, want %d", i, idx, i)
		}
	}
}

// AssertMessageStartFields verifies message_start event has required fields
func AssertMessageStartFields(t *testing.T, event SSEEvent) {
	t.Helper()
	if event.Event != models.EventMessageStart {
		t.Fatalf("expected message_start event, got %s", event.Event)
	}

	msg, ok := event.Data["message"].(map[string]interface{})
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
}
```

- [ ] **Step 2: 添加非流式请求转换集成测试 — 验证 Claude→OpenAI 请求体正确性**

```go
// --- Non-Streaming Integration Tests ---

func TestIntegration_NonStreamingConversion_TextOnly(t *testing.T) {
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
		t.Errorf("Content[0].Text = %s, want Hello! How can I help you?", claudeResp.Content[0].Text)
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

func TestIntegration_NonStreamingConversion_ToolCall(t *testing.T) {
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
							Function: models.OpenAIFunction{
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
			{
				Name:        "Read",
				Description: "Read a file",
				InputSchema: map[string]interface{}{"type": "object"},
			},
		},
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)

	if claudeResp.StopReason != "tool_use" {
		t.Errorf("StopReason = %s, want tool_use", claudeResp.StopReason)
	}

	// Should have one tool_use content block
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

	input, ok := tb.Input.(map[string]interface{})
	if !ok {
		t.Fatalf("tool input type = %T, want map[string]interface{}", tb.Input)
	}
	if input["file_path"] != "/etc/hosts" {
		t.Errorf("tool input file_path = %v, want /etc/hosts", input["file_path"])
	}
}

func TestIntegration_NonStreamingConversion_ReasoningThenText(t *testing.T) {
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

	// Should have 2 content blocks: thinking + text
	if len(claudeResp.Content) != 2 {
		t.Fatalf("Content blocks = %d, want 2", len(claudeResp.Content))
	}

	if claudeResp.Content[0].Type != "thinking" {
		t.Errorf("Content[0].Type = %s, want thinking", claudeResp.Content[0].Type)
	}
	if claudeResp.Content[0].Thinking != "Let me think about this step by step..." {
		t.Errorf("Content[0].Thinking wrong")
	}

	if claudeResp.Content[1].Type != "text" {
		t.Errorf("Content[1].Type = %s, want text", claudeResp.Content[1].Type)
	}
	if claudeResp.Content[1].Text != "The answer is 42." {
		t.Errorf("Content[1].Text = %s", claudeResp.Content[1].Text)
	}
}
```

- [ ] **Step 3: 添加流式 SSE 协议合规性测试 — 验证 SSE 事件序列符合 Anthropic 规范**

```go
// --- Streaming SSE Protocol Compliance Tests ---

func TestIntegration_StreamingProtocol_TextOnly(t *testing.T) {
	// Simulate OpenAI streaming response for text-only case
	sse := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\n" +
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

	reader := strings.NewReader(sse)
	result := ConvertOpenAIStreamingToClaude(c, reader, req, context.Background())

	if result == nil {
		t.Fatal("result is nil")
	}

	// Parse SSE output
	events, err := ParseSSEStream(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}

	// Verify protocol compliance
	AssertEventSequence(t, events)

	// Check message_start
	msgStartEvents := FindEvents(events, models.EventMessageStart)
	if len(msgStartEvents) != 1 {
		t.Fatalf("message_start count = %d, want 1", len(msgStartEvents))
	}
	AssertMessageStartFields(t, msgStartEvents[0])

	// Verify content block: should be text
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	if len(cbsEvents) != 1 {
		t.Fatalf("content_block_start count = %d, want 1", len(cbsEvents))
	}
	cb := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb["type"] != "text" {
		t.Errorf("content_block type = %v, want text", cb["type"])
	}

	// Verify content delta
	deltaEvents := FindEvents(events, models.EventContentBlockDelta)
	var fullText string
	for _, ev := range deltaEvents {
		delta := ev.Data["delta"].(map[string]interface{})
		if delta["type"] == "text_delta" {
			fullText += delta["text"].(string)
		}
	}
	if fullText != "Hello world" {
		t.Errorf("accumulated text = %q, want %q", fullText, "Hello world")
	}

	// Verify stop_reason in message_delta
	msgDeltaEvents := FindEvents(events, models.EventMessageDelta)
	if len(msgDeltaEvents) != 1 {
		t.Fatalf("message_delta count = %d, want 1", len(msgDeltaEvents))
	}
	delta := msgDeltaEvents[0].Data["delta"].(map[string]interface{})
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", delta["stop_reason"])
	}

	// Verify usage in message_delta
	usage := msgDeltaEvents[0].Data["usage"].(map[string]interface{})
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("input_tokens = %v, want 10", usage["input_tokens"])
	}
	if usage["output_tokens"].(float64) != 5 {
		t.Errorf("output_tokens = %v, want 5", usage["output_tokens"])
	}

	// Verify message_stop
	msgStopEvents := FindEvents(events, models.EventMessageStop)
	if len(msgStopEvents) != 1 {
		t.Errorf("message_stop count = %d, want 1", len(msgStopEvents))
	}
}

func TestIntegration_StreamingProtocol_ToolCall(t *testing.T) {
	// Simulate OpenAI streaming response for tool call
	sse := "data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"call_read_001\",\"type\":\"function\",\"function\":{\"name\":\"Read\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"file_"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"path\\\":\\\"/etc/hosts\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":20}}\n\n" +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read the hosts file"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	reader := Strings.NewReader(sse)
	result := ConvertOpenAIStreamingToClaude(c, reader, req, context.Background())

	if result == nil {
		t.Fatal("result is nil")
	}

	events, err := ParseSSEStream(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}

	AssertEventSequence(t, events)

	// Verify tool_use block
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	if len(cbsEvents) != 1 {
		t.Fatalf("content_block_start count = %d, want 1", len(cbsEvents))
	}
	cb := cbsEvents[0].Data["content_block"].(map[string]interface{})
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

	// Verify partial JSON deltas
	deltaEvents := FindEvents(events, models.EventContentBlockDelta)
	var fullArgs string
	for _, ev := range deltaEvents {
		delta := ev.Data["delta"].(map[string]interface{})
		if delta["type"] == "input_json_delta" {
			fullArgs += delta["partial_json"].(string)
		}
	}
	if fullArgs != `{"file_path":"/etc/hosts"}` {
		t.Errorf("accumulated args = %q, want %q", fullArgs, `{"file_path":"/etc/hosts"}`)
	}

	// Verify stop_reason
	msgDeltaEvents := FindEvents(events, models.EventMessageDelta)
	delta := msgDeltaEvents[0].Data["delta"].(map[string]interface{})
	if delta["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", delta["stop_reason"])
	}

	// Verify StreamingResult
	if result.StopReason != "tool_use" {
		t.Errorf("result.StopReason = %s, want tool_use", result.StopReason)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("result.ToolCalls count = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0]["name"] != "Read" {
		t.Errorf("result.ToolCalls[0].name = %v, want Read", result.ToolCalls[0]["name"])
	}
}
```

- [ ] **Step 4: 添加 Claude Code CLI 实际通信模式测试**

```go
// --- Claude Code CLI Real-World Pattern Tests ---

func TestIntegration_ClaudeCode_ReadFileWorkflow(t *testing.T) {
	// Simulate what Claude Code actually sends: user asks to read a file,
	// model responds with Read tool call, then tool_result comes back,
	// model responds with text content.

	// Step 1: Model calls Read tool
	sse := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"toolu_01ABC\",\"type\":\"function\",\"function\":{\"name\":\"Read\",\"arguments\":\"{\\\"file_path\\\":\\\"/tmp/test.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":30}}\n\n" +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		System:    "You are Claude Code, an AI assistant...",
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read the file /tmp/test.txt"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Write", Description: "Write a file", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Bash", Description: "Run bash", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events, _ := ParseSSEStream(bytes.NewReader(w.Body.Bytes()))
	AssertEventSequence(t, events)

	// Verify tool_use block has correct structure
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	if len(cbsEvents) < 1 {
		t.Fatal("no content_block_start events")
	}
	cb := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb["type"] != "tool_use" {
		t.Errorf("expected tool_use, got %v", cb["type"])
	}
	if cb["name"] != "Read" {
		t.Errorf("tool name = %v, want Read", cb["name"])
	}

	// Verify result contains the tool call with parsed input
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc["name"] != "Read" {
		t.Errorf("tool name = %v, want Read", tc["name"])
	}
	input, ok := tc["input"].(map[string]interface{})
	if !ok {
		t.Fatal("input is not a map")
	}
	if input["file_path"] != "/tmp/test.txt" {
		t.Errorf("file_path = %v, want /tmp/test.txt", input["file_path"])
	}
}

func TestIntegration_ClaudeCode_MultiTurnToolWorkflow(t *testing.T) {
	// Simulate a multi-turn conversation with tool results
	// Turn 1: Assistant calls Read → User provides tool_result → Assistant responds with text

	// First, verify tool_result message conversion (request side)
	claudeReq := BuildClaudeRequest("claude-sonnet-4-6", []map[string]interface{}{
		ClaudeTextMessage("Read /tmp/hello.txt"),
		ClaudeAssistantToolUse("toolu_01READ", "Read", map[string]interface{}{
			"file_path": "/tmp/hello.txt",
		}),
		ClaudeToolResultMessage("toolu_01READ", "Hello World!"),
	}, WithSystem("You are Claude Code"), WithTools([]map[string]interface{}{
		ReadFileTool(),
		WriteFileTool(),
		BashTool(),
	}))

	// Marshal and verify the request structure
	reqBody, err := json.Marshal(claudeReq)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	// Verify it can be parsed back
	var parsed map[string]interface{}
	if err := json.Unmarshal(reqBody, &parsed); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	// Verify messages structure
	msgs, _ := parsed["messages"].([]interface{})
	if len(msgs) != 3 {
		t.Fatalf("messages count = %d, want 3", len(msgs))
	}

	// First message should be user
	msg0, _ := msgs[0].(map[string]interface{})
	if msg0["role"] != "user" {
		t.Errorf("msg[0].role = %v, want user", msg0["role"])
	}

	// Second message should be assistant with tool_use
	msg1, _ := msgs[1].(map[string]interface{})
	if msg1["role"] != "assistant" {
		t.Errorf("msg[1].role = %v, want assistant", msg1["role"])
	}

	// Third message should be user with tool_result
	msg2, _ := msgs[2].(map[string]interface{})
	if msg2["role"] != "user" {
		t.Errorf("msg[2].role = %v, want user", msg2["role"])
	}
}

func TestIntegration_ClaudeCode_ParallelToolCalls(t *testing.T) {
	// Simulate parallel tool calls (read + write in same response)
	sse := "data: {\"id\":\"chatcmpl-par\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"call_r1\",\"type\":\"function\",\"function\":{\"name\":\"Read\",\"arguments\":\"{\\\"file_path\\\":\\\"/a.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-par\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call_w1\",\"type\":\"function\",\"function\":{\"name\":\"Write\",\"arguments\":\"{\\\"file_path\\\":\\\"/b.txt\\\",\\\"content\\\":\\\"hello\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-par\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":80,\"completion_tokens\":40}}\n\n" +
		"data: [DONE]\n\n"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: []models.ClaudeContentBlock{{Type: "text", Text: "Read a.txt and write to b.txt"}}},
		},
		Tools: []models.ClaudeTool{
			{Name: "Read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "Write", Description: "Write a file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), req, context.Background())
	if result == nil {
		t.Fatal("result is nil")
	}

	events, _ := ParseSSEStream(bytes.NewReader(w.Body.Bytes()))
	AssertEventSequence(t, events)

	// Should have 2 tool_use blocks
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	if len(cbsEvents) != 2 {
		t.Fatalf("content_block_start count = %d, want 2", len(cbsEvents))
	}

	// First tool_use: Read
	cb0 := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb0["name"] != "Read" {
		t.Errorf("first tool name = %v, want Read", cb0["name"])
	}

	// Second tool_use: Write
	cb1 := cbsEvents[1].Data["content_block"].(map[string]interface{})
	if cb1["name"] != "Write" {
		t.Errorf("second tool name = %v, want Write", cb1["name"])
	}

	// Verify block indices are sequential
	idx0, _ := cbsEvents[0].Data["index"].(float64)
	idx1, _ := cbsEvents[1].Data["index"].(float64)
	if int(idx0) != 0 || int(idx1) != 1 {
		t.Errorf("block indices = [%v, %v], want [0, 1]", idx0, idx1)
	}

	// Verify result tool calls
	if len(result.ToolCalls) != 2 {
		t.Fatalf("result tool calls = %d, want 2", len(result.ToolCalls))
	}
	if result.ToolCalls[0]["name"] != "Read" {
		t.Errorf("first tool = %v, want Read", result.ToolCalls[0]["name"])
	}
	if result.ToolCalls[1]["name"] != "Write" {
		t.Errorf("second tool = %v, want Write", result.ToolCalls[1]["name"])
	}
}

func TestIntegration_ClaudeCode_ThinkingThenToolCall(t *testing.T) {
	// Simulate thinking → text → tool_use sequence (common Claude Code pattern)
	sse := "data: {\"id\":\"chatcmpl-think\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"I need to read the file first...\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-think\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Let me read that file.\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-think\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"call_t1\",\"type\":\"function\",\"function\":{\"name\":\"Read\",\"arguments\":\"{\\\"file_path\\\":\\\"/test.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-think\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":60,\"completion_tokens\":35}}\n\n" +
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

	events, _ := ParseSSEStream(bytes.NewReader(w.Body.Bytes()))
	AssertEventSequence(t, events)

	// Should have 3 content blocks: thinking(0), text(1), tool_use(2)
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	if len(cbsEvents) != 3 {
		t.Fatalf("content_block_start count = %d, want 3", len(cbsEvents))
	}

	cb0 := cbsEvents[0].Data["content_block"].(map[string]interface{})
	cb1 := cbsEvents[1].Data["content_block"].(map[string]interface{})
	cb2 := cbsEvents[2].Data["content_block"].(map[string]interface{})

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
}

func TestIntegration_ClaudeCode_LongToolName(t *testing.T) {
	// Test that long tool names are truncated and restored correctly
	longName := "mcp_server_very_long_tool_name_that_exceeds_the_openai_limit_of_64_characters_for_function_names"
	truncated := truncateToolName(longName)

	// Verify truncation happened
	if len(truncated) > 64 {
		t.Errorf("truncated name length = %d, want <= 64", len(truncated))
	}
	if truncated == longName {
		t.Error("name was not truncated")
	}

	// Create mapping
	mapping := map[string]string{truncated: longName}

	// Simulate response with truncated name
	sse := fmt.Sprintf("data: {\"id\":\"chatcmpl-long\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"call_long\",\"type\":\"function\",\"function\":{\"name\":\"%s\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n", truncated) +
		"data: {\"id\":\"chatcmpl-long\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\n" +
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

	events, _ := ParseSSEStream(bytes.NewReader(w.Body.Bytes()))

	// Verify tool name was restored
	cbsEvents := FindEvents(events, models.EventContentBlockStart)
	if len(cbsEvents) < 1 {
		t.Fatal("no content_block_start events")
	}
	cb := cbsEvents[0].Data["content_block"].(map[string]interface{})
	restoredName := cb["name"].(string)

	if restoredName != longName {
		t.Errorf("restored name = %q, want %q", restoredName, longName)
	}

	// Also verify in result
	if len(result.ToolCalls) > 0 && result.ToolCalls[0]["name"] != longName {
		t.Errorf("result tool name = %v, want %v", result.ToolCalls[0]["name"], longName)
	}
}
```

- [ ] **Step 5: 验证所有集成测试通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -v -run "TestIntegration_" -count=1 2>&1 | tail -20`
Expected:
  - Exit code: 0
  - Output contains: "PASS" for all TestIntegration_ tests

- [ ] **Step 6: 提交**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && git add converter/proxy_integration_test.go && git commit -m "test(converter): add HTTP-level proxy integration tests for protocol compliance"`

---

### Task 2: 运行全面回归测试并验证代理可用性

**Depends on:** Task 1
**Files:** None (verification only)

- [ ] **Step 1: 运行完整 converter 测试套件**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "ok"

- [ ] **Step 2: 构建并重启服务器**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server . && kill $(pgrep -f 'ccr-server server') 2>/dev/null; sleep 1 && nohup ./ccr-server server --port 54988 > /tmp/ccr-server.log 2>&1 & echo "restarted"`
Expected:
  - Exit code: 0

- [ ] **Step 3: 验证服务器响应**
Run: `curl -s http://localhost:54988/v1/models | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'models: {len(d[\"data\"])}')" 2>&1`
Expected:
  - Output contains: "models: 4"
