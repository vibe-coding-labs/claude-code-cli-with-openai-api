package converter

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
