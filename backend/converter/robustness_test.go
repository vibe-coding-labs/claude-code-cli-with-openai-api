package converter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/types"
)

// --- Panic guard helpers (task: stack + conversion_error attribution) ---

func TestGuardStreamPanic_PushesStackErrorAndRecovers(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	errChan := make(chan error, 1)
	recovered := false
	func() {
		defer func() {
			r := recover() // must be called directly in this deferred frame
			recovered = guardStreamPanic(c, errChan, types.StageStreaming, "test-model", r)
		}()
		panic("boom") //nolint:forbidigo
	}()

	if !recovered {
		t.Fatal("guardStreamPanic should have reported recovery")
	}
	select {
	case err := <-errChan:
		msg := err.Error()
		if !strings.Contains(msg, "conversion panic: boom") {
			t.Errorf("error = %q, want it to include the panic value", msg)
		}
		if !strings.Contains(msg, "goroutine") || !strings.Contains(msg, "panic_guard.go") {
			t.Errorf("error should carry a stack trace for post-mortem, got: %.200s", msg)
		}
	default:
		t.Fatal("guardStreamPanic did not push to errChan on panic")
	}
}

func TestGuardStreamPanic_NoPanic_ClosesCleanly(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	errChan := make(chan error, 1)
	recovered := false
	func() {
		defer func() {
			r := recover() // must be called directly in this deferred frame
			recovered = guardStreamPanic(c, errChan, types.StageStreaming, "test-model", r)
		}()
		// no panic
	}()
	if recovered {
		t.Fatal("guardStreamPanic must report no recovery when the body returns normally")
	}
	if len(errChan) != 0 {
		t.Fatal("guardStreamPanic must not push an error when no panic occurred")
	}
}

func TestGuardConversionPanic_DegradesToFallback(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// A panicking inner func must be caught and replaced by the fallback — the
	// whole proxy must not crash on malformed input.
	got := guardConversionPanic(c, "test-model", func() string { return "fallback" }, func() string {
		panic("malformed payload") //nolint:forbidigo
	})
	if got != "fallback" {
		t.Errorf("got %q, want fallback (panic must degrade, not crash)", got)
	}
}

func TestGuardConversionPanic_SuccessPath(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	got := guardConversionPanic(c, "test-model", func() string { return "fallback" }, func() string { return "ok" })
	if got != "ok" {
		t.Errorf("got %q, want ok (success path must pass through untouched)", got)
	}
}

// --- Single-shot response conversion: nil / malformed input must not crash ---

func TestConvertOpenAIToClaudeResponse_NilResponseDegradesGracefully(t *testing.T) {
	ensureGinTestMode()
	orig := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	resp := ConvertOpenAIToClaudeResponse(nil, orig)
	if resp == nil {
		t.Fatal("nil upstream response must yield a fallback Claude response, not nil")
	}
	if resp.Type != "message" {
		t.Errorf("fallback type = %q, want message", resp.Type)
	}
	if resp.Content == nil {
		t.Error("fallback must carry a (possibly empty) content block")
	}
}

// --- AbortSSEStream keeps the protocol well-formed (0% before this test) ---

func TestAbortSSEStream_CompletesProtocol(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	orig := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6", Stream: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	state, hb := PrepareSSEStream(c, ctx, orig, nil)
	// Simulate a block already opened (e.g. a delimited text block started before
	// the upstream errored) so the abort must close it — pairing invariant. Emit
	// a real content_block_start to the wire so start/stop pairing is genuine.
	if !sendSSE(c, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]interface{}{"type": "text", "text": ""},
	}) {
		t.Fatal("could not write content_block_start")
	}
	state.sentContentBlockStart = true
	state.sentContentBlockFinish = false
	state.currentBlockType = BlockText

	AbortSSEStream(c, state, hb, "api_error", "forced")

	types := getEventTypes(parseClaudeSSEEvents(w.Body.String()))
	if len(types) < 2 {
		t.Fatalf("abort emitted %d event(s), want full terminal sequence", len(types))
	}
	if last := types[len(types)-1]; last != "message_stop" {
		t.Errorf("last event = %q, want message_stop (abort must not truncate the stream)", last)
	}
	if has := countEvents(parseClaudeSSEEvents(w.Body.String()), "content_block_start"); has != 1 {
		t.Errorf("content_block_start = %d, want 1", has)
	}
	if has := countEvents(parseClaudeSSEEvents(w.Body.String()), "content_block_stop"); has != 1 {
		t.Errorf("content_block_stop = %d, want 1 (open block must be closed)", has)
	}
}

// --- sendSSE / sendSSEError write well-formed events ---

func TestSendSSE_WritesEvent(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ok := sendSSE(c, "message_start", map[string]interface{}{"type": "message_start", "message": map[string]interface{}{"id": "m1"}})
	if !ok {
		t.Fatal("sendSSE reported failure on a live recorder")
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: message_start") {
		t.Errorf("missing SSE event header, got: %q", body)
	}
	if !strings.Contains(body, `"id":"m1"`) {
		t.Errorf("missing JSON payload, got: %q", body)
	}
}

func TestSendSSEError_WritesErrorEvent(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ok := sendSSEError(c, "overloaded_error", "retry please")
	if !ok {
		t.Fatal("sendSSEError reported failure on a live recorder")
	}
	body := w.Body.String()
	if !strings.Contains(body, `"overloaded_error"`) {
		t.Errorf("error event should carry the error type, got: %q", body)
	}
}

// --- Responses streaming: excessive malformed chunks fail, not fake-success ---

// garbageChunk builds a non-JSON SSE line.
func TestResponsesStream_MalformedChunkCapFailsNotSuccess(t *testing.T) {
	ensureGinTestMode()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("data: this is not json" + "\n\n")
	}
	reqBody := map[string]interface{}{"model": "astro-code-latest"}

	// On the malformed-cap failure the function terminates via the errChan path
	// and returns nil; the interesting contract is the wire event sequence.
	ConvertOpenAIStreamingToResponses(c, strings.NewReader(sb.String()), "astro-code-latest", reqBody, 0)

	body := w.Body.String()
	if !strings.Contains(body, "response.failed") {
		t.Errorf("garbage stream must emit response.failed, got body tail: %.200s", body)
	}
	if strings.Contains(body, "response.completed") {
		t.Errorf("garbage stream must NOT pretend success by emitting response.completed, got: %.200s", body)
	}
}