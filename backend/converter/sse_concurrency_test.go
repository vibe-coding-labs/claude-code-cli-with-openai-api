package converter

// HTTP-level concurrency stress tests for the streaming pipeline.
//
// Unlike the ResponseRecorder-based E2E tests, these spin up a REAL
// httptest.Server (real http.Server, real chunked transfer over a real
// net.Conn) and drive it with keep-alive HTTP clients that share a Transport —
// mirroring undici's connection reuse, the exact precondition under which
// orphan/interleaved bytes surface as InvalidHTTPResponse. If the sseWriter
// mutex or the synchronous heartbeat Stop failed, chunked framing would
// corrupt and a client would see malformed frames or a wrong status line.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

const streamContent = "The quick brown fox jumps over the lazy dog repeatedly."

// slowStream returns an OpenAI SSE reader that emits content in small deltas
// with a short gap, so several heartbeats fire mid-stream — forcing the
// heartbeat goroutine and the content scanner to contend for the connection.
func slowStream(body string) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		runes := []rune(body)
		for i := 0; i < len(runes); i += 3 {
			end := i + 3
			if end > len(runes) {
				end = len(runes)
			}
			chunk := openAIChunk("c", "gpt-4", map[string]interface{}{"content": string(runes[i:end])}, "")
			if _, err := pw.Write([]byte(chunk)); err != nil {
				return
			}
			time.Sleep(3 * time.Millisecond)
		}
		end := openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop") + "data: [DONE]\n\n"
		pw.Write([]byte(end))
	}()
	return pr
}

// scanSSE reads a full SSE stream and returns the ordered event types and the
// concatenated text-delta content. Any malformed data line or read error is
// reported — those signal chunked-framing corruption.
func scanSSE(r io.Reader) (types []string, content string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var curEvent string
	var sb strings.Builder
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			curEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			var m map[string]interface{}
			if jerr := json.Unmarshal([]byte(data), &m); jerr != nil {
				return nil, "", fmt.Errorf("malformed data line (frame corruption): %v near %q", jerr, data)
			}
			types = append(types, curEvent)
			if curEvent == "content_block_delta" {
				if d, ok := m["delta"].(map[string]interface{}); ok {
					if txt, ok := d["text"].(string); ok {
						sb.WriteString(txt)
					}
				}
			}
			curEvent = ""
		}
	}
	if serr := sc.Err(); serr != nil {
		return nil, "", fmt.Errorf("stream read error (transfer/framing corruption): %w", serr)
	}
	return types, sb.String(), nil
}

// runOneClient posts one streaming request and asserts the response is a
// well-formed, complete SSE stream whose concatenated text equals wantContent.
func runOneClient(t *testing.T, client *http.Client, baseURL, wantContent string) error {
	t.Helper()
	resp, err := client.Post(baseURL+"/v1/messages", "application/json", strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status = %d, want 200", resp.StatusCode)
	}
	types, content, err := scanSSE(resp.Body)
	if err != nil {
		return err
	}
	if len(types) == 0 {
		return fmt.Errorf("no events received")
	}
	if content != wantContent {
		return fmt.Errorf("content length = %d, want %d (frame loss/corruption)", len(content), len(wantContent))
	}
	if types[len(types)-1] != "message_stop" {
		return fmt.Errorf("last event = %q, want message_stop", types[len(types)-1])
	}
	return nil
}

// TestSSE_ConcurrentClients_FrameIntegrity fires 32 concurrent keep-alive
// clients (shared Transport) at the real server. Every response must be a
// clean, complete SSE stream — proving the fix holds under high concurrency.
func TestSSE_ConcurrentClients_FrameIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP stress test in -short mode")
	}
	ensureGinTestMode()
	prev := heartbeatInterval
	heartbeatInterval = 8 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		req := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6", MaxTokens: 4096, Stream: true}
		ConvertOpenAIStreamingToClaude(c, slowStream(streamContent), req, c.Request.Context())
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	const clients = 32
	transport := &http.Transport{
		MaxIdleConns:        clients,
		MaxIdleConnsPerHost: clients,
		IdleConnTimeout:     120 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := runOneClient(t, client, srv.URL, streamContent); err != nil {
				errs <- fmt.Errorf("client %d: %w", id, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestSSE_EarlyDisconnectDoesNotCorruptSubsequentRequests reproduces the
// orphan-bytes scenario: a client disconnects mid-stream, then a subsequent
// request over the pooled keep-alive connection must still parse cleanly.
func TestSSE_EarlyDisconnectDoesNotCorruptSubsequentRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping disconnect test in -short mode")
	}
	ensureGinTestMode()
	prev := heartbeatInterval
	heartbeatInterval = 5 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		req := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6", MaxTokens: 4096, Stream: true}
		ConvertOpenAIStreamingToClaude(c, slowStream(strings.Repeat("x", 2000)), req, c.Request.Context())
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	transport := &http.Transport{MaxIdleConnsPerHost: 4, IdleConnTimeout: 120 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	// Request 1: disconnect after reading a few bytes (client gone mid-stream).
	ctx, cancel := context.WithCancel(context.Background())
	req1, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/messages", strings.NewReader("{}"))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("req1 failed: %v", err)
	}
	io.CopyN(io.Discard, resp1.Body, 64)
	cancel()
	resp1.Body.Close()

	// Let the server observe the broken pipe and stop writing (closed flag).
	time.Sleep(50 * time.Millisecond)

	// Request 2: must succeed cleanly. Orphan ping bytes after req1's
	// terminator would surface as a framing error or wrong status line here.
	if err := runOneClient(t, client, srv.URL, strings.Repeat("x", 2000)); err != nil {
		t.Fatalf("request after disconnect was not clean: %v", err)
	}
}
