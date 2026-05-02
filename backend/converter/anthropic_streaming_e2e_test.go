package converter

// End-to-end streaming protocol tests for Anthropic SSE compliance.
// Tests the full OpenAI SSE → ConvertOpenAIStreamingToClaude → Anthropic SSE pipeline,
// verifying event sequence, indices, content fidelity, and protocol correctness.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ============================================================================
// Helpers
// ============================================================================

// claudeSSEEvent represents a parsed Anthropic SSE event
type claudeSSEEvent struct {
	EventType string
	Data      map[string]interface{}
}

// parseClaudeSSEEvents parses the recorder body into a list of SSE events.
func parseClaudeSSEEvents(body string) []claudeSSEEvent {
	var events []claudeSSEEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(data), &parsed); err == nil {
				events = append(events, claudeSSEEvent{
					EventType: currentEvent,
					Data:      parsed,
				})
			}
			currentEvent = ""
		}
	}
	return events
}

// getEventTypes extracts the list of event types from parsed events
func getEventTypes(events []claudeSSEEvent) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.EventType
	}
	return types
}

// findEventsByType returns all events of a given type
func findEventsByType(events []claudeSSEEvent, eventType string) []claudeSSEEvent {
	var result []claudeSSEEvent
	for _, e := range events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

// openAIChunk builds a single OpenAI SSE chunk string
func openAIChunk(id, model string, delta map[string]interface{}, finishReason string) string {
	choice := map[string]interface{}{
		"index": 0,
		"delta": delta,
	}
	if finishReason != "" {
		choice["finish_reason"] = finishReason
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{choice},
	}
	data, _ := json.Marshal(chunk)
	return "data: " + string(data) + "\n\n"
}

// openAIUsageChunk builds an OpenAI SSE chunk with usage info
func openAIUsageChunk(id, model string, promptTokens, completionTokens int, finishReason string) string {
	delta := map[string]interface{}{}
	choice := map[string]interface{}{
		"index":         0,
		"delta":         delta,
		"finish_reason": finishReason,
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{choice},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	}
	data, _ := json.Marshal(chunk)
	return "data: " + string(data) + "\n\n"
}

// openAIToolCallChunk builds an OpenAI SSE chunk for a tool call
func openAIToolCallChunk(id, model string, tcIndex int, tcID, fnName, fnArgs string) string {
	delta := map[string]interface{}{
		"tool_calls": []interface{}{
			map[string]interface{}{
				"index":  tcIndex,
				"id":     tcID,
				"type":   "function",
				"function": map[string]interface{}{
					"name":      fnName,
					"arguments": fnArgs,
				},
			},
		},
	}
	choice := map[string]interface{}{
		"index": 0,
		"delta": delta,
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{choice},
	}
	data, _ := json.Marshal(chunk)
	return "data: " + string(data) + "\n\n"
}

// openAIReasoningChunk builds an OpenAI SSE chunk with reasoning_content
func openAIReasoningChunk(id, model, reasoning string) string {
	delta := map[string]interface{}{
		"reasoning_content": reasoning,
	}
	choice := map[string]interface{}{
		"index": 0,
		"delta": delta,
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{choice},
	}
	data, _ := json.Marshal(chunk)
	return "data: " + string(data) + "\n\n"
}

// buildOpenAIStream builds a complete OpenAI SSE stream from content parts
func buildOpenAIStream(id, model, content string, finishReason string) string {
	var sb strings.Builder
	// Send content in rune-safe chunks (5 runes per chunk to avoid splitting multi-byte chars)
	runes := []rune(content)
	chunkSize := 5
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		delta := map[string]interface{}{"content": string(runes[i:end])}
		sb.WriteString(openAIChunk(id, model, delta, ""))
	}
	// Send finish
	sb.WriteString(openAIChunk(id, model, map[string]interface{}{}, finishReason))
	return sb.String()
}

// runStreamingTest sets up gin context and runs the streaming converter
func runStreamingTest(t *testing.T, openaiSSE string, model string) ([]claudeSSEEvent, *StreamingResult) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	originalReq := &models.ClaudeMessagesRequest{
		Model:     model,
		MaxTokens: 4096,
		Stream:    true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result *StreamingResult
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		result = ConvertOpenAIStreamingToClaude(c, strings.NewReader(openaiSSE), originalReq, ctx)
	}()

	wg.Wait()

	events := parseClaudeSSEEvents(w.Body.String())
	return events, result
}

// runStreamingTestWithDone sends data: [DONE] at the end
func runStreamingTestWithDone(t *testing.T, openaiSSE string, model string) ([]claudeSSEEvent, *StreamingResult) {
	t.Helper()
	fullStream := openaiSSE + "data: [DONE]\n\n"
	return runStreamingTest(t, fullStream, model)
}

// ============================================================================
// Section 1: Basic Text Streaming
// ============================================================================

func TestStreamingE2E_SimpleText(t *testing.T) {
	sse := buildOpenAIStream("chatcmpl-001", "gpt-4", "Hello, world!", "stop")
	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Content != "Hello, world!" {
		t.Errorf("content = %q, want %q", result.Content, "Hello, world!")
	}
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result.StopReason)
	}

	// Verify event sequence
	types := getEventTypes(events)
	if types[0] != "message_start" {
		t.Errorf("first event = %q, want message_start", types[0])
	}
	if types[1] != "ping" {
		t.Errorf("second event = %q, want ping", types[1])
	}

	// Must have content_block_start, content_block_delta(s), content_block_stop
	hasCBS := false
	hasCBD := false
	hasCBSt := false
	for _, et := range types {
		if et == "content_block_start" { hasCBS = true }
		if et == "content_block_delta" { hasCBD = true }
		if et == "content_block_stop" { hasCBSt = true }
	}
	if !hasCBS { t.Error("missing content_block_start") }
	if !hasCBD { t.Error("missing content_block_delta") }
	if !hasCBSt { t.Error("missing content_block_stop") }

	// Last two events should be message_delta and message_stop
	if len(types) < 2 { t.Fatalf("too few events: %d", len(types)) }
	if types[len(types)-2] != "message_delta" {
		t.Errorf("second-to-last event = %q, want message_delta", types[len(types)-2])
	}
	if types[len(types)-1] != "message_stop" {
		t.Errorf("last event = %q, want message_stop", types[len(types)-1])
	}
}

func TestStreamingE2E_EmptyContent(t *testing.T) {
	sse := openAIChunk("chatcmpl-002", "gpt-4", map[string]interface{}{}, "stop")
	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	// Empty content should still produce a valid protocol response
	if result == nil {
		t.Fatal("result is nil")
	}
	types := getEventTypes(events)
	if types[0] != "message_start" {
		t.Errorf("first event = %q, want message_start", types[0])
	}
	// Should still end properly
	last := types[len(types)-1]
	if last != "message_stop" {
		t.Errorf("last event = %q, want message_stop", last)
	}
}

func TestStreamingE2E_SingleChunkText(t *testing.T) {
	delta := map[string]interface{}{"content": "Short"}
	sse := openAIChunk("chatcmpl-003", "gpt-4", delta, "") +
		openAIChunk("chatcmpl-003", "gpt-4", map[string]interface{}{}, "stop")
	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "Short" {
		t.Fatalf("content = %q", result)
	}
	cbdEvents := findEventsByType(events, "content_block_delta")
	if len(cbdEvents) != 1 {
		t.Errorf("delta events = %d, want 1", len(cbdEvents))
	}
}

