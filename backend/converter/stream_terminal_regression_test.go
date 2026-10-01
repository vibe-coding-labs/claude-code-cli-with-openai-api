package converter

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestClaudeTruncatedStreamHasNoSuccessTerminal(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"
	result := ConvertOpenAIStreamingToClaude(c, strings.NewReader(sse), &models.ClaudeMessagesRequest{Model: "test"}, context.Background())
	if result == nil || result.Error == nil || result.Content != "partial" {
		t.Fatalf("expected partial content and truncation error, got %+v", result)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("missing error event: %s", body)
	}
	if strings.Contains(body, "event: message_stop") || strings.Contains(body, "event: message_delta") {
		t.Fatalf("failed stream emitted success terminal: %s", body)
	}
}
