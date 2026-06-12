package converter

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
)

const sseWriterKey = "__claudeProxySSEWriter__"

// sseWriter serializes every SSE write to one connection and remembers whether
// the connection is still writable.
//
// Why this exists: http.ResponseWriter (and Gin's wrapper, and the underlying
// *bufio.Writer) is NOT safe for concurrent use. Previously the streaming
// scanner goroutine and the heartbeat goroutine both called c.Writer.Write /
// Flush at the same time. Interleaved writes corrupted the chunked-transfer
// framing, so the client (undici) received malformed HTTP and threw
// InvalidHTTPResponse. The mutex makes all writes sequential; the closed flag
// stops further writes the moment a write fails (client gone), preventing
// orphan SSE bytes from polluting the next keep-alive response.
type sseWriter struct {
	c      *gin.Context
	mu     sync.Mutex
	closed bool
}

// bindSSEWriter attaches a serialized writer to the request. Call exactly once
// at the start of every streaming response, before any SSE event is emitted
// and before the heartbeat goroutine starts.
func bindSSEWriter(c *gin.Context) *sseWriter {
	w := &sseWriter{c: c}
	c.Set(sseWriterKey, w)
	return w
}

// getSSEWriter returns the writer bound to the request, or nil if none.
func getSSEWriter(c *gin.Context) *sseWriter {
	if v, ok := c.Get(sseWriterKey); ok {
		if w, _ := v.(*sseWriter); w != nil {
			return w
		}
	}
	return nil
}

// write emits one SSE event under the lock. Returns false when the connection
// is broken or the stream has been marked closed; callers must stop writing.
func (w *sseWriter) write(event string, data interface{}) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		w.closed = true
		return false
	}
	if _, err := w.c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData)))); err != nil {
		w.closed = true
		return false
	}
	// http.Flusher.Flush returns no error; the Write above is the failure signal.
	w.c.Writer.Flush()
	return true
}

// sseWriteClosed reports whether the bound writer has given up (a write
// failed). The streaming scanner uses it to stop consuming upstream data once
// the client is gone. Returns false when no writer is bound.
func sseWriteClosed(c *gin.Context) bool {
	w := getSSEWriter(c)
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

// sendSSE sends a Server-Sent Event. Thread-safe when a writer is bound.
// Returns false if the connection is broken (caller should stop writing).
func sendSSE(c *gin.Context, event string, data interface{}) bool {
	if w := getSSEWriter(c); w != nil {
		return w.write(event, data)
	}
	// Fallback (no bound writer, e.g. a unit-test path): legacy direct write,
	// still reporting success/failure so callers can react.
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err := c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData)))); err != nil {
		return false
	}
	// http.Flusher.Flush returns no error; the Write above is the failure signal.
	c.Writer.Flush()
	return true
}

// sendSSEError sends an error event via SSE. Returns false if the write failed.
func sendSSEError(c *gin.Context, errorType, message string) bool {
	return sendSSE(c, "error", map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	})
}
