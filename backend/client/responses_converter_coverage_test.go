package client

// Coverage tests for responses_converter.go — these exercise every remaining
// executable branch in the protocol-conversion functions (task #2: reach 100%
// statement coverage). They intentionally probe malformed / duplicate /
// stream-aborted inputs that the happy-path tests in responses_converter_test.go
// and responses_converter_extra_test.go do not produce.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// errWriter always fails on Write, forcing writeSSE's fmt.Fprintf error branch.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, errors.New("simulated write failure") }

// ---------------------------------------------------------------------
// writeSSE (package-level, extracted so marshal/writer failures are reachable)
// ---------------------------------------------------------------------

func TestWriteSSE_MarshalError(t *testing.T) {
	// map[string]interface{} containing a chan cannot be JSON-encoded.
	err := writeSSE(io.Discard, map[string]interface{}{"bad": make(chan int)})
	if err == nil {
		t.Fatal("expected json.Marshal error for unserializable payload")
	}
}

func TestWriteSSE_WriterError(t *testing.T) {
	err := writeSSE(errWriter{}, map[string]interface{}{"ok": true})
	if err == nil {
		t.Fatal("expected fmt.Fprintf error when writer fails")
	}
	if !strings.Contains(err.Error(), "simulated write failure") {
		t.Errorf("expected writer error to surface, got %v", err)
	}
}

// ---------------------------------------------------------------------
// convertResponsesStreamingToChat — edge branches
// ---------------------------------------------------------------------

// "data:" without the following space (the second switch arm), a malformed JSON
// chunk that must be skipped, an empty comment line, and a bare "[DONE]" that
// stops the loop before any completed event.
func TestConvertResponsesStreamingToChat_DataNoSpaceAndMalformedAndDone(t *testing.T) {
	upstream := strings.Join([]string{
		`data:{"type":"response.output_text.delta","delta":"tight"}`,
		`: comment line skipped`,
		`garbage: line hits the default continue`,
		`data: {not-valid-json`,
		`data: [DONE]`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "tight") {
		t.Errorf("expected the no-space data: chunk to be converted, got %s", out.String())
	}
	// Loop broke on [DONE] with no completed: the fallback finish_reason chunk
	// must still be emitted.
	reasons := extractFinishReasons(t, out.String())
	if len(reasons) != 1 || reasons[0] != "stop" {
		t.Errorf("expected single fallback finish_reason=stop, got %v", reasons)
	}
}

// Stream that never sends response.completed: loop exits on EOF, the
// finishReason=="" -> "stop" default and the final choice emission run.
func TestConvertResponsesStreamingToChat_NoCompletedFallbackStop(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reasons := extractFinishReasons(t, out.String())
	if len(reasons) != 1 || reasons[0] != "stop" {
		t.Errorf("expected fallback finish_reason=stop, got %v. Output:\n%s", reasons, out.String())
	}
}

// Tool call announced but arguments.delta omits call_id (opencode.ai behavior):
// the fragment must be appended to the most recent tracked call.
func TestConvertResponsesStreamingToChat_ArgumentsDeltaNoCallIDAppendsToLast(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","delta":"{\"city\":\"NYC\"}"}`,
		`data: {"type":"response.function_call_arguments.done"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), `"name":"get_weather"`) ||
		!strings.Contains(out.String(), `{\"city\":\"NYC\"}`) {
		t.Errorf("expected full tool call emitted with accumulated arguments, got %s", out.String())
	}
	reasons := extractFinishReasons(t, out.String())
	if len(reasons) == 0 || reasons[0] != "tool_calls" {
		t.Errorf("expected finish_reason=tool_calls, got %v", reasons)
	}
}

// arguments.delta arriving with no prior output_item.added: creates a fresh
// toolCall entry keyed by the call_id.
func TestConvertResponsesStreamingToChat_ArgumentsDeltaCreatesFresh(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.function_call_arguments.delta","call_id":"orphan","delta":"{\"x\":1}"}`,
		`data: {"type":"response.function_call_arguments.done","call_id":"orphan"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), `"arguments":"{\"x\":1}"`) {
		t.Errorf("expected fresh tool call with arguments emitted, got %s", out.String())
	}
}

