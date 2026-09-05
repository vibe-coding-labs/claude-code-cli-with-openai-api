package converter

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

// errAfterDataReader yields data then fails with err, simulating an upstream
// connection that breaks mid-stream (scanner.Err() != io.EOF path).
type errAfterDataReader struct {
	data string
	pos  int
	err  error
}

func (r *errAfterDataReader) Read(p []byte) (int, error) {
	if r.pos < len(r.data) {
		n := copy(p, r.data[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, r.err
}

// TestStreamingE2E_UpstreamAbortCompletesProtocol guards the fix in
// finishAbortedStream: when the upstream breaks mid-stream the client must
// still receive the full terminal sequence (error → message_delta →
// message_stop). Before the fix the stream was truncated right after the
// error event, which aborted the Claude Code session instead of letting it
// auto-retry.
func TestStreamingE2E_UpstreamAbortCompletesProtocol(t *testing.T) {
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

	chunk := `data: {"id":"e1","model":"gpt-4","choices":[{"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n"
	reader := &errAfterDataReader{data: chunk, err: io.ErrUnexpectedEOF}

	var result *StreamingResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result = ConvertOpenAIStreamingToClaude(c, reader, originalReq, ctx)
	}()
	wg.Wait()

	events := parseClaudeSSEEvents(w.Body.String())
	if len(events) == 0 {
		t.Fatal("no SSE events emitted")
	}
	types := getEventTypes(events)

	if types[0] != "message_start" {
		t.Errorf("first event = %q, want message_start", types[0])
	}
	if last := types[len(types)-1]; last != "message_stop" {
		t.Errorf("last event = %q, want message_stop (stream must not be truncated)", last)
	}

	hasError := false
	for _, ev := range events {
		if ev.EventType == "error" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("missing error event for the upstream abort")
	}

	// The text block was opened by the first delta, so the abort tail must
	// have closed it — start/stop pairing is an Anthropic SSE invariant.
	starts := countEvents(events, "content_block_start")
	stops := countEvents(events, "content_block_stop")
	if starts != 1 {
		t.Errorf("content_block_start count = %d, want 1", starts)
	}
	if stops != 1 {
		t.Errorf("content_block_stop count = %d, want 1 (block opened by the first delta must be closed)", stops)
	}

	if result != nil {
		t.Errorf("result = %+v, want nil on the abort path", result)
	}
}

// TestStreamingE2E_EmptyFinishNoOrphanBlockStop guards the finish-path fix:
// a stream that ends with only a finish_reason (no content deltas) must not
// emit a content_block_stop without a matching content_block_start.
func TestStreamingE2E_EmptyFinishNoOrphanBlockStop(t *testing.T) {
	sse := openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop")
	events, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")

	if result == nil {
		t.Fatal("result is nil")
	}
	types := getEventTypes(events)
	if types[0] != "message_start" {
		t.Errorf("first event = %q, want message_start", types[0])
	}
	if last := types[len(types)-1]; last != "message_stop" {
		t.Errorf("last event = %q, want message_stop", last)
	}

	starts := countEvents(events, "content_block_start")
	stops := countEvents(events, "content_block_stop")
	if starts != 0 {
		t.Errorf("content_block_start count = %d, want 0 (no content was streamed)", starts)
	}
	if stops != 0 {
		t.Errorf("content_block_stop count = %d, want 0 (orphan stop without start violates SSE pairing)", stops)
	}
}

func countEvents(events []claudeSSEEvent, eventType string) int {
	n := 0
	for _, ev := range events {
		if ev.EventType == eventType {
			n++
		}
	}
	return n
}
