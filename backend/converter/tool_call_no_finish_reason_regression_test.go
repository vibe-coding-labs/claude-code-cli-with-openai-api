package converter

// Regression test for a bug found during live multi-provider compatibility
// testing: when an upstream's stream closes without ever sending an explicit
// finish_reason chunk (connection drop right after the tool call completes,
// or a provider quirk that omits it), ConsumeSSEStream's fallback used to
// default state.finalStopReason straight to "end_turn" regardless of whether
// a tool call had actually been collected. That makes the Claude Code
// agentic loop think the assistant's turn is finished and skip executing the
// tool entirely. The fallback must instead default to "tool_use" whenever
// any tool call was captured.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// eofAfterDataReader yields data then a clean io.EOF, simulating an upstream
// that closes the connection normally but without an explicit finish_reason.
type eofAfterDataReader struct {
	data string
	pos  int
}

func (r *eofAfterDataReader) Read(p []byte) (int, error) {
	if r.pos < len(r.data) {
		n := copy(p, r.data[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, io.EOF
}

func TestStreamingE2E_ToolCallWithoutFinishReasonDefaultsToToolUse(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
	}

	// A tool_calls delta with a name and complete arguments, but the stream
	// ends right after — no chunk ever carries a finish_reason.
	chunk := `data: {"id":"e1","model":"gpt-4","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}}]},"finish_reason":null}]}` + "\n\n"
	reader := &eofAfterDataReader{data: chunk}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ConvertOpenAIStreamingToClaude(c, reader, originalReq, ctx)
	}()
	wg.Wait()

	events := parseClaudeSSEEvents(w.Body.String())
	if len(events) == 0 {
		t.Fatal("no SSE events emitted")
	}

	deltas := findEventsByType(events, "message_delta")
	if len(deltas) == 0 {
		t.Fatal("no message_delta event emitted")
	}
	delta, _ := deltas[len(deltas)-1].Data["delta"].(map[string]interface{})
	if delta == nil {
		t.Fatal("message_delta event missing delta field")
	}
	if stopReason, _ := delta["stop_reason"].(string); stopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use (a tool call was collected even though "+
			"the upstream never sent finish_reason before closing the stream); full body:\n%s",
			stopReason, w.Body.String())
	}
}

// TestStreamingE2E_TextOnlyWithoutFinishReasonDefaultsToEndTurn guards against
// over-correcting the fix above: a plain text stream with no tool calls that
// closes without finish_reason must still default to end_turn.
func TestStreamingE2E_TextOnlyWithoutFinishReasonDefaultsToEndTurn(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	originalReq := &models.ClaudeMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Stream:    true,
	}

	chunk := `data: {"id":"e1","model":"gpt-4","choices":[{"delta":{"content":"hello"},"finish_reason":null}]}` + "\n\n"
	reader := &eofAfterDataReader{data: chunk}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ConvertOpenAIStreamingToClaude(c, reader, originalReq, ctx)
	}()
	wg.Wait()

	events := parseClaudeSSEEvents(w.Body.String())
	deltas := findEventsByType(events, "message_delta")
	if len(deltas) == 0 {
		t.Fatal("no message_delta event emitted")
	}
	delta, _ := deltas[len(deltas)-1].Data["delta"].(map[string]interface{})
	if delta == nil {
		t.Fatal("message_delta event missing delta field")
	}
	if stopReason, _ := delta["stop_reason"].(string); stopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn for a text-only stream with no tool calls", stopReason)
	}
}
