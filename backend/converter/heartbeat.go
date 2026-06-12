package converter

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// heartbeatInterval is the gap between keep-alive ping events during a stream.
// It defaults to 5s; tests lower it (then restore via t.Cleanup) to exercise
// the heartbeat goroutine without long waits — see sse_writer_race_test.go and
// sse_concurrency_test.go.
var heartbeatInterval = 5 * time.Second

// Heartbeat coordinates periodic ping events. The goroutine stops on ctx
// cancel, an explicit Stop, or the first ping write failure (client gone).
type Heartbeat struct {
	stop chan struct{} // closed by Stop to request termination
	done chan struct{} // closed by the goroutine once it has fully exited
}

// StartHeartbeat sends periodic ping events to keep the connection alive,
// preventing proxy/CDN timeouts during long-running operations. Writes go
// through the serialized, error-aware sendSSE (see sse_utils.go), so a failed
// ping (client disconnected) stops the heartbeat immediately.
func StartHeartbeat(c *gin.Context, ctx context.Context, interval time.Duration) *Heartbeat {
	hb := &Heartbeat{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	go func() {
		defer close(hb.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-hb.stop:
				return
			case <-ticker.C:
				// sendSSE is serialized + error-aware: if the ping write fails
				// the client has disconnected, so stop pinging immediately.
				if !sendSSE(c, models.EventPing, map[string]interface{}{
					"type": models.EventPing,
				}) {
					return
				}
			}
		}
	}()

	return hb
}

// Stop signals the heartbeat to stop and BLOCKS until the goroutine has fully
// exited, guaranteeing no in-flight ping write remains. Idempotent.
//
// Call before emitting the terminal SSE events (message_delta / message_stop)
// so no ping can be written after the response is finalized — such orphan
// bytes on a keep-alive connection are read as the next response's status
// line by undici, producing InvalidHTTPResponse.
func (hb *Heartbeat) Stop() {
	if hb == nil {
		return
	}
	select {
	case <-hb.stop:
		// already stopped / stopping
	default:
		close(hb.stop)
	}
	<-hb.done
}
