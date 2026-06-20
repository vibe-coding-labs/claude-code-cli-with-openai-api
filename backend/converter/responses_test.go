package converter

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// mustReq builds an OpenAIRequest from a Responses JSON body, failing the test
// on error.
func mustReq(t *testing.T, body string) (*models.OpenAIRequest, map[string]interface{}) {
	t.Helper()
	req, raw, err := ConvertResponsesToOpenAIRequest([]byte(body))
	if err != nil {
		t.Fatalf("ConvertResponsesToOpenAIRequest error: %v", err)
	}
	return req, raw
}

// TestResponsesInputStringAndInstructions covers string input + instructions.
func TestResponsesInputStringAndInstructions(t *testing.T) {
	req, _ := mustReq(t, `{"model":"m","instructions":"be brief","input":"hello"}`)
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "be brief" {
		t.Errorf("system msg wrong: %+v", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "hello" {
		t.Errorf("user msg wrong: %+v", req.Messages[1])
	}
}

// TestResponsesInputTextParts joins input_text/text parts with newlines.
func TestResponsesInputTextParts(t *testing.T) {
	req, _ := mustReq(t, `{"input":[{"type":"message","role":"user","content":[
		{"type":"input_text","text":"hello"},{"type":"text","text":"world"}]}]}`)
	if req.Messages[0].Content != "hello\nworld" {
		t.Errorf("expected joined text, got %v", req.Messages[0].Content)
	}
}

// TestResponsesFunctionCallFlushOrder verifies pending assistant tool_calls are
// flushed before message items and before function_call_output items.
func TestResponsesFunctionCallFlushOrder(t *testing.T) {
	req, _ := mustReq(t, `{"input":[
		{"type":"message","role":"user","content":"list"},
		{"type":"function_call","call_id":"c1","name":"ls","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"f1\nf2"}]}`)
	// Expect: user, assistant(tool_calls=[c1]), tool(c1)
	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(req.Messages), req.Messages)
	}
	if req.Messages[1].Role != "assistant" || len(req.Messages[1].ToolCalls) != 1 {
		t.Errorf("assistant tool_calls wrong: %+v", req.Messages[1])
	} else if req.Messages[1].ToolCalls[0].ID != "c1" || req.Messages[1].ToolCalls[0].Function.Name != "ls" {
		t.Errorf("tool call wrong: %+v", req.Messages[1].ToolCalls[0])
	}
	if req.Messages[2].Role != "tool" || req.Messages[2].ToolCallID != "c1" || req.Messages[2].Content != "f1\nf2" {
		t.Errorf("tool result wrong: %+v", req.Messages[2])
	}
}

// TestResponsesMultipleToolCallsThenOutput flushes 2 calls together.
func TestResponsesMultipleToolCallsThenOutput(t *testing.T) {
	req, _ := mustReq(t, `{"input":[
		{"type":"function_call","call_id":"c1","name":"ls","arguments":"{}"},
		{"type":"function_call","call_id":"c2","name":"pwd","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"/h"}]}`)
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "assistant" || len(req.Messages[0].ToolCalls) != 2 {
		t.Errorf("expected assistant with 2 tool_calls: %+v", req.Messages[0])
	}
}

// TestResponsesTrailingFunctionCall is flushed at end with no following output.
func TestResponsesTrailingFunctionCall(t *testing.T) {
	req, _ := mustReq(t, `{"input":[
		{"type":"message","role":"user","content":"go"},
		{"type":"function_call","call_id":"c1","name":"ls","arguments":"{}"}]}`)
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[1].Role != "assistant" || len(req.Messages[1].ToolCalls) != 1 {
		t.Errorf("trailing tool_call not flushed: %+v", req.Messages[1])
	}
}

// TestResponsesDeveloperRole is coerced to system.
func TestResponsesDeveloperRole(t *testing.T) {
	req, _ := mustReq(t, `{"input":[{"type":"message","role":"developer","content":"be nice"}]}`)
	if req.Messages[0].Role != "system" {
		t.Errorf("developer not coerced to system: %+v", req.Messages[0])
	}
}

