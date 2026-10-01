package client

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// extractFinishReasons scans a Chat Completions SSE stream produced by
// convertResponsesStreamingToChat and returns every non-empty finish_reason
// it finds, in emission order.
func extractFinishReasons(t *testing.T, sse string) []string {
	t.Helper()
	var reasons []string
	for _, line := range strings.Split(sse, "\n") {
		line = strings.TrimPrefix(line, "data: ")
		if line == "" || line == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.FinishReason != "" {
				reasons = append(reasons, c.FinishReason)
			}
		}
	}
	return reasons
}

// TestConvertResponsesStreamingToChat_ToolCallGetsToolCallsFinishReason is a
// regression test: the Responses API's top-level response.completed status is
// "completed" even when the model's turn ended in a function call (there is no
// distinct "tool_calls" status), so finish_reason must be derived from whether
// any function_call was actually streamed, not from status alone. Providers
// hit by this bug (upstream_endpoint="responses": apibest gpt-6.1-sol,
// bin-cc-luna, opencode-cc) previously reported finish_reason="stop" here,
// which the downstream converter maps to Anthropic stop_reason "end_turn" even
// though a tool_use content block was emitted — breaking the Claude Code
// agentic tool-call loop.
func TestConvertResponsesStreamingToChat_ToolCallGetsToolCallsFinishReason(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","call_id":"call_1","delta":"{\"city\":\"Beijing\"}"}`,
		`data: {"type":"response.function_call_arguments.done","call_id":"call_1"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":10,"output_tokens":5}}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "gpt-6.1-sol"); err != nil {
		t.Fatalf("convertResponsesStreamingToChat error: %v", err)
	}

	reasons := extractFinishReasons(t, out.String())
	if len(reasons) == 0 {
		t.Fatalf("expected a finish_reason chunk, got none. Full output:\n%s", out.String())
	}
	for _, r := range reasons {
		if r != "tool_calls" {
			t.Errorf("expected finish_reason=tool_calls for a stream containing a function call, got %q. Full output:\n%s", r, out.String())
		}
	}
}

// TestConvertResponsesStreamingToChat_TextOnlyGetsStopFinishReason ensures the
// fix above does not regress the plain text-completion case.
func TestConvertResponsesStreamingToChat_TextOnlyGetsStopFinishReason(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":3,"output_tokens":1}}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "gpt-6.1-sol"); err != nil {
		t.Fatalf("convertResponsesStreamingToChat error: %v", err)
	}

	reasons := extractFinishReasons(t, out.String())
	if len(reasons) == 0 {
		t.Fatalf("expected a finish_reason chunk, got none. Full output:\n%s", out.String())
	}
	for _, r := range reasons {
		if r != "stop" {
			t.Errorf("expected finish_reason=stop for a text-only stream, got %q. Full output:\n%s", r, out.String())
		}
	}
}

// TestConvertResponsesStreamingToChat_ToolCallFinishReasonEmittedWithoutUsage
// regression-tests a second bug found alongside the one above: the
// finish_reason chunk used to be gated on the upstream having sent a usage
// block in response.completed, so an upstream that omits usage would drop the
// finish_reason entirely, leaving the downstream converter to fall back to a
// generic default.
func TestConvertResponsesStreamingToChat_ToolCallFinishReasonEmittedWithoutUsage(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","call_id":"call_1","delta":"{\"city\":\"Tokyo\"}"}`,
		`data: {"type":"response.function_call_arguments.done","call_id":"call_1"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "gpt-6.1-sol"); err != nil {
		t.Fatalf("convertResponsesStreamingToChat error: %v", err)
	}

	reasons := extractFinishReasons(t, out.String())
	if len(reasons) == 0 {
		t.Fatalf("expected a finish_reason chunk even without usage, got none. Full output:\n%s", out.String())
	}
	if reasons[0] != "tool_calls" {
		t.Errorf("expected finish_reason=tool_calls, got %q", reasons[0])
	}
}

// TestConvertResponsesStreamingToChat_ResponseFailedIsError ensures the
// response.failed path (a genuine upstream error, not a tool call) still
// reports finish_reason=error and is not overridden by the tool-call fix.
func TestConvertResponsesStreamingToChat_ResponseFailedIsError(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.failed"}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "gpt-6.1-sol"); err != nil {
		t.Fatalf("convertResponsesStreamingToChat error: %v", err)
	}

	reasons := extractFinishReasons(t, out.String())
	if len(reasons) == 0 || reasons[0] != "error" {
		t.Fatalf("expected finish_reason=error, got %v. Full output:\n%s", reasons, out.String())
	}
}
