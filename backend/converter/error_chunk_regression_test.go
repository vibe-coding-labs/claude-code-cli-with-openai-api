package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestStreamingE2E_ErrorChunkNumericCode_Surfaced guards two real-world bugs
// observed live against the lant relay provider (RELAY_101, insufficient
// balance): the upstream sends a mid-stream SSE "data:" line that is a bare
// {"error":{...}} object (no "choices" field at all), and its error.code is
// a JSON number (e.g. 403) even though OpenAIAPIError.Code used to be typed
// as a plain Go string.
//
// Before the fix, this compounded into total silent data loss:
//  1. json.Unmarshal failed outright on the whole chunk ("cannot unmarshal
//     number into Go struct field OpenAIAPIError.error.code of type
//     string"), so it was logged as a generic parse error and skipped.
//  2. Even had it parsed, the old code had no chunk.Error handling at all —
//     a successfully parsed error-only chunk (len(Choices)==0) was silently
//     `continue`d past.
//
// The end result: the client received what looked like a normal, empty
// end_turn completion instead of the real upstream error, making a genuine
// billing problem invisible and undiagnosable from the Claude Code side.
func TestStreamingE2E_ErrorChunkNumericCode_Surfaced(t *testing.T) {
	sse := `data: {"id":"e1","model":"glm-5.3","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n" +
		`data: {"error":{"code":403,"message":"余额或额度不足：余额或订阅额度不足，请充值 [RELAY_101]","type":"api_error"}}` + "\n\n"

	events, result := runStreamingTest(t, sse, "claude-sonnet-4-6")

	if result != nil {
		t.Errorf("result = %+v, want nil on the error-chunk abort path", result)
	}

	hasError := false
	for _, ev := range events {
		if ev.EventType != "error" {
			continue
		}
		hasError = true
		errData, _ := ev.Data["error"].(map[string]interface{})
		errType, _ := errData["type"].(string)
		errMsg, _ := errData["message"].(string)
		// Insufficient balance is a hard, non-retryable failure — must NOT be
		// classified as overloaded_error (Claude Code would just burn retries
		// against a provider that will never succeed).
		if errType != "api_error" {
			t.Errorf("error.type = %q, want api_error (insufficient balance is not transient)", errType)
		}
		if !strings.Contains(errMsg, "余额或额度不足") {
			t.Errorf("error.message = %q, want it to contain the upstream's actual diagnostic text instead of being dropped", errMsg)
		}
		break
	}
	if !hasError {
		t.Fatal("missing error event — upstream error chunk was silently swallowed")
	}

	// The stream must still end with the full terminal sequence, or Claude
	// Code treats it as a hard-truncated session instead of a clean failure.
	types := getEventTypes(events)
	if last := types[len(types)-1]; last != "message_stop" {
		t.Errorf("last event = %q, want message_stop (stream must not be truncated)", last)
	}
}

// TestOpenAIAPIError_CodeAcceptsStringOrNumber is a focused unit test on the
// JSON-tolerance fix itself, independent of the streaming pipeline.
func TestOpenAIAPIError_CodeAcceptsStringOrNumber(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"numeric code", `{"error":{"code":403,"message":"m","type":"api_error"}}`, "403"},
		{"string code", `{"error":{"code":"invalid_api_key","message":"m","type":"api_error"}}`, "invalid_api_key"},
		{"missing code", `{"error":{"message":"m","type":"api_error"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp models.OpenAIResponse
			if err := json.Unmarshal([]byte(tc.json), &resp); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if resp.Error == nil {
				t.Fatal("Error is nil")
			}
			if got := string(resp.Error.Code); got != tc.want {
				t.Errorf("Code = %q, want %q", got, tc.want)
			}
		})
	}
}