func TestStreamingE2E_ManyChunks(t *testing.T) {
	var sb strings.Builder
	id := "chatcmpl-004"
	model := "gpt-4"
	for i := 0; i < 50; i++ {
		delta := map[string]interface{}{"content": fmt.Sprintf("word%d ", i)}
		sb.WriteString(openAIChunk(id, model, delta, ""))
	}
	sb.WriteString(openAIChunk(id, model, map[string]interface{}{}, "stop"))
	events, result := runStreamingTestWithDone(t, sb.String(), "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Content should contain all 50 words
	for i := 0; i < 50; i++ {
		expected := fmt.Sprintf("word%d ", i)
		if !strings.Contains(result.Content, expected) {
			t.Errorf("missing word%d in content", i)
		}
	}
	cbdEvents := findEventsByType(events, "content_block_delta")
	if len(cbdEvents) != 50 {
		t.Errorf("delta events = %d, want 50", len(cbdEvents))
	}
}

func TestStreamingE2E_NewlinesInContent(t *testing.T) {
	content := "line1\nline2\nline3\n"
	sse := buildOpenAIStream("chatcmpl-005", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

func TestStreamingE2E_CJKContent(t *testing.T) {
	content := "你好世界こんにちは안녕하세요"
	sse := buildOpenAIStream("chatcmpl-006", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

func TestStreamingE2E_EmojiContent(t *testing.T) {
	content := "Hello 🌍🎉🚀💻🔧"
	sse := buildOpenAIStream("chatcmpl-007", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

func TestStreamingE2E_LargePayload(t *testing.T) {
	// 10KB of text
	content := strings.Repeat("A", 10*1024)
	sse := buildOpenAIStream("chatcmpl-008", "gpt-4", content, "stop")
	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.Content) != 10*1024 {
		t.Errorf("content length = %d, want %d", len(result.Content), 10*1024)
	}
	cbdEvents := findEventsByType(events, "content_block_delta")
	if len(cbdEvents) < 1 {
		t.Error("expected multiple delta events for large payload")
	}
}

func TestStreamingE2E_SpecialCharsInContent(t *testing.T) {
	content := `{"key": "value", "nested": {"a": 1}} <html> & "quotes" 'single' \n\t\r`
	sse := buildOpenAIStream("chatcmpl-009", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

func TestStreamingE2E_CodeBlockContent(t *testing.T) {
	content := "```python\ndef hello():\n    print('Hello')\n```"
	sse := buildOpenAIStream("chatcmpl-010", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

// ============================================================================
// Section 2: SSE Protocol Compliance
// ============================================================================

func TestStreamingE2E_MessageStartStructure(t *testing.T) {
	sse := buildOpenAIStream("chatcmpl-011", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	startEvents := findEventsByType(events, "message_start")
	if len(startEvents) != 1 {
		t.Fatalf("message_start count = %d, want 1", len(startEvents))
	}

	msgStart := startEvents[0].Data
	msg, ok := msgStart["message"].(map[string]interface{})
	if !ok {
		t.Fatal("message_start.message is not a map")
	}
	if msg["type"] != "message" {
		t.Errorf("message.type = %v, want message", msg["type"])
	}
	if msg["role"] != "assistant" {
		t.Errorf("message.role = %v, want assistant", msg["role"])
	}
	if msg["model"] != "claude-sonnet-4-6" {
		t.Errorf("message.model = %v, want claude-sonnet-4-6", msg["model"])
	}
	if msg["id"] == nil || msg["id"] == "" {
		t.Error("message.id is empty")
	}
}

func TestStreamingE2E_ContentBlockStartIndex(t *testing.T) {
	sse := buildOpenAIStream("chatcmpl-012", "gpt-4", "hello", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) == 0 {
		t.Fatal("no content_block_start events")
	}

	idx := cbsEvents[0].Data["index"]
	if idx != float64(0) {
		t.Errorf("content_block_start index = %v, want 0", idx)
	}

	cb, ok := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if !ok {
		t.Fatal("content_block is not a map")
	}
	if cb["type"] != "text" {
		t.Errorf("content_block type = %v, want text", cb["type"])
	}
}

func TestStreamingE2E_ContentBlockDeltaType(t *testing.T) {
	delta := map[string]interface{}{"content": "hi"}
	sse := openAIChunk("chatcmpl-013", "gpt-4", delta, "") +
		openAIChunk("chatcmpl-013", "gpt-4", map[string]interface{}{}, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbdEvents := findEventsByType(events, "content_block_delta")
	if len(cbdEvents) == 0 {
		t.Fatal("no content_block_delta events")
	}

	deltaData, ok := cbdEvents[0].Data["delta"].(map[string]interface{})
	if !ok {
		t.Fatal("delta is not a map")
	}
	if deltaData["type"] != "text_delta" {
		t.Errorf("delta type = %v, want text_delta", deltaData["type"])
	}
	if deltaData["text"] != "hi" {
		t.Errorf("delta text = %v, want hi", deltaData["text"])
	}
}

func TestStreamingE2E_MessageDeltaStructure(t *testing.T) {
	sse := openAIUsageChunk("chatcmpl-014", "gpt-4", 100, 50, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	mdEvents := findEventsByType(events, "message_delta")
	if len(mdEvents) != 1 {
		t.Fatalf("message_delta count = %d, want 1", len(mdEvents))
	}

	md := mdEvents[0].Data
	deltaData, ok := md["delta"].(map[string]interface{})
	if !ok {
		t.Fatal("message_delta.delta is not a map")
	}
	if deltaData["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", deltaData["stop_reason"])
	}

	usage, ok := md["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("message_delta.usage is not a map")
	}
	if usage["input_tokens"] != float64(100) {
		t.Errorf("input_tokens = %v, want 100", usage["input_tokens"])
	}
	if usage["output_tokens"] != float64(50) {
		t.Errorf("output_tokens = %v, want 50", usage["output_tokens"])
	}
}

func TestStreamingE2E_PingAfterMessageStart(t *testing.T) {
	sse := buildOpenAIStream("chatcmpl-015", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	types := getEventTypes(events)
	if len(types) < 2 {
		t.Fatalf("too few events: %d", len(types))
	}
	if types[0] != "message_start" {
		t.Errorf("first event = %q, want message_start", types[0])
	}
	if types[1] != "ping" {
		t.Errorf("second event = %q, want ping", types[1])
	}
}

func TestStreamingE2E_MessageStopIsLast(t *testing.T) {
	sse := buildOpenAIStream("chatcmpl-016", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	types := getEventTypes(events)
	if len(types) == 0 {
		t.Fatal("no events")
	}
	if types[len(types)-1] != "message_stop" {
		t.Errorf("last event = %q, want message_stop", types[len(types)-1])
	}
}

func TestStreamingE2E_ContentBlockLifecycle(t *testing.T) {
	// Each content block should have: start → delta(s) → stop
	sse := buildOpenAIStream("chatcmpl-017", "gpt-4", "hello world", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbsIdx := -1
	cbdFirst := -1
	cbdLast := -1
	cbStopIdx := -1

	for i, e := range events {
		if e.EventType == "content_block_start" && cbsIdx == -1 {
			cbsIdx = i
		}
		if e.EventType == "content_block_delta" {
			if cbdFirst == -1 {
				cbdFirst = i
			}
			cbdLast = i
		}
		if e.EventType == "content_block_stop" && cbStopIdx == -1 {
			cbStopIdx = i
		}
	}

	if cbsIdx == -1 { t.Error("no content_block_start") }
	if cbdFirst == -1 { t.Error("no content_block_delta") }
	if cbStopIdx == -1 { t.Error("no content_block_stop") }
	if cbsIdx >= cbdFirst { t.Error("content_block_start not before content_block_delta") }
	if cbdLast >= cbStopIdx { t.Error("content_block_delta not before content_block_stop") }
}

// ============================================================================
// Section 3: Tool Call Streaming
// ============================================================================

func TestStreamingE2E_SingleToolCall(t *testing.T) {
	id := "chatcmpl-100"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_abc123", "read_file", `{"path":"/tmp/test.txt"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", result.StopReason)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls count = %d, want 1", len(result.ToolCalls))
	}

	tc := result.ToolCalls[0]
	if tc["type"] != "tool_use" {
		t.Errorf("tool type = %v, want tool_use", tc["type"])
	}
	if tc["id"] != "toolu_abc123" {
		t.Errorf("tool id = %v, want toolu_abc123", tc["id"])
	}
	if tc["name"] != "read_file" {
		t.Errorf("tool name = %v, want read_file", tc["name"])
	}

	// Verify SSE events
	cbsEvents := findEventsByType(events, "content_block_start")
	toolCBS := false
	for _, e := range cbsEvents {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "tool_use" {
				toolCBS = true
				if cb["id"] != "toolu_abc123" {
					t.Errorf("tool id = %v, want toolu_abc123", cb["id"])
				}
				if cb["name"] != "read_file" {
					t.Errorf("tool name = %v, want read_file", cb["name"])
				}
			}
		}
	}
	if !toolCBS {
		t.Error("no tool_use content_block_start found")
	}
}

func TestStreamingE2E_ToolCallStreamingArgs(t *testing.T) {
	// Tool call arguments arrive in multiple chunks
	id := "chatcmpl-101"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_def456", "write_file", `{"path":"/`) +
		openAIToolCallChunk(id, model, 0, "", "", `tmp/out.txt","con`) +
		openAIToolCallChunk(id, model, 0, "", "", `tent":"hello"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}

	tc := result.ToolCalls[0]
	input, ok := tc["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("input type = %T, want map", tc["input"])
	}
	if input["path"] != "/tmp/out.txt" {
		t.Errorf("path = %v, want /tmp/out.txt", input["path"])
	}
	if input["content"] != "hello" {
		t.Errorf("content = %v, want hello", input["content"])
	}

	// Should have input_json_delta events
	cbdEvents := findEventsByType(events, "content_block_delta")
	jsonDeltas := 0
	for _, e := range cbdEvents {
		if delta, ok := e.Data["delta"].(map[string]interface{}); ok {
			if delta["type"] == "input_json_delta" {
				jsonDeltas++
			}
		}
	}
	if jsonDeltas != 3 {
		t.Errorf("input_json_delta events = %d, want 3", jsonDeltas)
	}
}

func TestStreamingE2E_MultipleToolCalls(t *testing.T) {
	id := "chatcmpl-102"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_001", "read_file", `{"path":"/a"}`) +
		openAIToolCallChunk(id, model, 1, "call_002", "write_file", `{"path":"/b","content":"x"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool_calls = %d, want 2", len(result.ToolCalls))
	}
	if result.ToolCalls[0]["name"] != "read_file" {
		t.Errorf("first tool = %v, want read_file", result.ToolCalls[0]["name"])
	}
	if result.ToolCalls[1]["name"] != "write_file" {
		t.Errorf("second tool = %v, want write_file", result.ToolCalls[1]["name"])
	}
}

func TestStreamingE2E_ToolCallIndices(t *testing.T) {
	id := "chatcmpl-103"
	model := "gpt-4"

	// Text + 2 tool calls
	sse := openAIChunk(id, model, map[string]interface{}{"content": "I'll help."}, "") +
		openAIToolCallChunk(id, model, 0, "call_t1", "tool_a", `{"x":1}`) +
		openAIToolCallChunk(id, model, 1, "call_t2", "tool_b", `{"y":2}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbsEvents := findEventsByType(events, "content_block_start")
	// Should have: text(0), tool_a(1), tool_b(2)
	if len(cbsEvents) < 3 {
		t.Fatalf("content_block_start count = %d, want >= 3", len(cbsEvents))
	}

	expectedIndices := []float64{0, 1, 2}
	for i, idx := range expectedIndices {
		if cbsEvents[i].Data["index"] != idx {
			t.Errorf("content_block_start[%d].index = %v, want %v", i, cbsEvents[i].Data["index"], idx)
		}
	}
}

func TestStreamingE2E_ToolCallWithEmptyArgs(t *testing.T) {
	id := "chatcmpl-104"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_empty", "simple_tool", `{}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}
	input, ok := result.ToolCalls[0]["input"].(map[string]interface{})
	if !ok || len(input) != 0 {
		t.Errorf("input = %v, want empty map", input)
	}
}

func TestStreamingE2E_ToolCallWithNestedArgs(t *testing.T) {
	id := "chatcmpl-105"
	model := "gpt-4"
	args := `{"options":{"recursive":true,"depth":3},"path":"/src"}`
	sse := openAIToolCallChunk(id, model, 0, "call_nested", "search", args) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	opts, ok := input["options"].(map[string]interface{})
	if !ok {
		t.Fatal("options not a map")
	}
	if opts["recursive"] != true {
		t.Error("recursive should be true")
	}
	if opts["depth"] != float64(3) {
		t.Errorf("depth = %v, want 3", opts["depth"])
	}
}

func TestStreamingE2E_ThreeToolCallsStreaming(t *testing.T) {
	id := "chatcmpl-106"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_1", "read", `{"f":"a"}`) +
		openAIToolCallChunk(id, model, 1, "call_2", "write", `{"f":"b"}`) +
		openAIToolCallChunk(id, model, 2, "call_3", "exec", `{"cmd":"ls"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 3 {
		t.Fatalf("tool_calls = %d, want 3", len(result.ToolCalls))
	}
	// Map iteration order is non-deterministic, check all names are present
	names := map[string]bool{"read": false, "write": false, "exec": false}
	for _, tc := range result.ToolCalls {
		name, ok := tc["name"].(string)
		if !ok {
			t.Errorf("tool name is not a string: %v", tc["name"])
			continue
		}
		if _, exists := names[name]; !exists {
			t.Errorf("unexpected tool name: %s", name)
		}
		names[name] = true
	}
	for name, found := range names {
		if !found {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestStreamingE2E_TextFollowedByToolCall(t *testing.T) {
	id := "chatcmpl-107"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "Let me check."}, "") +
		openAIToolCallChunk(id, model, 0, "call_mix", "grep", `{"pattern":"TODO"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Content != "Let me check." {
		t.Errorf("content = %q, want 'Let me check.'", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Errorf("tool_calls = %d, want 1", len(result.ToolCalls))
	}
}

// ============================================================================
// Section 4: Thinking/Reasoning Streaming
// ============================================================================

func TestStreamingE2E_ReasoningContent(t *testing.T) {
	id := "chatcmpl-200"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "Let me think...") +
		openAIReasoningChunk(id, model, " Step by step.") +
		openAIChunk(id, model, map[string]interface{}{"content": "The answer is 42."}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Content != "The answer is 42." {
		t.Errorf("content = %q, want 'The answer is 42.'", result.Content)
	}

	// Should have thinking block events
	cbsEvents := findEventsByType(events, "content_block_start")
	thinkingFound := false
	for _, e := range cbsEvents {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "thinking" {
				thinkingFound = true
			}
		}
	}
	if !thinkingFound {
		t.Error("no thinking content_block_start found")
	}

	// Should have thinking_delta events
	cbdEvents := findEventsByType(events, "content_block_delta")
	thinkingDeltas := 0
	for _, e := range cbdEvents {
		if delta, ok := e.Data["delta"].(map[string]interface{}); ok {
			if delta["type"] == "thinking_delta" {
				thinkingDeltas++
			}
		}
	}
	if thinkingDeltas != 2 {
		t.Errorf("thinking_delta events = %d, want 2", thinkingDeltas)
	}
}

func TestStreamingE2E_ReasoningThenTextBlockIndices(t *testing.T) {
	id := "chatcmpl-201"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "Hmm...") +
		openAIChunk(id, model, map[string]interface{}{"content": "Done."}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbsEvents := findEventsByType(events, "content_block_start")
	// thinking at index 0, text at index 1
	if len(cbsEvents) < 2 {
		t.Fatalf("content_block_start count = %d, want >= 2", len(cbsEvents))
	}
	if cbsEvents[0].Data["index"] != float64(0) {
		t.Errorf("thinking block index = %v, want 0", cbsEvents[0].Data["index"])
	}
	if cbsEvents[1].Data["index"] != float64(1) {
		t.Errorf("text block index = %v, want 1", cbsEvents[1].Data["index"])
	}
}

func TestStreamingE2E_ReasoningOnly(t *testing.T) {
	id := "chatcmpl-202"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "Just thinking...") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Thinking only, no text content
	if result.Content != "" {
		t.Errorf("content = %q, want empty", result.Content)
	}

	// Should have thinking block events
	thinkingCBS := false
	for _, e := range findEventsByType(events, "content_block_start") {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "thinking" {
				thinkingCBS = true
			}
		}
	}
	if !thinkingCBS {
		t.Error("no thinking block found")
	}
}

func TestStreamingE2E_ReasoningWithMultipleChunks(t *testing.T) {
	id := "chatcmpl-203"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "First thought. ") +
		openAIReasoningChunk(id, model, "Second thought. ") +
		openAIReasoningChunk(id, model, "Third thought.") +
		openAIChunk(id, model, map[string]interface{}{"content": "Final."}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	thinkingDeltas := 0
	for _, e := range findEventsByType(events, "content_block_delta") {
		if delta, ok := e.Data["delta"].(map[string]interface{}); ok {
			if delta["type"] == "thinking_delta" {
				thinkingDeltas++
			}
		}
	}
	if thinkingDeltas != 3 {
		t.Errorf("thinking_delta events = %d, want 3", thinkingDeltas)
	}
}

func TestStreamingE2E_ReasoningThenToolCall(t *testing.T) {
	id := "chatcmpl-204"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "Need to search...") +
		openAIToolCallChunk(id, model, 0, "call_r1", "search", `{"q":"test"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", result.StopReason)
	}

	// Should have thinking block (index 0) and tool block
	cbsEvents := findEventsByType(events, "content_block_start")
	types := []string{}
	for _, e := range cbsEvents {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			types = append(types, cb["type"].(string))
		}
	}
	hasThinking := false
	for _, typ := range types {
		if typ == "thinking" { hasThinking = true }
	}
	if !hasThinking {
		t.Error("missing thinking block")
	}
}

// ============================================================================
// Section 5: Stop Reason Mapping
// ============================================================================

func TestStreamingE2E_StopReason_EndTurn(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "hi"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result)
	}
}

func TestStreamingE2E_StopReason_ToolUse(t *testing.T) {
	sse := openAIToolCallChunk("c", "gpt-4", 0, "id", "tool", `{}`) +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "tool_calls")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", result)
	}
}

func TestStreamingE2E_StopReason_MaxTokens(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "hi"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "length")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.StopReason != "max_tokens" {
		t.Errorf("stop_reason = %q, want max_tokens", result)
	}
}

func TestStreamingE2E_StopReason_FunctionCall(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "hi"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "function_call")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use (from function_call)", result)
	}
}

func TestStreamingE2E_StopReasonInMessageDelta(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "done"}, "") +
		openAIUsageChunk("c", "gpt-4", 50, 25, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	mdEvents := findEventsByType(events, "message_delta")
	if len(mdEvents) == 0 {
		t.Fatal("no message_delta event")
	}
	delta, ok := mdEvents[0].Data["delta"].(map[string]interface{})
	if !ok {
		t.Fatal("delta not a map")
	}
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason in message_delta = %v, want end_turn", delta["stop_reason"])
	}
}

// ============================================================================
// Section 6: Usage Tracking
// ============================================================================

func TestStreamingE2E_UsageInResult(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "test"}, "") +
		openAIUsageChunk("c", "gpt-4", 100, 50, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.InputTokens != 100 {
		t.Errorf("input_tokens = %d, want 100", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("output_tokens = %d, want 50", result.OutputTokens)
	}
}

func TestStreamingE2E_UsageInMessageDeltaEvent(t *testing.T) {
	sse := openAIUsageChunk("c", "gpt-4", 200, 100, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	mdEvents := findEventsByType(events, "message_delta")
	if len(mdEvents) == 0 {
		t.Fatal("no message_delta")
	}
	usage, ok := mdEvents[0].Data["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("usage not a map")
	}
	if usage["input_tokens"] != float64(200) {
		t.Errorf("input_tokens = %v, want 200", usage["input_tokens"])
	}
	if usage["output_tokens"] != float64(100) {
		t.Errorf("output_tokens = %v, want 100", usage["output_tokens"])
	}
}

func TestStreamingE2E_CacheTokensInUsage(t *testing.T) {
	// OpenAI chunk with cached tokens
	delta := map[string]interface{}{}
	choice := map[string]interface{}{
		"index":         0,
		"delta":         delta,
		"finish_reason": "stop",
	}
	chunk := map[string]interface{}{
		"id":      "c",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   "gpt-4",
		"choices": []interface{}{choice},
		"usage": map[string]interface{}{
			"prompt_tokens":     150,
			"completion_tokens": 30,
			"total_tokens":      180,
			"prompt_tokens_details": map[string]interface{}{
				"cached_tokens": 80,
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\ndata: [DONE]\n\n"

	events, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.InputTokens != 150 {
		t.Errorf("input_tokens = %d, want 150", result.InputTokens)
	}

	// Check message_delta event has cache_read_input_tokens
	mdEvents := findEventsByType(events, "message_delta")
	if len(mdEvents) > 0 {
		usage, ok := mdEvents[0].Data["usage"].(map[string]interface{})
		if ok && usage["cache_read_input_tokens"] != nil {
			if usage["cache_read_input_tokens"] != float64(80) {
				t.Errorf("cache_read_input_tokens = %v, want 80", usage["cache_read_input_tokens"])
			}
		}
	}
}

func TestStreamingE2E_NoUsageData(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "test"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Should default to 0 when no usage chunk
	if result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Errorf("usage should be 0 when no usage data: in=%d out=%d", result.InputTokens, result.OutputTokens)
	}
}

func TestStreamingE2E_UsageUpdatedMidStream(t *testing.T) {
	// Usage info arrives before finish
	id := "c"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "part1"}, "")
	// Usage chunk with no finish
	delta := map[string]interface{}{}
	choice := map[string]interface{}{
		"index": 0,
		"delta": delta,
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"model":   model,
		"choices": []interface{}{choice},
		"usage": map[string]interface{}{
			"prompt_tokens":     50,
			"completion_tokens": 10,
		},
	}
	data, _ := json.Marshal(chunk)
	sse += "data: " + string(data) + "\n\n"

	// Final content + finish
	sse += openAIChunk(id, model, map[string]interface{}{"content": "part2"}, "") +
		openAIUsageChunk(id, model, 100, 30, "stop") +
		"data: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Should use the latest usage (the one with finish_reason)
	if result.InputTokens != 100 {
		t.Errorf("input_tokens = %d, want 100", result.InputTokens)
	}
	if result.OutputTokens != 30 {
		t.Errorf("output_tokens = %d, want 30", result.OutputTokens)
	}
}

// ============================================================================
// Section 7: Edge Cases
// ============================================================================

func TestStreamingE2E_EmptyLines(t *testing.T) {
	id := "c"

	sse := "\n\ndata: " + `{"id":"` + id + `","choices":[{"delta":{"content":"hi"}}]}` + "\n\n\n\n" +
		"data: " + `{"id":"` + id + `","choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "hi" {
		t.Errorf("content = %q, want 'hi'", result)
	}
}

func TestStreamingE2E_DataPrefixWithoutSpace(t *testing.T) {
	// "data:" without space (some providers do this)
	id := "c"
	chunk := map[string]interface{}{
		"id":      id,
		"choices": []interface{}{
			map[string]interface{}{
				"delta":         map[string]interface{}{"content": "compact"},
				"finish_reason": "stop",
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data:" + string(data) + "\n\ndata: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "compact" {
		t.Errorf("content = %q, want 'compact'", result)
	}
}

func TestStreamingE2E_MalformedJSON(t *testing.T) {
	id := "c"
	model := "gpt-4"

	sse := "data: {invalid json}\n\n" +
		openAIChunk(id, model, map[string]interface{}{"content": "after bad"}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop") +
		"data: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "after bad" {
		t.Errorf("content = %q, want 'after bad'", result)
	}
}

func TestStreamingE2E_NoChoices(t *testing.T) {
	chunk := map[string]interface{}{
		"id":      "c",
		"choices": []interface{}{},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\n" +
		openAIChunk("c", "gpt-4", map[string]interface{}{"content": "after"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop") +
		"data: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "after" {
		t.Errorf("content = %q, want 'after'", result)
	}
}

func TestStreamingE2E_DoneMarker(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "ok"}, "") +
		"data: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Should complete without finish_reason chunk, using default end_turn
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result.StopReason)
	}
}

func TestStreamingE2E_ContextCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Build a slow stream
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte(openAIChunk("c", "gpt-4", map[string]interface{}{"content": "start"}, "")))
		time.Sleep(100 * time.Millisecond)
		// Cancel context before stream finishes
		cancel()
	}()

	result := ConvertOpenAIStreamingToClaude(c, pr, originalReq, ctx)

	// Should handle cancellation gracefully
	// Result may be nil due to cancellation
	_ = result
}

func TestStreamingE2E_MultipleMalformedThenGood(t *testing.T) {
	id := "c"
	model := "gpt-4"

	sse := "data: {}\n\n" +
		"data: {bad\n\n" +
		"data: null\n\n" +
		"data: 42\n\n" +
		openAIChunk(id, model, map[string]interface{}{"content": "good"}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop") +
		"data: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "good" {
		t.Errorf("content = %q, want 'good'", result)
	}
}

func TestStreamingE2E_EmptyDelta(t *testing.T) {
	// Delta with no content, no tool_calls, no reasoning
	delta := map[string]interface{}{
		"role": "assistant",
	}
	sse := openAIChunk("c", "gpt-4", delta, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{"content": "real"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "real" {
		t.Errorf("content = %q, want 'real'", result)
	}
}

func TestStreamingE2E_ContentFieldNull(t *testing.T) {
	// OpenAI sometimes sends content: null
	chunk := map[string]interface{}{
		"id":      "c",
		"model":   "gpt-4",
		"choices": []interface{}{
			map[string]interface{}{
				"delta":         map[string]interface{}{"content": nil},
				"finish_reason": "stop",
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\ndata: [DONE]\n\n"

	events, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	// Should handle null content gracefully
	if result == nil {
		t.Fatal("result is nil")
	}
	// Should still produce valid SSE sequence
	lastEvents := findEventsByType(events, "message_stop")
	if len(lastEvents) == 0 {
		t.Error("missing message_stop event")
	}
}

// ============================================================================
// Section 8: Model Preservation
// ============================================================================

func TestStreamingE2E_ModelPreserved(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-opus-4-7")

	startEvents := findEventsByType(events, "message_start")
	if len(startEvents) == 0 {
		t.Fatal("no message_start")
	}
	msg, ok := startEvents[0].Data["message"].(map[string]interface{})
	if !ok {
		t.Fatal("message not a map")
	}
	if msg["model"] != "claude-opus-4-7" {
		t.Errorf("model = %v, want claude-opus-4-7", msg["model"])
	}
}

func TestStreamingE2E_DifferentModelNames(t *testing.T) {
	models_to_test := []string{
		"claude-sonnet-4-6",
		"claude-opus-4-7",
		"claude-haiku-4-5-20251001",
		"claude-3-5-sonnet-20241022",
		"claude-3-opus-20240229",
	}

	for _, model := range models_to_test {
		t.Run(model, func(t *testing.T) {
			sse := buildOpenAIStream("c", "gpt-4", "ok", "stop")
			events, _ := runStreamingTestWithDone(t, sse, model)

			startEvents := findEventsByType(events, "message_start")
			if len(startEvents) == 0 {
				t.Fatal("no message_start")
			}
			msg := startEvents[0].Data["message"].(map[string]interface{})
			if msg["model"] != model {
				t.Errorf("model = %v, want %s", msg["model"], model)
			}
		})
	}
}

// ============================================================================
// Section 9: Mixed Content Patterns
// ============================================================================

func TestStreamingE2E_ThinkingTextToolSequence(t *testing.T) {
	id := "chatcmpl-300"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "Analyzing the request...") +
		openAIChunk(id, model, map[string]interface{}{"content": "I'll search for that."}, "") +
		openAIToolCallChunk(id, model, 0, "call_search", "search_files", `{"pattern":"TODO"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", result.StopReason)
	}
	if result.Content != "I'll search for that." {
		t.Errorf("content = %q", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}

	// Verify block indices: thinking=0, text=1, tool=2
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) < 3 {
		t.Fatalf("content_block_start = %d, want >= 3", len(cbsEvents))
	}

	// Check types
	blockTypes := []string{}
	for _, e := range cbsEvents {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			blockTypes = append(blockTypes, cb["type"].(string))
		}
	}

	if blockTypes[0] != "thinking" {
		t.Errorf("block[0] = %s, want thinking", blockTypes[0])
	}
	if blockTypes[1] != "text" {
		t.Errorf("block[1] = %s, want text", blockTypes[1])
	}
	if blockTypes[2] != "tool_use" {
		t.Errorf("block[2] = %s, want tool_use", blockTypes[2])
	}
}

func TestStreamingE2E_ThinkingTextThenMoreText(t *testing.T) {
	id := "chatcmpl-301"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "Hmm...") +
		openAIChunk(id, model, map[string]interface{}{"content": "Part 1. "}, "") +
		openAIChunk(id, model, map[string]interface{}{"content": "Part 2."}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	expected := "Part 1. Part 2."
	if result.Content != expected {
		t.Errorf("content = %q, want %q", result.Content, expected)
	}
}

func TestStreamingE2E_ToolCallOnlyNoText(t *testing.T) {
	id := "chatcmpl-302"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_direct", "exec", `{"cmd":"ls"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}

	// No text block should be created when only tool calls are present
	// (empty text blocks before tool blocks violate Claude SSE protocol)
	cbsEvents := findEventsByType(events, "content_block_start")
	for _, e := range cbsEvents {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "text" {
				t.Error("unexpected empty text block before tool blocks - violates Claude SSE protocol")
			}
		}
	}

	// Should have tool_use block directly
	hasToolBlock := false
	for _, e := range cbsEvents {
		if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
			if cb["type"] == "tool_use" {
				hasToolBlock = true
			}
		}
	}
	if !hasToolBlock {
		t.Error("expected tool_use content block")
	}
}

// ============================================================================
// Section 10: Real-world Claude Code CLI Patterns
// ============================================================================

func TestStreamingE2E_ClaudeCodeReadFilePattern(t *testing.T) {
	// Simulates: Claude Code reads a file using tool call
	id := "chatcmpl-400"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "I'll read that file for you."}, "") +
		openAIToolCallChunk(id, model, 0, "call_read", "Read", `{"file_path":"/src/main.go"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc["name"] != "Read" {
		t.Errorf("tool name = %v, want Read", tc["name"])
	}
	input := tc["input"].(map[string]interface{})
	if input["file_path"] != "/src/main.go" {
		t.Errorf("file_path = %v, want /src/main.go", input["file_path"])
	}

	// Verify event order: message_start → ping → content_block_start(text) → delta(text) →
	// content_block_stop(text) → content_block_start(tool) → delta(tool) → content_block_stop(tool) →
	// message_delta → message_stop
	_ = events
}

func TestStreamingE2E_ClaudeCodeWriteFilePattern(t *testing.T) {
	// Simulates: Claude Code writes a file with tool call, streaming args
	id := "chatcmpl-401"
	model := "gpt-4"

	// Large file content streamed in chunks (no raw newlines to avoid SSE scanner issues)
	fileContent := strings.Repeat("line of code\\n", 100)
	sse := openAIChunk(id, model, map[string]interface{}{"content": "Creating the file now."}, "") +
		openAIToolCallChunk(id, model, 0, "call_write", "Write", `{"file_path":"/src/new.go","content":"`) +
		openAIToolCallChunk(id, model, 0, "", "", fileContent[:50]) +
		openAIToolCallChunk(id, model, 0, "", "", fileContent[50:100]) +
		openAIToolCallChunk(id, model, 0, "", "", `"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	input := tc["input"].(map[string]interface{})
	if input["file_path"] != "/src/new.go" {
		t.Errorf("file_path = %v", input["file_path"])
	}
}

func TestStreamingE2E_ClaudeCodeMultiToolPattern(t *testing.T) {
	// Simulates: Claude Code reads multiple files then writes
	id := "chatcmpl-402"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "Let me read these files."}, "") +
		openAIToolCallChunk(id, model, 0, "call_r1", "Read", `{"file_path":"/a.go"}`) +
		openAIToolCallChunk(id, model, 1, "call_r2", "Read", `{"file_path":"/b.go"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool_calls = %d, want 2", len(result.ToolCalls))
	}
}

func TestStreamingE2E_ClaudeCodeBashCommand(t *testing.T) {
	id := "chatcmpl-403"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_bash", "Bash", `{"command":"go test ./..."}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	tc := result.ToolCalls[0]
	input := tc["input"].(map[string]interface{})
	if input["command"] != "go test ./..." {
		t.Errorf("command = %v", input["command"])
	}
}

func TestStreamingE2E_ClaudeCodeGrepPattern(t *testing.T) {
	id := "chatcmpl-404"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "Searching for references."}, "") +
		openAIToolCallChunk(id, model, 0, "call_grep", "Grep", `{"pattern":"func.*Handler","path":"/src"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	tc := result.ToolCalls[0]
	input := tc["input"].(map[string]interface{})
	if input["pattern"] != "func.*Handler" {
		t.Errorf("pattern = %v", input["pattern"])
	}
}

// ============================================================================
// Section 11: Protocol Sequence Validation
// ============================================================================

func TestStreamingE2E_FullEventSequence_TextOnly(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "hello", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	// Expected: message_start, ping, content_block_start, content_block_delta*, content_block_stop,
	//           message_delta, message_stop
	types := getEventTypes(events)
	if len(types) < 6 {
		t.Fatalf("too few events: %v", types)
	}

	expected := []string{"message_start", "ping"}
	for i, e := range expected {
		if types[i] != e {
			t.Errorf("event[%d] = %q, want %q", i, types[i], e)
		}
	}

	// Find content blocks
	cbsIdx := indexOf(types, "content_block_start")
	cbdIdx := indexOf(types, "content_block_delta")
	cbStIdx := indexOf(types, "content_block_stop")
	mdIdx := indexOf(types, "message_delta")
	msIdx := indexOf(types, "message_stop")

	if cbsIdx < 0 { t.Error("missing content_block_start") }
	if cbdIdx < 0 { t.Error("missing content_block_delta") }
	if cbStIdx < 0 { t.Error("missing content_block_stop") }
	if mdIdx < 0 { t.Error("missing message_delta") }
	if msIdx < 0 { t.Error("missing message_stop") }

	if cbdIdx < cbsIdx { t.Error("content_block_delta before content_block_start") }
	if cbStIdx < cbdIdx { t.Error("content_block_stop before content_block_delta") }
	if mdIdx < cbStIdx { t.Error("message_delta before content_block_stop") }
	if msIdx < mdIdx { t.Error("message_stop before message_delta") }
}

func TestStreamingE2E_FullEventSequence_ToolCall(t *testing.T) {
	sse := openAIToolCallChunk("c", "gpt-4", 0, "call_1", "tool", `{"a":1}`) +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "tool_calls")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	types := getEventTypes(events)
	expected := []string{"message_start", "ping"}
	for i, e := range expected {
		if types[i] != e {
			t.Errorf("event[%d] = %q, want %q", i, types[i], e)
		}
	}
	if types[len(types)-2] != "message_delta" {
		t.Errorf("second-to-last = %q, want message_delta", types[len(types)-2])
	}
	if types[len(types)-1] != "message_stop" {
		t.Errorf("last = %q, want message_stop", types[len(types)-1])
	}
}

func TestStreamingE2E_FullEventSequence_ThinkingText(t *testing.T) {
	sse := openAIReasoningChunk("c", "gpt-4", "thinking...") +
		openAIChunk("c", "gpt-4", map[string]interface{}{"content": "answer"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	types := getEventTypes(events)

	// Expected: message_start, ping, content_block_start(thinking,0), content_block_delta(thinking),
	//           content_block_stop(0), content_block_start(text,1), content_block_delta(text),
	//           content_block_stop(1), message_delta, message_stop
	cbsEvents := findEventsByType(events, "content_block_start")
	if len(cbsEvents) != 2 {
		t.Fatalf("content_block_start count = %d, want 2", len(cbsEvents))
	}

	// First should be thinking at index 0
	if cbsEvents[0].Data["index"] != float64(0) {
		t.Errorf("first block index = %v, want 0", cbsEvents[0].Data["index"])
	}
	cb0 := cbsEvents[0].Data["content_block"].(map[string]interface{})
	if cb0["type"] != "thinking" {
		t.Errorf("first block type = %v, want thinking", cb0["type"])
	}

	// Second should be text at index 1
	if cbsEvents[1].Data["index"] != float64(1) {
		t.Errorf("second block index = %v, want 1", cbsEvents[1].Data["index"])
	}
	cb1 := cbsEvents[1].Data["content_block"].(map[string]interface{})
	if cb1["type"] != "text" {
		t.Errorf("second block type = %v, want text", cb1["type"])
	}

	// Verify message_stop is last
	if types[len(types)-1] != "message_stop" {
		t.Errorf("last event = %q, want message_stop", types[len(types)-1])
	}

	_ = types
}

func TestStreamingE2E_NoDuplicateMessageStart(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	msEvents := findEventsByType(events, "message_start")
	if len(msEvents) != 1 {
		t.Errorf("message_start count = %d, want 1", len(msEvents))
	}
}

func TestStreamingE2E_NoDuplicateMessageStop(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	mstEvents := findEventsByType(events, "message_stop")
	if len(mstEvents) != 1 {
		t.Errorf("message_stop count = %d, want 1", len(mstEvents))
	}
}

func TestStreamingE2E_MessageStartIDIsUnique(t *testing.T) {
	sse1 := buildOpenAIStream("c1", "gpt-4", "test1", "stop")
	events1, _ := runStreamingTestWithDone(t, sse1, "claude-sonnet-4-6")

	sse2 := buildOpenAIStream("c2", "gpt-4", "test2", "stop")
	events2, _ := runStreamingTestWithDone(t, sse2, "claude-sonnet-4-6")

	id1 := events1[0].Data["message"].(map[string]interface{})["id"].(string)
	id2 := events2[0].Data["message"].(map[string]interface{})["id"].(string)

	if id1 == id2 {
		t.Errorf("message IDs should be unique: %q == %q", id1, id2)
	}
	if !strings.HasPrefix(id1, "msg_") {
		t.Errorf("message ID should start with msg_: %q", id1)
	}
}

// ============================================================================
// Section 12: Content Block Stop Events
// ============================================================================

func TestStreamingE2E_ContentBlockStopIndex(t *testing.T) {
	sse := openAIReasoningChunk("c", "gpt-4", "think") +
		openAIChunk("c", "gpt-4", map[string]interface{}{"content": "answer"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbStopEvents := findEventsByType(events, "content_block_stop")
	if len(cbStopEvents) < 2 {
		t.Fatalf("content_block_stop count = %d, want >= 2", len(cbStopEvents))
	}

	// Thinking stop at index 0
	if cbStopEvents[0].Data["index"] != float64(0) {
		t.Errorf("first stop index = %v, want 0", cbStopEvents[0].Data["index"])
	}
	// Text stop at index 1
	if cbStopEvents[1].Data["index"] != float64(1) {
		t.Errorf("second stop index = %v, want 1", cbStopEvents[1].Data["index"])
	}
}

func TestStreamingE2E_ToolBlockStopIndex(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "Here:"}, "") +
		openAIToolCallChunk("c", "gpt-4", 0, "call_1", "tool_a", `{"x":1}`) +
		openAIToolCallChunk("c", "gpt-4", 1, "call_2", "tool_b", `{"y":2}`) +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "tool_calls")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbStopEvents := findEventsByType(events, "content_block_stop")
	// Should have stops for: text(0), tool_a(1), tool_b(2)
	if len(cbStopEvents) < 3 {
		t.Fatalf("content_block_stop = %d, want >= 3", len(cbStopEvents))
	}

	// Collect all stop indices and verify they contain 0, 1, 2
	stopIndices := map[float64]bool{}
	for _, e := range cbStopEvents {
		idx, ok := e.Data["index"].(float64)
		if ok {
			stopIndices[idx] = true
		}
	}
	for _, expected := range []float64{0, 1, 2} {
		if !stopIndices[expected] {
			t.Errorf("missing content_block_stop for index %v", expected)
		}
	}
}

// ============================================================================
// Section 13: Unicode & Encoding Edge Cases
// ============================================================================

func TestStreamingE2E_UTF8MultiByte(t *testing.T) {
	content := "日本語テスト 🎉 ñ é ü"
	sse := buildOpenAIStream("c", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

func TestStreamingE2E_NullBytesInJSON(t *testing.T) {
	// JSON with escaped characters
	content := `{"escaped": "value with \"quotes\" and \\backslash"}`
	delta := map[string]interface{}{"content": content}
	sse := openAIChunk("c", "gpt-4", delta, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content mismatch")
	}
}

func TestStreamingE2E_VeryLongSingleChunk(t *testing.T) {
	// Single chunk with 5KB content
	content := strings.Repeat("x", 5*1024)
	delta := map[string]interface{}{"content": content}
	sse := openAIChunk("c", "gpt-4", delta, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.Content) != 5*1024 {
		t.Errorf("content length = %d, want %d", len(result.Content), 5*1024)
	}
}

func TestStreamingE2E_JSONInToolArgs(t *testing.T) {
	// Tool args with nested JSON
	args := `{"query":"SELECT * FROM users WHERE name='O\\'Brien'","format":"json"}`
	sse := openAIToolCallChunk("c", "gpt-4", 0, "call_sql", "execute_sql", args) +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "tool_calls")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	if input["query"] == nil {
		t.Error("query should not be nil")
	}
}

func TestStreamingE2E_MarkdownContent(t *testing.T) {
	content := "# Header\n\n- item 1\n- item 2\n\n```go\nfmt.Println(\"hi\")\n```\n"
	sse := buildOpenAIStream("c", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content mismatch")
	}
}

func TestStreamingE2E_TabAndControlChars(t *testing.T) {
	content := "col1\tcol2\tcol3\r\nline2\r\n"
	sse := buildOpenAIStream("c", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content mismatch: got %q", result)
	}
}

// ============================================================================
// Section 14: Concurrent Safety
// ============================================================================

func TestStreamingE2E_ConcurrentStreams(t *testing.T) {
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			content := fmt.Sprintf("stream %d content", idx)
			sse := buildOpenAIStream(fmt.Sprintf("c-%d", idx), "gpt-4", content, "stop")
			_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

			if result == nil {
				errors <- fmt.Errorf("stream %d: nil result", idx)
				return
			}
			if result.Content != content {
				errors <- fmt.Errorf("stream %d: content = %q, want %q", idx, result.Content, content)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// ============================================================================
// Section 15: OpenAI Response Format Variations
// ============================================================================

func TestStreamingE2E_OpenAIDeltaWithRole(t *testing.T) {
	// First chunk often has role: "assistant"
	delta := map[string]interface{}{
		"role":    "assistant",
		"content": "",
	}
	sse := openAIChunk("c", "gpt-4", delta, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{"content": "Hello"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "Hello" {
		t.Errorf("content = %q, want Hello", result)
	}
}

func TestStreamingE2E_OpenAIFinishInDeltaChunk(t *testing.T) {
	// Some providers send finish_reason in the same chunk as content
	delta := map[string]interface{}{
		"content": "final",
	}
	choice := map[string]interface{}{
		"index":         0,
		"delta":         delta,
		"finish_reason": "stop",
	}
	chunk := map[string]interface{}{
		"id":      "c",
		"object":  "chat.completion.chunk",
		"model":   "gpt-4",
		"choices": []interface{}{choice},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\ndata: [DONE]\n\n"

	events, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Content != "final" {
		t.Errorf("content = %q, want final", result.Content)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result.StopReason)
	}

	// Should still have proper event sequence
	lastEvents := findEventsByType(events, "message_stop")
	if len(lastEvents) != 1 {
		t.Error("missing message_stop")
	}
}

func TestStreamingE2E_OpenAICachedTokensInDetails(t *testing.T) {
	delta := map[string]interface{}{}
	choice := map[string]interface{}{
		"index":         0,
		"delta":         delta,
		"finish_reason": "stop",
	}
	chunk := map[string]interface{}{
		"id":      "c",
		"object":  "chat.completion.chunk",
		"model":   "gpt-4",
		"choices": []interface{}{choice},
		"usage": map[string]interface{}{
			"prompt_tokens":     500,
			"completion_tokens": 100,
			"total_tokens":      600,
			"prompt_tokens_details": map[string]interface{}{
				"cached_tokens": 300,
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\ndata: [DONE]\n\n"

	events, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.InputTokens != 500 {
		t.Errorf("input_tokens = %d, want 500", result.InputTokens)
	}

	// Check message_delta for cache info
	mdEvents := findEventsByType(events, "message_delta")
	if len(mdEvents) > 0 {
		usage := mdEvents[0].Data["usage"].(map[string]interface{})
		if usage["cache_read_input_tokens"] != float64(300) {
			t.Errorf("cache_read_input_tokens = %v, want 300", usage["cache_read_input_tokens"])
		}
	}
}

// ============================================================================
// Section 16: Complex Real-world Scenarios
// ============================================================================

func TestStreamingE2E_CodeGeneration(t *testing.T) {
	code := `package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
`
	sse := buildOpenAIStream("c", "gpt-4", code, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != code {
		t.Errorf("content mismatch for code generation")
	}
}

func TestStreamingE2E_ToolCallArrayArgs(t *testing.T) {
	args := `{"files":["/a.go","/b.go","/c.go"],"options":{"verbose":true}}`
	sse := openAIToolCallChunk("c", "gpt-4", 0, "call_arr", "multi_edit", args) +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "tool_calls")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	files, ok := input["files"].([]interface{})
	if !ok || len(files) != 3 {
		t.Errorf("files = %v, want 3-element array", input["files"])
	}
}

func TestStreamingE2E_LargeToolCallArgs(t *testing.T) {
	// Simulate a tool call with large arguments (e.g., Write tool with big file)
	largeContent := strings.Repeat("x", 50000)
	id := "c"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "Writing file."}, "") +
		openAIToolCallChunk(id, model, 0, "call_big", "Write", `{"file_path":"/big.txt","content":"`)

	// Stream args in 1KB chunks
	for i := 0; i < len(largeContent); i += 1024 {
		end := i + 1024
		if end > len(largeContent) {
			end = len(largeContent)
		}
		sse += openAIToolCallChunk(id, model, 0, "", "", largeContent[i:end])
	}

	sse += openAIToolCallChunk(id, model, 0, "", "", `"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	input := tc["input"].(map[string]interface{})
	content := input["content"].(string)
	if len(content) != 50000 {
		t.Errorf("content length = %d, want 50000", len(content))
	}
}

func TestStreamingE2E_MixedEmptyAndNonEmptyContent(t *testing.T) {
	id := "c"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": ""}, "") +
		openAIChunk(id, model, map[string]interface{}{"content": "real"}, "") +
		openAIChunk(id, model, map[string]interface{}{"content": ""}, "") +
		openAIChunk(id, model, map[string]interface{}{"content": " content"}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Empty content chunks should be skipped, only "real" + " content" counted
	expected := "real content"
	if result.Content != expected {
		t.Errorf("content = %q, want %q", result.Content, expected)
	}
}

func TestStreamingE2E_MessageStartEmptyContent(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	startEvents := findEventsByType(events, "message_start")
	if len(startEvents) == 0 {
		t.Fatal("no message_start")
	}
	msg := startEvents[0].Data["message"].(map[string]interface{})
	content := msg["content"]
	contentArr, ok := content.([]interface{})
	if !ok {
		t.Fatalf("content type = %T, want []interface{}", content)
	}
	if len(contentArr) != 0 {
		t.Errorf("initial content = %v, want empty array", contentArr)
	}
}

func TestStreamingE2E_MessageStartNilStopReason(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	startEvents := findEventsByType(events, "message_start")
	msg := startEvents[0].Data["message"].(map[string]interface{})
	if msg["stop_reason"] != nil {
		t.Errorf("initial stop_reason = %v, want nil", msg["stop_reason"])
	}
}

func TestStreamingE2E_MessageStartNilStopSequence(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	startEvents := findEventsByType(events, "message_start")
	msg := startEvents[0].Data["message"].(map[string]interface{})
	if msg["stop_sequence"] != nil {
		t.Errorf("initial stop_sequence = %v, want nil", msg["stop_sequence"])
	}
}

func TestStreamingE2E_MessageDeltaNilStopSequence(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	mdEvents := findEventsByType(events, "message_delta")
	if len(mdEvents) == 0 {
		t.Fatal("no message_delta")
	}
	delta := mdEvents[0].Data["delta"].(map[string]interface{})
	if delta["stop_sequence"] != nil {
		t.Errorf("stop_sequence = %v, want nil", delta["stop_sequence"])
	}
}

// ============================================================================
// Section 17: Additional Edge Cases
// ============================================================================

func TestStreamingE2E_SSEFormatCorrectness(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if len(events) == 0 {
		t.Fatal("no events")
	}

	// Every event must have a type and data
	for i, e := range events {
		if e.EventType == "" {
			t.Errorf("event[%d] has empty type", i)
		}
		if e.Data == nil {
			t.Errorf("event[%d] has nil data", i)
		}
		if e.Data["type"] != e.EventType {
			t.Errorf("event[%d] data.type = %v, event type = %q", i, e.Data["type"], e.EventType)
		}
	}
}

func TestStreamingE2E_ContentBlockDeltaHasIndex(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "hello world", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbdEvents := findEventsByType(events, "content_block_delta")
	for i, e := range cbdEvents {
		if _, ok := e.Data["index"]; !ok {
			t.Errorf("content_block_delta[%d] missing index", i)
		}
	}
}

func TestStreamingE2E_ContentBlockStartHasContentBlock(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbsEvents := findEventsByType(events, "content_block_start")
	for i, e := range cbsEvents {
		if _, ok := e.Data["content_block"]; !ok {
			t.Errorf("content_block_start[%d] missing content_block", i)
		}
	}
}

func TestStreamingE2E_ContentBlockStopHasIndex(t *testing.T) {
	sse := buildOpenAIStream("c", "gpt-4", "test", "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbStopEvents := findEventsByType(events, "content_block_stop")
	for i, e := range cbStopEvents {
		if _, ok := e.Data["index"]; !ok {
			t.Errorf("content_block_stop[%d] missing index", i)
		}
	}
}

func TestStreamingE2E_SSEHeadersSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Stream:    true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "hi"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop") +
		"data: [DONE]\n\n"

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), originalReq, ctx)
	}()
	wg.Wait()

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", w.Header().Get("Cache-Control"))
	}
	if w.Header().Get("X-Accel-Buffering") != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", w.Header().Get("X-Accel-Buffering"))
	}
}

// helper: indexOf returns first index of target in slice, or -1
func indexOf(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

// ============================================================================
// Section 18: Non-Streaming Response Conversion E2E
// Tests ConvertOpenAIToClaudeResponse for protocol compliance
// ============================================================================

func TestStreamingE2E_NonStreamingTextResponse(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:      "chatcmpl-ns-001",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:    "assistant",
					Content: "Hello from non-streaming!",
				},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     50,
			CompletionTokens: 10,
			TotalTokens:      60,
		},
	}

	claudeReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
	}

	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	if claudeResp.ID != "chatcmpl-ns-001" {
		t.Errorf("id = %q, want chatcmpl-ns-001", claudeResp.ID)
	}
	if claudeResp.Type != "message" {
		t.Errorf("type = %q, want message", claudeResp.Type)
	}
	if claudeResp.Role != "assistant" {
		t.Errorf("role = %q, want assistant", claudeResp.Role)
	}
	if claudeResp.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", claudeResp.Model)
	}
	if claudeResp.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", claudeResp.StopReason)
	}
	if len(claudeResp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(claudeResp.Content))
	}
	if claudeResp.Content[0].Type != "text" {
		t.Errorf("content type = %q, want text", claudeResp.Content[0].Type)
	}
	if claudeResp.Content[0].Text != "Hello from non-streaming!" {
		t.Errorf("content text = %q", claudeResp.Content[0].Text)
	}
	if claudeResp.Usage.InputTokens != 50 {
		t.Errorf("input_tokens = %d, want 50", claudeResp.Usage.InputTokens)
	}
	if claudeResp.Usage.OutputTokens != 10 {
		t.Errorf("output_tokens = %d, want 10", claudeResp.Usage.OutputTokens)
	}
}

func TestStreamingE2E_NonStreamingToolCallResponse(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:      "chatcmpl-ns-002",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:    "assistant",
					Content: "I'll read that file.",
					ToolCalls: []models.OpenAIToolCall{
						{
							ID:   "call_read_001",
							Type: "function",
							Function: models.OpenAIFunctionCall{
								Name:      "Read",
								Arguments: `{"file_path":"/src/main.go"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     100,
			CompletionTokens: 20,
		},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	if claudeResp.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", claudeResp.StopReason)
	}

	// Should have text + tool_use blocks
	if len(claudeResp.Content) < 2 {
		t.Fatalf("content blocks = %d, want >= 2", len(claudeResp.Content))
	}

	// Find tool_use block
	var toolBlock *models.ClaudeContentBlock
	for i := range claudeResp.Content {
		if claudeResp.Content[i].Type == "tool_use" {
			toolBlock = &claudeResp.Content[i]
			break
		}
	}
	if toolBlock == nil {
		t.Fatal("no tool_use content block found")
	}
	if toolBlock.ID != "call_read_001" {
		t.Errorf("tool id = %q", toolBlock.ID)
	}
	if toolBlock.Name != "Read" {
		t.Errorf("tool name = %q", toolBlock.Name)
	}
	input := toolBlock.Input
	if input["file_path"] != "/src/main.go" {
		t.Errorf("file_path = %v", input["file_path"])
	}
}

func TestStreamingE2E_NonStreamingMultipleToolCalls(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:      "chatcmpl-ns-003",
		Object:  "chat.completion",
		Model:   "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:    "assistant",
					Content: "",
					ToolCalls: []models.OpenAIToolCall{
						{ID: "call_1", Type: "function", Function: models.OpenAIFunctionCall{Name: "Read", Arguments: `{"file_path":"/a"}`}},
						{ID: "call_2", Type: "function", Function: models.OpenAIFunctionCall{Name: "Read", Arguments: `{"file_path":"/b"}`}},
						{ID: "call_3", Type: "function", Function: models.OpenAIFunctionCall{Name: "Bash", Arguments: `{"command":"diff /a /b"}`}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: models.OpenAIUsage{PromptTokens: 80, CompletionTokens: 30},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	toolBlocks := 0
	for _, cb := range claudeResp.Content {
		if cb.Type == "tool_use" {
			toolBlocks++
		}
	}
	if toolBlocks != 3 {
		t.Errorf("tool_use blocks = %d, want 3", toolBlocks)
	}
}

func TestStreamingE2E_NonStreamingReasoningContent(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:     "chatcmpl-ns-004",
		Object: "chat.completion",
		Model:  "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: models.OpenAIMessage{
					Role:             "assistant",
					Content:          "The answer.",
					ReasoningContent: "Let me think about this step by step.",
				},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{PromptTokens: 60, CompletionTokens: 25},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	// The factory path or legacy path should produce valid response
	if claudeResp == nil {
		t.Fatal("response is nil")
	}
	if claudeResp.Type != "message" {
		t.Errorf("type = %q", claudeResp.Type)
	}
	if claudeResp.Role != "assistant" {
		t.Errorf("role = %q", claudeResp.Role)
	}
	// Should have at least text content
	textFound := false
	for _, cb := range claudeResp.Content {
		if cb.Type == "text" {
			textFound = true
		}
	}
	if !textFound {
		t.Error("no text block found")
	}
}

func TestStreamingE2E_NonStreamingCachedTokens(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:     "chatcmpl-ns-005",
		Object: "chat.completion",
		Model:  "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index:         0,
				Message:       models.OpenAIMessage{Role: "assistant", Content: "ok"},
				FinishReason:  "stop",
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     200,
			CompletionTokens: 10,
			TotalTokens:      210,
			PromptTokensDetails: &models.PromptTokensDetails{
				CachedTokens: 150,
			},
		},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	if claudeResp.Usage.CacheReadInputTokens != 150 {
		t.Errorf("cache_read_input_tokens = %d, want 150", claudeResp.Usage.CacheReadInputTokens)
	}
}

func TestStreamingE2E_NonStreamingNilResponse(t *testing.T) {
	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(nil, claudeReq)

	if claudeResp == nil {
		t.Fatal("response should not be nil")
	}
	if claudeResp.Type != "message" {
		t.Errorf("type = %q", claudeResp.Type)
	}
}

func TestStreamingE2E_NonStreamingEmptyChoices(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:      "chatcmpl-ns-006",
		Model:   "gpt-4",
		Choices: []models.OpenAIChoice{},
		Usage:   models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 0},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	if claudeResp == nil {
		t.Fatal("response should not be nil")
	}
	if claudeResp.Type != "message" {
		t.Errorf("type = %q", claudeResp.Type)
	}
}

func TestStreamingE2E_NonStreamingMaxTokensStop(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:    "chatcmpl-ns-007",
		Model: "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index:         0,
				Message:       models.OpenAIMessage{Role: "assistant", Content: "truncated"},
				FinishReason:  "length",
			},
		},
		Usage: models.OpenAIUsage{PromptTokens: 50, CompletionTokens: 100},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	if claudeResp.StopReason != "max_tokens" {
		t.Errorf("stop_reason = %q, want max_tokens", claudeResp.StopReason)
	}
}

func TestStreamingE2E_NonStreamingFunctionCallStop(t *testing.T) {
	openaiResp := &models.OpenAIResponse{
		ID:    "chatcmpl-ns-008",
		Model: "gpt-4",
		Choices: []models.OpenAIChoice{
			{
				Index:         0,
				Message:       models.OpenAIMessage{Role: "assistant", Content: "calling", ToolCalls: []models.OpenAIToolCall{{ID: "x", Type: "function", Function: models.OpenAIFunctionCall{Name: "f", Arguments: "{}"}}}},
				FinishReason:  "function_call",
			},
		},
		Usage: models.OpenAIUsage{PromptTokens: 30, CompletionTokens: 15},
	}

	claudeReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	claudeResp := ConvertOpenAIToClaudeResponse(openaiResp, claudeReq)

	if claudeResp.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", claudeResp.StopReason)
	}
}

// ============================================================================
// Section 19: More Claude Code Tool Patterns
// ============================================================================

func TestStreamingE2E_ClaudeCodeEditTool(t *testing.T) {
	id := "chatcmpl-500"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "I'll fix that bug."}, "") +
		openAIToolCallChunk(id, model, 0, "call_edit", "Edit", `{"file_path":"/src/bug.go","old_string":"if err != nil","new_string":"if err != nil {"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	if input["file_path"] != "/src/bug.go" {
		t.Errorf("file_path = %v", input["file_path"])
	}
	if input["old_string"] != "if err != nil" {
		t.Errorf("old_string = %v", input["old_string"])
	}
}

func TestStreamingE2E_ClaudeCodeGlobTool(t *testing.T) {
	id := "chatcmpl-501"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_glob", "Glob", `{"pattern":"**/*.go","path":"/src"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	if input["pattern"] != "**/*.go" {
		t.Errorf("pattern = %v", input["pattern"])
	}
}

func TestStreamingE2E_ClaudeCodeMultiEdit(t *testing.T) {
	id := "chatcmpl-502"
	model := "gpt-4"

	sse := openAIChunk(id, model, map[string]interface{}{"content": "Making several changes."}, "") +
		openAIToolCallChunk(id, model, 0, "call_e1", "Edit", `{"file_path":"/a.go","old_string":"v1","new_string":"v2"}`) +
		openAIToolCallChunk(id, model, 1, "call_e2", "Edit", `{"file_path":"/b.go","old_string":"x1","new_string":"x2"}`) +
		openAIToolCallChunk(id, model, 2, "call_e3", "Edit", `{"file_path":"/c.go","old_string":"y1","new_string":"y2"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 3 {
		t.Fatalf("tool_calls = %d, want 3", len(result.ToolCalls))
	}
}

func TestStreamingE2E_ClaudeCodeTodoWrite(t *testing.T) {
	id := "chatcmpl-503"
	model := "gpt-4"

	todos := `[{"subject":"Fix bug","status":"in_progress"},{"subject":"Add tests","status":"pending"}]`
	sse := openAIToolCallChunk(id, model, 0, "call_todo", "TodoWrite", `{"todos":`+todos+`}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	todoList, ok := input["todos"].([]interface{})
	if !ok || len(todoList) != 2 {
		t.Errorf("todos = %v", input["todos"])
	}
}

func TestStreamingE2E_ClaudeCodeBashWithTimeout(t *testing.T) {
	id := "chatcmpl-504"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_bash", "Bash", `{"command":"go build ./...","timeout":60000}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatal("unexpected result")
	}
	input := result.ToolCalls[0]["input"].(map[string]interface{})
	if input["command"] != "go build ./..." {
		t.Errorf("command = %v", input["command"])
	}
	if input["timeout"] != float64(60000) {
		t.Errorf("timeout = %v, want 60000", input["timeout"])
	}
}

// ============================================================================
// Section 20: SSE Format Edge Cases
// ============================================================================

func TestStreamingE2E_WhitespaceVariations(t *testing.T) {
	// Multiple spaces after data:
	chunk := map[string]interface{}{
		"id": "c",
		"choices": []interface{}{
			map[string]interface{}{
				"delta":         map[string]interface{}{"content": "ws"},
				"finish_reason": "stop",
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data:  " + string(data) + "\n\ndata: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "ws" {
		t.Errorf("content = %q, want 'ws'", result)
	}
}

func TestStreamingE2E_NoDoneMarker(t *testing.T) {
	// Stream ends without [DONE], just finishes with stop
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "no done"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "no done" {
		t.Errorf("content = %q, want 'no done'", result)
	}
}

func TestStreamingE2E_OnlyDoneMarker(t *testing.T) {
	sse := "data: [DONE]\n\n"
	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	// Should handle gracefully
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestStreamingE2E_EmptyStream(t *testing.T) {
	sse := ""
	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestStreamingE2E_CommentLines(t *testing.T) {
	// SSE spec allows comment lines starting with :
	chunk := map[string]interface{}{
		"id": "c",
		"choices": []interface{}{
			map[string]interface{}{
				"delta":         map[string]interface{}{"content": "after comment"},
				"finish_reason": "stop",
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := ": this is a comment\ndata: " + string(data) + "\n\ndata: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	// Comments are ignored but shouldn't break parsing
	if result == nil || result.Content != "after comment" {
		t.Errorf("content = %q, want 'after comment'", result)
	}
}

func TestStreamingE2E_MultipleFinishReasons(t *testing.T) {
	// Edge: multiple chunks with finish_reason (should use first)
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{"content": "test"}, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "length")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// First finish_reason wins: stop → end_turn
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result.StopReason)
	}
}

// ============================================================================
// Section 21: Tool Call ID/Name Arriving Separately
// ============================================================================

func TestStreamingE2E_ToolCallIDArrivesFirst(t *testing.T) {
	id := "c"
	model := "gpt-4"

	// OpenAI sometimes sends ID+name in first chunk, args in subsequent
	sse := openAIToolCallChunk(id, model, 0, "call_sep", "my_tool", "") +
		openAIToolCallChunk(id, model, 0, "", "", `{"key":"value"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc["id"] != "toolu_sep" {
		t.Errorf("id = %v", tc["id"])
	}
	if tc["name"] != "my_tool" {
		t.Errorf("name = %v", tc["name"])
	}
	input := tc["input"].(map[string]interface{})
	if input["key"] != "value" {
		t.Errorf("input.key = %v", input["key"])
	}
}

func TestStreamingE2E_ToolCallEmptyNameFirst(t *testing.T) {
	id := "c"
	model := "gpt-4"

	// Edge: args arrive before name is known
	delta := map[string]interface{}{
		"tool_calls": []interface{}{
			map[string]interface{}{
				"index": 0,
				"id":    "",
				"type":  "function",
				"function": map[string]interface{}{
					"name":      "",
					"arguments": `{"partial":`,
				},
			},
		},
	}
	sse := openAIChunk(id, model, delta, "") +
		openAIToolCallChunk(id, model, 0, "call_late", "late_tool", `"data":true}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	// Should handle gracefully even if initial data was incomplete
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ============================================================================
// Section 22: Thinking Block Edge Cases
// ============================================================================

func TestStreamingE2E_ThinkingFollowedByToolOnly(t *testing.T) {
	id := "c"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "I need to use a tool.") +
		openAIToolCallChunk(id, model, 0, "call_th", "search", `{"q":"test"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", result.StopReason)
	}

	// Should have thinking stop event
	cbStops := findEventsByType(events, "content_block_stop")
	thinkingStop := false
	for _, e := range cbStops {
		if e.Data["index"] == float64(0) {
			thinkingStop = true
		}
	}
	if !thinkingStop {
		t.Error("thinking block should have been stopped at index 0")
	}
}

func TestStreamingE2E_LongThinking(t *testing.T) {
	id := "c"
	model := "gpt-4"

	// Many reasoning chunks
	var sse string
	for i := 0; i < 20; i++ {
		sse += openAIReasoningChunk(id, model, fmt.Sprintf("Step %d: thinking... ", i))
	}
	sse += openAIChunk(id, model, map[string]interface{}{"content": "Done."}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != "Done." {
		t.Errorf("content = %q, want 'Done.'", result)
	}

	thinkingDeltas := 0
	for _, e := range findEventsByType(events, "content_block_delta") {
		if delta, ok := e.Data["delta"].(map[string]interface{}); ok {
			if delta["type"] == "thinking_delta" {
				thinkingDeltas++
			}
		}
	}
	if thinkingDeltas != 20 {
		t.Errorf("thinking_delta count = %d, want 20", thinkingDeltas)
	}
}

func TestStreamingE2E_ThinkingBlockStopBeforeText(t *testing.T) {
	id := "c"
	model := "gpt-4"

	sse := openAIReasoningChunk(id, model, "hmm") +
		openAIChunk(id, model, map[string]interface{}{"content": "answer"}, "") +
		openAIChunk(id, model, map[string]interface{}{}, "stop")

	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	types := getEventTypes(events)
	// Find the thinking stop and text start
	thinkingStopIdx := indexOf(types, "content_block_stop")
	textStartIdx := -1
	for i, e := range events {
		if e.EventType == "content_block_start" {
			if cb, ok := e.Data["content_block"].(map[string]interface{}); ok {
				if cb["type"] == "text" {
					textStartIdx = i
					break
				}
			}
		}
	}

	if thinkingStopIdx < 0 || textStartIdx < 0 {
		t.Fatal("missing thinking stop or text start")
	}
	if textStartIdx <= thinkingStopIdx {
		t.Error("text start should come after thinking stop")
	}
}

// ============================================================================
// Section 23: Response Content Fidelity
// ============================================================================

func TestStreamingE2E_JSONContentPreserved(t *testing.T) {
	jsonContent := `{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`
	sse := buildOpenAIStream("c", "gpt-4", jsonContent, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	// Verify the JSON is preserved exactly
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	users := parsed["users"].([]interface{})
	if len(users) != 2 {
		t.Errorf("users count = %d, want 2", len(users))
	}
}

func TestStreamingE2E_XMLContentPreserved(t *testing.T) {
	xmlContent := `<response><status>ok</status><data><item key="value"/></data></response>`
	sse := buildOpenAIStream("c", "gpt-4", xmlContent, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || !strings.Contains(result.Content, "<status>ok</status>") {
		t.Errorf("XML not preserved: %q", result)
	}
}

func TestStreamingE2E_HTMLEntities(t *testing.T) {
	content := "Use &amp; for &, &lt; for <, &gt; for >"
	sse := buildOpenAIStream("c", "gpt-4", content, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || result.Content != content {
		t.Errorf("content = %q, want %q", result, content)
	}
}

func TestStreamingE2E_DeltaTextContentInEvent(t *testing.T) {
	delta := map[string]interface{}{"content": "exact text"}
	sse := openAIChunk("c", "gpt-4", delta, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	cbdEvents := findEventsByType(events, "content_block_delta")
	if len(cbdEvents) == 0 {
		t.Fatal("no content_block_delta events")
	}
	deltaData := cbdEvents[0].Data["delta"].(map[string]interface{})
	if deltaData["text"] != "exact text" {
		t.Errorf("delta text = %v, want 'exact text'", deltaData["text"])
	}
}

func TestStreamingE2E_ToolUseDeltaPartialJSON(t *testing.T) {
	id := "c"
	model := "gpt-4"

	sse := openAIToolCallChunk(id, model, 0, "call_pj", "tool", `{"key":"`) +
		openAIToolCallChunk(id, model, 0, "", "", `value"}`) +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	events, _ := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	// Should have 2 input_json_delta events
	cbdEvents := findEventsByType(events, "content_block_delta")
	jsonDeltas := 0
	for _, e := range cbdEvents {
		if delta, ok := e.Data["delta"].(map[string]interface{}); ok {
			if delta["type"] == "input_json_delta" {
				jsonDeltas++
				if delta["partial_json"] == nil {
					t.Error("input_json_delta missing partial_json")
				}
			}
		}
	}
	if jsonDeltas != 2 {
		t.Errorf("input_json_delta count = %d, want 2", jsonDeltas)
	}
}

// ============================================================================
// Section 24: Real-world Error Scenarios
// ============================================================================

func TestStreamingE2E_OpenAIErrorFormat(t *testing.T) {
	// OpenAI sometimes sends error in the response
	chunk := map[string]interface{}{
		"id":      "c",
		"object":  "chat.completion.chunk",
		"model":   "gpt-4",
		"choices": []interface{}{},
		"error": map[string]interface{}{
			"message": "Rate limit exceeded",
			"type":    "rate_limit_error",
			"code":    "429",
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\ndata: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	// Should handle error gracefully (no panic)
	_ = result
}

func TestStreamingE2E_TruncatedJSON(t *testing.T) {
	// Truncated JSON in data line
	sse := "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"test\"}\n\ndata: [DONE]\n\n"

	_, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	// Should handle gracefully
	_ = result
}

func TestStreamingE2E_ExtremelyLargeSingleChunk(t *testing.T) {
	// 100KB single content chunk
	content := strings.Repeat("A", 100*1024)
	delta := map[string]interface{}{"content": content}
	sse := openAIChunk("c", "gpt-4", delta, "") +
		openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil || len(result.Content) != 100*1024 {
		t.Errorf("content length = %d, want %d", len(result.Content), 100*1024)
	}
}

func TestStreamingE2E_StreamingResultFields(t *testing.T) {
	sse := openAIUsageChunk("c", "gpt-4", 500, 250, "stop")
	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", result.InputTokens)
	}
	if result.OutputTokens != 250 {
		t.Errorf("OutputTokens = %d, want 250", result.OutputTokens)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", result.StopReason)
	}
	if result.ToolCalls == nil {
		t.Error("ToolCalls should be initialized (empty slice)")
	}
}
