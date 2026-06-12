package converter

// Race-detection tests for the serialized SSE writer (sse_utils.go) and the
// synchronous heartbeat stop (heartbeat.go). These directly target the
// InvalidHTTPResponse root cause: the heartbeat goroutine and the streaming
// scanner goroutine must never call c.Writer.Write at the same time, and no
// ping may be written after the response is finalized.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ginTestModeOnce ensures gin.SetMode runs exactly once for the whole test
// binary. Previously runStreamingTest called gin.SetMode(gin.TestMode) on every
// invocation, so TestStreamingE2E_ConcurrentStreams (which calls it from 10
// goroutines) raced on gin's global mode variable under -race.
var ginTestModeOnce sync.Once

// ensureGinTestMode sets gin to TestMode exactly once. Use this instead of a
// bare gin.SetMode in every test/helper.
func ensureGinTestMode() {
	ginTestModeOnce.Do(func() { gin.SetMode(gin.TestMode) })
}

// raceDetectingWriter is an http.ResponseWriter whose Write detects overlapping
// calls via an atomic in-flight counter. Two concurrent Writes (heartbeat +
// content writer without the sseWriter mutex) increment overlapping. With the
// mutex in place, Writes are fully serialized and overlapping stays 0.
type raceDetectingWriter struct {
	header      http.Header
	inFlight    int32 // accessed atomically — concurrent writers currently inside Write
	overlapping int32 // accessed atomically — number of detected concurrent-Write overlaps
	mu          sync.Mutex
	buf         bytes.Buffer
}

func newRaceDetectingWriter() *raceDetectingWriter {
	return &raceDetectingWriter{header: http.Header{}}
}

func (w *raceDetectingWriter) Header() http.Header { return w.header }

func (w *raceDetectingWriter) Write(b []byte) (int, error) {
	// A second writer entering while the first is still inside Write means two
	// goroutines hit the connection at once — the exact root cause.
	if atomic.AddInt32(&w.inFlight, 1) > 1 {
		atomic.AddInt32(&w.overlapping, 1)
	}
	// A tiny hold widens the race window so overlap is reliably observable on
	// fast hardware, not just under -race.
	time.Sleep(time.Microsecond)
	atomic.AddInt32(&w.inFlight, -1)

	w.mu.Lock()
	w.buf.Write(b)
	w.mu.Unlock()
	return len(b), nil
}

func (w *raceDetectingWriter) WriteHeader(int) {}
func (w *raceDetectingWriter) Flush()          {}

// countPings reports how many ping events were written to buf.
func countPings(s string) int { return strings.Count(s, "event: ping") }

// TestSSE_HeartbeatAndContentAreSerialized proves the sseWriter mutex prevents
// the heartbeat goroutine and a content writer from writing concurrently. If
// the mutex is removed, the race-detecting writer records overlapping writes.
func TestSSE_HeartbeatAndContentAreSerialized(t *testing.T) {
	ensureGinTestMode()
	w := newRaceDetectingWriter()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	bindSSEWriter(c)

	prev := heartbeatInterval
	heartbeatInterval = 2 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hb := StartHeartbeat(c, ctx, heartbeatInterval)

	// Hammer content writes concurrently with the heartbeat for 50ms.
	deadline := time.After(50 * time.Millisecond)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-deadline:
				return
			default:
			}
			sendSSE(c, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": fmt.Sprintf("d%d", i),
				},
			})
		}
	}()
	wg.Wait()
	hb.Stop()

	if n := atomic.LoadInt32(&w.overlapping); n != 0 {
		t.Fatalf("detected %d concurrent writes to the connection — heartbeat and content writer are not serialized", n)
	}
}

// TestHeartbeat_StopIsSynchronousAndTerminal proves Heartbeat.Stop() blocks
// until the goroutine has exited AND that no ping is written afterwards.
// Orphan pings after the chunked terminator are read as the next keep-alive
// response's status line by undici → InvalidHTTPResponse.
func TestHeartbeat_StopIsSynchronousAndTerminal(t *testing.T) {
	ensureGinTestMode()
	w := newRaceDetectingWriter()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	bindSSEWriter(c)

	prev := heartbeatInterval
	heartbeatInterval = 5 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hb := StartHeartbeat(c, ctx, heartbeatInterval)

	time.Sleep(20 * time.Millisecond) // let a few pings fire
	hb.Stop()                         // must block until the goroutine exited

	pingsBefore := countPings(w.buf.String())
	time.Sleep(30 * time.Millisecond) // ~6 intervals — no ping should fire now
	pingsAfter := countPings(w.buf.String())

	if pingsAfter != pingsBefore {
		t.Fatalf("heartbeat wrote %d ping(s) after Stop() returned — orphan bytes, Stop is not terminal",
			pingsAfter-pingsBefore)
	}
}