// TestResponsesPassthroughFields verifies temperature/top_p/max_output_tokens/
// tool_choice/stream/parallel_tool_calls forward to the upstream request.
func TestResponsesPassthroughFields(t *testing.T) {
	req, _ := mustReq(t, `{"model":"m","stream":true,"temperature":0.5,"top_p":0.9,
		"max_output_tokens":128,"tool_choice":"auto","parallel_tool_calls":false}`)
	if !req.Stream || req.Temperature != 0.5 {
		t.Errorf("stream/temp wrong: %+v", req)
	}
	if req.TopP == nil || *req.TopP != 0.9 {
		t.Errorf("top_p wrong: %v", req.TopP)
	}
	if req.MaxTokens != 128 {
		t.Errorf("max_tokens wrong: %d", req.MaxTokens)
	}
	if req.ToolChoice != "auto" {
		t.Errorf("tool_choice wrong: %v", req.ToolChoice)
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls != false {
		t.Errorf("parallel_tool_calls wrong: %v", req.ParallelToolCalls)
	}
}

// TestResponsesTools covers flat / nested / namespace tool shapes.
func TestResponsesTools(t *testing.T) {
	req, _ := mustReq(t, `{"tools":[
		{"type":"function","name":"flat","description":"d","parameters":{"type":"object"}},
		{"type":"function","function":{"name":"nested"}},
		{"type":"namespace","name":"fs","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]},
		{"type":"web_search"}
	]}`)
	if len(req.Tools) != 3 {
		t.Fatalf("expected 3 tools (web_search ignored), got %d", len(req.Tools))
	}
	if req.Tools[0].Function.Name != "flat" || req.Tools[0].Function.Description != "d" {
		t.Errorf("flat tool wrong: %+v", req.Tools[0])
	}
	if req.Tools[1].Function.Name != "nested" {
		t.Errorf("nested tool wrong: %+v", req.Tools[1])
	}
	if req.Tools[2].Function.Name != "mcp__fs__read" {
		t.Errorf("namespace tool wrong: %+v", req.Tools[2])
	}
}

// TestConvertOpenAIResponseToResponses covers text + tool_calls + status.
func TestConvertOpenAIResponseToResponses(t *testing.T) {
	resp := &models.OpenAIResponse{
		Choices: []models.OpenAIChoice{{
			FinishReason: "stop",
			Message: models.OpenAIMessage{
				Role:    "assistant",
				Content: "pong",
				ToolCalls: []models.OpenAIToolCall{{
					ID:       "c1",
					Type:     "function",
					Function: models.OpenAIFunctionCall{Name: "ls", Arguments: "{\"x\":1}"},
				}},
			},
		}},
		Usage: models.OpenAIUsage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
	}
	out := ConvertOpenAIResponseToResponses(resp, "m", map[string]interface{}{"instructions": "hi"})
	if out["status"] != "completed" {
		t.Errorf("status wrong: %v", out["status"])
	}
	output := out["output"].([]map[string]interface{})
	if len(output) != 2 {
		t.Fatalf("expected 2 output items, got %d", len(output))
	}
	if output[0]["type"] != "message" {
		t.Errorf("first output not message: %v", output[0]["type"])
	}
	if output[1]["type"] != "function_call" || output[1]["call_id"] != "c1" {
		t.Errorf("second output not function_call: %+v", output[1])
	}
	usage := out["usage"].(map[string]interface{})
	if usage["input_tokens"] != 5 || usage["output_tokens"] != 2 || usage["total_tokens"] != 7 {
		t.Errorf("usage wrong: %+v", usage)
	}
	if out["instructions"] != "hi" {
		t.Errorf("instructions not echoed back: %v", out["instructions"])
	}
}

// TestConvertOpenAIResponseToResponsesEmptyChoices still emits an empty message.
func TestConvertOpenAIResponseToResponsesEmptyChoices(t *testing.T) {
	out := ConvertOpenAIResponseToResponses(&models.OpenAIResponse{}, "m", nil)
	output := out["output"].([]map[string]interface{})
	if len(output) != 1 || output[0]["type"] != "message" {
		t.Fatalf("expected one empty message item, got %+v", output)
	}
}

// TestConvertOpenAIResponseToResponsesLengthStatus maps finish_reason "length".
func TestConvertOpenAIResponseToResponsesLengthStatus(t *testing.T) {
	resp := &models.OpenAIResponse{Choices: []models.OpenAIChoice{{FinishReason: "length"}}}
	out := ConvertOpenAIResponseToResponses(resp, "m", nil)
	if out["status"] != "incomplete" {
		t.Errorf("expected incomplete, got %v", out["status"])
	}
}

// ---- streaming tests ----

// sseContext builds a gin context backed by a recorder for streaming tests.
func sseContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	return c, w
}

