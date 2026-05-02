package converter

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// StartHeartbeat sends periodic ping events to keep the connection alive
// This prevents proxy/CDN timeouts during long-running operations
func StartHeartbeat(c *gin.Context, ctx context.Context, interval time.Duration) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-ticker.C:
				// Send ping event to keep connection alive
				sendSSE(c, models.EventPing, map[string]interface{}{
					"type": models.EventPing,
				})
				c.Writer.Flush()
			}
		}
	}()

	return stopChan
}

// StopHeartbeat stops the heartbeat goroutine
func StopHeartbeat(stopChan chan struct{}) {
	if stopChan != nil {
		close(stopChan)
	}
}
