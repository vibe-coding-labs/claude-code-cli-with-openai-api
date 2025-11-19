package converter

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

// sendSSE sends a Server-Sent Event
func sendSSE(c *gin.Context, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData))))
	c.Writer.Flush()
}

// sendSSEError sends an error event via SSE
func sendSSEError(c *gin.Context, errorType, message string) {
	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	}
	sendSSE(c, "error", errorEvent)
}
