package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestCreateChatCompletionStreamContextCancellationDuring429Backoff(t *testing.T) {
	requestSeen := make(chan struct{}, 1)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"rate limit exceeded"}}`)
		requestSeen <- struct{}{}
	}))
	defer server.Close()

	cfg := &config.Config{
		ConfigID:       fmt.Sprintf("stream-cancel-429-%d", time.Now().UnixNano()),
		OpenAIAPIKey:   "test-key",
		OpenAIBaseURL:  server.URL,
		RequestTimeout: 30,
		RetryCount:     3,
	}
	client := NewOpenAIClient(cfg)
	defer client.httpClient.CloseIdleConnections()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		stream, err := client.CreateChatCompletionStreamContext(ctx, &models.OpenAIRequest{Model: "test-model"})
		if stream != nil {
			_ = stream.Close()
		}
		result <- err
	}()

	select {
	case <-requestSeen:
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for upstream 429 response")
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("CreateChatCompletionStreamContext error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream call did not return after caller cancellation during 429 backoff")
	}

	if got := requestCount.Load(); got != 1 {
		t.Errorf("upstream request count = %d, want 1 after cancellation", got)
	}
}
