package converter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSendSSE_ConcurrentWritesAreSerialized proves the sseWriter mutex prevents
// concurrent access to c.Writer. Run with -race: a missing lock trips the race
// detector, and interleaved bytes change the total body length.
func TestSendSSE_ConcurrentWritesAreSerialized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	bindSSEWriter(c)

	const goroutines = 8
	const writesPerGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				sendSSE(c, "ping", map[string]string{"type": "ping"})
			}
		}()
	}
	wg.Wait()

	// Each write produces exactly one 35-byte frame:
	// "event: ping\ndata: {\"type\":\"ping\"}\n\n"
	const frameSize = len("event: ping\ndata: {\"type\":\"ping\"}\n\n")
	expected := goroutines * writesPerGoroutine * frameSize
	if recorder.Body.Len() != expected {
		t.Fatalf("body length = %d, want %d (bytes corrupted/lost by concurrent writes)",
			recorder.Body.Len(), expected)
	}
}

// failingResponseWriter is an http.ResponseWriter (+Flusher) whose Write always
// errors, simulating a broken pipe after the client disconnects.
type failingResponseWriter struct{}

func (failingResponseWriter) Header() http.Header       { return http.Header{} }
func (failingResponseWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
func (failingResponseWriter) WriteHeader(int)           {}
func (failingResponseWriter) Flush()                    {}

// TestSendSSE_FailureStopsFurtherWrites proves the closed flag makes every
// write after the first failure a no-op, so a dead connection receives no
// orphan bytes.
func TestSendSSE_FailureStopsFurtherWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(failingResponseWriter{})
	bindSSEWriter(c)

	first := sendSSE(c, "ping", map[string]string{"type": "ping"})
	second := sendSSE(c, "ping", map[string]string{"type": "ping"})

	if first {
		t.Fatalf("first write to broken connection should fail, got success")
	}
	if second {
		t.Fatalf("second write after failure should be a no-op (return false), got success")
	}
}

// TestSSEWriter_Write_MarshalError proves a JSON-marshal failure (e.g. a
// channel value, which encoding/json cannot serialize) marks the writer
// closed and returns false, instead of writing garbage to the connection.
func TestSSEWriter_Write_MarshalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	w := bindSSEWriter(c)

	unmarshalable := map[string]interface{}{"ch": make(chan int)}
	if ok := w.write("ping", unmarshalable); ok {
		t.Fatal("expected write to report failure for an unmarshalable payload")
	}
	if !w.closed {
		t.Error("expected writer to be marked closed after a marshal error")
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("expected no bytes written on marshal error, got %q", recorder.Body.String())
	}
}

// TestSSEWriteClosed proves sseWriteClosed correctly reports both the
// no-writer-bound case and the writer-has-failed case.
func TestSSEWriteClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// No writer bound at all.
	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	if sseWriteClosed(c1) {
		t.Error("expected false when no writer is bound")
	}

	// Writer bound but healthy.
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	bindSSEWriter(c2)
	if sseWriteClosed(c2) {
		t.Error("expected false for a healthy bound writer")
	}

	// Writer bound and failed.
	c3, _ := gin.CreateTestContext(failingResponseWriter{})
	bindSSEWriter(c3)
	sendSSE(c3, "ping", map[string]string{"type": "ping"})
	if !sseWriteClosed(c3) {
		t.Error("expected true after the bound writer's write failed")
	}
}

// TestSendSSE_FallbackNoBoundWriter exercises sendSSE's legacy direct-write
// path, taken when no sseWriter has been bound to the context (e.g. a code
// path that predates bindSSEWriter, or a minimal unit-test context).
func TestSendSSE_FallbackNoBoundWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	// Deliberately do NOT call bindSSEWriter.
	if ok := sendSSE(c, "ping", map[string]string{"type": "ping"}); !ok {
		t.Fatal("expected fallback direct write to succeed")
	}
	want := "event: ping\ndata: {\"type\":\"ping\"}\n\n"
	if recorder.Body.String() != want {
		t.Errorf("body = %q, want %q", recorder.Body.String(), want)
	}

	cFail, _ := gin.CreateTestContext(failingResponseWriter{})
	if ok := sendSSE(cFail, "ping", map[string]string{"type": "ping"}); ok {
		t.Fatal("expected fallback direct write to report failure on a broken connection")
	}
}

// TestSendSSEError proves sendSSEError shapes the payload as a proper SSE
// "error" event carrying the given type/message.
func TestSendSSEError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	bindSSEWriter(c)

	if ok := sendSSEError(c, "overloaded_error", "upstream overloaded"); !ok {
		t.Fatal("expected sendSSEError to succeed")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected an 'error' SSE event, got %q", body)
	}
	if !strings.Contains(body, "overloaded_error") || !strings.Contains(body, "upstream overloaded") {
		t.Errorf("expected error type/message in payload, got %q", body)
	}
}