// parseSSEData extracts the JSON payloads from `data:` lines.
func parseSSEData(t *testing.T, body string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// eventTypes returns the ordered "type" of each SSE payload.
func eventTypes(events []map[string]interface{}) []string {
	types := make([]string, 0, len(events))
	for _, e := range events {
		if t, ok := e["type"].(string); ok {
			types = append(types, t)
		}
	}
	return types
}

func assertMonotonicSeq(t *testing.T, events []map[string]interface{}) {
	t.Helper()
	for i, e := range events {
		seq, ok := e["sequence_number"].(float64)
		if !ok {
			t.Errorf("event %d missing sequence_number: %+v", i, e)
			continue
		}
		if int(seq) != i {
			t.Errorf("event %d has sequence_number %v (concurrency/global bug)", i, seq)
		}
	}
}

func contains(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

// TestStreamingText verifies the full text lifecycle + usage capture.
func TestStreamingText(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"po"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"ng"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	c, w := sseContext(t)
	res := ConvertOpenAIStreamingToResponses(c, strings.NewReader(sse), "m", map[string]interface{}{}, 0)
	if res == nil {
		t.Fatal("streaming result nil")
	}
	if res.Content != "pong" {
		t.Errorf("content wrong: %q", res.Content)
	}
	if res.InputTokens != 5 || res.OutputTokens != 2 {
		t.Errorf("usage wrong: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}
	events := parseSSEData(t, w.Body.String())
	types := eventTypes(events)
	want := []string{"response.created", "response.in_progress", "response.output_item.added",
		"response.content_part.added", "response.output_text.delta", "response.output_text.delta",
		"response.output_text.done", "response.content_part.done", "response.output_item.done",
		"response.completed"}
	if len(types) != len(want) {
		t.Fatalf("event sequence wrong:\ngot:  %v\nwant: %v", types, want)
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("event %d: got %s want %s", i, types[i], w)
		}
	}
	assertMonotonicSeq(t, events)
	// response.completed carries usage
	completed := events[len(events)-1]
	resp := completed["response"].(map[string]interface{})
	usage := resp["usage"].(map[string]interface{})
	if usage["input_tokens"] != float64(5) || usage["output_tokens"] != float64(2) {
		t.Errorf("completed usage wrong: %+v", usage)
	}
}

// TestStreamingToolOnly verifies the message item is omitted on tool-only turns.
func TestStreamingToolOnly(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"ls","arguments":""}}]},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"p\":\"/\"}"}}]},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	c, w := sseContext(t)
	res := ConvertOpenAIStreamingToResponses(c, strings.NewReader(sse), "m", nil, 0)
	if res == nil {
		t.Fatal("result nil")
	}
	events := parseSSEData(t, w.Body.String())
	types := eventTypes(events)
	if contains(types, "response.output_text.delta") || contains(types, "response.content_part.added") {
		t.Errorf("text/content_part events should not appear on tool-only turn: %v", types)
	}
	if !contains(types, "response.function_call_arguments.done") {
		t.Errorf("missing function_call_arguments.done: %v", types)
	}
	// response.completed output has NO message item, only function_call(s).
	// (After the JSON round-trip through SSE, arrays decode to []interface{}.)
	completed := events[len(events)-1]
	resp := completed["response"].(map[string]interface{})
	output := resp["output"].([]interface{})
	if len(output) != 1 {
		t.Fatalf("expected single output item, got %d", len(output))
	}
	first := output[0].(map[string]interface{})
	if first["type"] != "function_call" {
		t.Errorf("expected function_call output, got %v", first["type"])
	}
	if first["arguments"] != `{"p":"/"}` {
		t.Errorf("tool arguments wrong: %v", first["arguments"])
	}
	assertMonotonicSeq(t, events)
}

// TestStreamingErrorChunk emits response.failed on an upstream error chunk.
func TestStreamingErrorChunk(t *testing.T) {
	sse := `data: {"error":{"message":"rate limited","type":"rate_limit"}}` + "\n\n"
	c, w := sseContext(t)
	ConvertOpenAIStreamingToResponses(c, strings.NewReader(sse), "m", nil, 0)
	events := parseSSEData(t, w.Body.String())
	types := eventTypes(events)
	if !contains(types, "response.failed") {
		t.Errorf("expected response.failed, got %v", types)
	}
}

// TestSequenceNumberIndependentAcrossInvocations guards against a global seq.
func TestSequenceNumberIndependentAcrossInvocations(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"x"},"finish_reason":null}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" + `data: [DONE]` + "\n\n"
	done := make(chan struct{})
	go func() {
		c, _ := sseContext(t)
		ConvertOpenAIStreamingToResponses(c, strings.NewReader(sse), "m", nil, 0)
		close(done)
	}()
	c, w := sseContext(t)
	ConvertOpenAIStreamingToResponses(c, strings.NewReader(sse), "m", nil, 0)
	<-done
	events := parseSSEData(t, w.Body.String())
	// Both invocations must start sequence_number at 0.
	if len(events) == 0 {
		t.Fatal("no events")
	}
	if seq, _ := events[0]["sequence_number"].(float64); seq != 0 {
		t.Errorf("first event sequence_number not 0 (shared global?): %v", seq)
	}
}