// Two output_item.added events for the same call_id: the second hits the
// found=true branch and updates lastToolIdx without appending a duplicate.
func TestConvertResponsesStreamingToChat_DuplicateOutputItemAdded(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","call_id":"call_1","delta":"{}"}`,
		`data: {"type":"response.function_call_arguments.done","call_id":"call_1"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Dedup must produce exactly one tool_calls delta chunk (the string
	// `"tool_calls":[` appears only inside the delta; "finish_reason":"tool_calls"
	// also contains the substring so we count the bracket form).
	if got := strings.Count(out.String(), `"tool_calls":[`); got != 1 {
		t.Errorf("expected exactly 1 tool_calls chunk after duplicate item.added, got %d:\n%s", got, out.String())
	}
}

// arguments.done for a call_id that was never tracked, with no tracked calls at
// all: targetIdx stays -1 and the done branch breaks out without emitting.
func TestConvertResponsesStreamingToChat_DoneForUntrackedCallID(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.function_call_arguments.done","call_id":"ghost"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.String(), `"tool_calls"`) {
		t.Errorf("expected no tool_calls emission for an untracked done, got %s", out.String())
	}
}

// Tool call whose arguments are streamed but the stream ends without a
// function_call_arguments.done or response.completed event: the EOF fallback
// must still emit the accumulated tool call with finish_reason=tool_calls.
func TestConvertResponsesStreamingToChat_StreamEndFallbackToolCalls(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","call_id":"call_1","delta":"{}"}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reasons := extractFinishReasons(t, out.String())
	if len(reasons) != 1 || reasons[0] != "tool_calls" {
		t.Errorf("expected EOF-fallback finish_reason=tool_calls, got %v. Output:\n%s", reasons, out.String())
	}
	// fullText was never flushed, but the fallback choice carries finish_reason.
}

// response.completed with a full usage block (input + output + total tokens).
// input/output tokens are covered by existing tests; total_tokens is not.
func TestConvertResponsesStreamingToChat_CompletedWithTotalTokens(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(upstream), &out, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), `"total_tokens":7`) {
		t.Errorf("expected total_tokens in the usage chunk, got %s", out.String())
	}
}

// Scanner-level I/O error mid-stream must surface as a wrapped error.
func TestConvertResponsesStreamingToChat_ScannerError(t *testing.T) {
	err := convertResponsesStreamingToChat(&errorReader{}, io.Discard, "m")
	if err == nil {
		t.Fatal("expected scanner error to propagate")
	}
	if !strings.Contains(err.Error(), "responses streaming scanner") {
		t.Errorf("expected wrapped scanner error, got %v", err)
	}
}

// ---------------------------------------------------------------------
// convertResponsesStreamingToChat — writeSSE failure propagation
// ---------------------------------------------------------------------

func TestConvertResponsesStreamingToChat_OutputDeltaWriteError(t *testing.T) {
	upstream := `data: {"type":"response.output_text.delta","delta":"hi"}`
	err := convertResponsesStreamingToChat(strings.NewReader(upstream), errWriter{}, "m")
	if err == nil {
		t.Fatal("expected write error to propagate from output_text.delta")
	}
}

func TestConvertResponsesStreamingToChat_ArgumentsDoneWriteError(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"c1","name":"f"}}`,
		`data: {"type":"response.function_call_arguments.done","call_id":"c1"}`,
		``,
	}, "\n")
	err := convertResponsesStreamingToChat(strings.NewReader(upstream), errWriter{}, "m")
	if err == nil {
		t.Fatal("expected write error to propagate from arguments.done")
	}
}

// Write failure on the response.completed chunk. The stream must NOT contain an
// earlier output_text.delta / arguments.done event, otherwise writeSSE would
// fail there first and never reach the completed branch.
func TestConvertResponsesStreamingToChat_CompletedWriteError(t *testing.T) {
	upstream := `data: {"type":"response.completed","response":{"status":"completed"}}`
	err := convertResponsesStreamingToChat(strings.NewReader(upstream), errWriter{}, "m")
	if err == nil {
		t.Fatal("expected write error to propagate from response.completed")
	}
}

// ---------------------------------------------------------------------
// convertChatToResponsesRequest — non-map tools elements passthrough
// ---------------------------------------------------------------------

func TestConvertChatToResponsesRequest_NonMapToolEntryPassedThrough(t *testing.T) {
	out, err := convertChatToResponsesRequest([]byte(`{"model":"m","tools":["not-a-map",{"type":"function","function":{"name":"f1"}}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	tools := resp["tools"].([]interface{})
	if tools[0] != "not-a-map" {
		t.Errorf("expected non-map tool entry passed through unchanged, got %+v", tools[0])
	}
	if len(tools) != 2 {
		t.Errorf("expected both entries preserved, got %+v", tools)
	}
}