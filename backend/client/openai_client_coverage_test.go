package client

// Coverage tests for openai_client.go — these drive every remaining uncovered
// branch to reach 100% statement coverage for the client package (task #3).
//
// Strategy highlights:
//   - PROXY_RESPONSE_HEADER_TIMEOUT_SEC=0 shortens retryDeadline's floor to
//     zero so a tiny Timeout triggers the timeout / backoff-deadline branches.
//   - marshalOverride / prepareOverride / wait429Override (test-only seams on
//     OpenAIClient) force deterministic marshal, body-prepare and 429-backoff
//     failures without hostile upstreams.
//   - A stub http.RoundTripper produces mid-stream read errors that a real
//     httptest server cannot emit, driving the pipe CloseWithError branch.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/ratelimit"
)

// ---------------------------------------------------------------------
// upstreamResponseHeaderTimeoutFromEnv
// ---------------------------------------------------------------------

func TestUpstreamResponseHeaderTimeoutFromEnv(t *testing.T) {
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "0")
	if got := upstreamResponseHeaderTimeoutFromEnv(); got != 0 {
		t.Errorf("expected 0s for env=0, got %v", got)
	}
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "120")
	if got := upstreamResponseHeaderTimeoutFromEnv(); got != 120*time.Second {
		t.Errorf("expected 120s for env=120, got %v", got)
	}
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "abc")
	if got := upstreamResponseHeaderTimeoutFromEnv(); got != 600*time.Second {
		t.Errorf("expected 600s fallback for non-numeric env, got %v", got)
	}
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "")
	if got := upstreamResponseHeaderTimeoutFromEnv(); got != 600*time.Second {
		t.Errorf("expected 600s fallback for empty env, got %v", got)
	}
}

// ---------------------------------------------------------------------
// normalizeToolChoiceForRetry — non-function type / empty name branches
// ---------------------------------------------------------------------

func TestNormalizeToolChoiceForRetry_BranchBranches(t *testing.T) {
	body := "tool_choice.name is required when type is function"

	req := &models.OpenAIRequest{ToolChoice: map[string]interface{}{"type": "auto"}}
	if out, changed := normalizeToolChoiceForRetry(req, body); changed || out != req {
		t.Errorf("expected unchanged for non-function type, changed=%v", changed)
	}

	req = &models.OpenAIRequest{ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": ""}}}
	if out, changed := normalizeToolChoiceForRetry(req, body); changed || out != req {
		t.Errorf("expected unchanged for empty name, changed=%v", changed)
	}

	req = &models.OpenAIRequest{ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "f1"}}}
	out, changed := normalizeToolChoiceForRetry(req, body)
	if !changed {
		t.Fatal("expected change for flattenable tool_choice")
	}
	flat, ok := out.ToolChoice.(map[string]interface{})
	if !ok || flat["name"] != "f1" {
		t.Errorf("expected flat name=f1, got %+v", out.ToolChoice)
	}
}

// ---------------------------------------------------------------------
// saveDebugRequest — WriteFile failure branch
// ---------------------------------------------------------------------

func TestSaveDebugRequestWriteFileError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join(".", "data", "debug"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pre-create a DIRECTORY at the exact filename saveDebugRequest will write.
	// os.WriteFile on an existing directory fails — exercising the warn branch.
	// The timestamp is second-granularity, so retry across buckets until the
	// pre-created directory collides with saveDebugRequest's own timestamp.
	for i := 0; i < 10; i++ {
		ts := time.Now().Format("20060102-150405")
		target := filepath.Join(".", "data", "debug", fmt.Sprintf("error-m-%s.json", ts))
		if err := os.Mkdir(target, 0o755); err != nil && !os.IsExist(err) {
			t.Fatalf("mkdir target: %v", err)
		}
		saveDebugRequest("m", []byte(`{"model":"m"}`), "boom")
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			return // WriteFile was blocked by the pre-created directory → branch hit
		}
		// Bucket rolled over: saveDebugRequest wrote a normal file elsewhere.
		// Remove our pre-created directory (now stale) and retry in the next bucket.
		os.RemoveAll(target)
	}
	t.Fatal("never hit the same second bucket to force the write failure")
}

// ---------------------------------------------------------------------
// classifyError — generic-error fallback branch
// ---------------------------------------------------------------------

func TestClassifyErrorGenericErrFallback(t *testing.T) {
	if got := classifyError(0, errors.New("boom")); got != database.ErrorCategoryNetwork {
		t.Errorf("expected generic error -> network category, got %q", got)
	}
}

// ---------------------------------------------------------------------
// assembleStreamToResponse — remaining branch arms
// ---------------------------------------------------------------------

func TestAssembleStreamToResponse_RemainingBranches(t *testing.T) {
	c := &OpenAIClient{}
	// Covers: "data:" with no space (second switch arm), the switch default
	// branch (garbage line), the top-level usage map capture, and the
	// delta==nil continue.
	sse := "garbage-line\n" +
		"data:{\"choices\":[{\"delta\":{\"content\":\"tight\"}}]}\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3},\"choices\":[]}\n\n" +
		"data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	resp, err := c.assembleStreamToResponse(strings.NewReader(sse), &models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "tight" {
		t.Errorf("expected content from no-space data: chunk, got %q", resp.Choices[0].Message.Content)
	}
	if resp.Usage.PromptTokens != 1 || resp.Usage.CompletionTokens != 2 || resp.Usage.TotalTokens != 3 {
		t.Errorf("unexpected top-level usage capture: %+v", resp.Usage)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected default finish_reason=stop, got %q", resp.Choices[0].FinishReason)
	}
}

// ---------------------------------------------------------------------
// NonStream — marshal / prepare failures
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_MarshalFail(t *testing.T) {
	c := newTestClient("http://upstream.invalid")
	c.marshalOverride = func(any) ([]byte, error) { return nil, errors.New("injected marshal failure") }
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to marshal request") {
		t.Fatalf("expected marshal failure, got %v", err)
	}
}

func TestCreateChatCompletionNonStream_PrepareFail(t *testing.T) {
	c := newTestClient("http://upstream.invalid")
	c.prepareOverride = func(*models.OpenAIRequest, []byte) ([]byte, error) {
		return nil, errors.New("injected prepare failure")
	}
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to prepare request body") {
		t.Fatalf("expected prepare failure, got %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — timeout / backoff-deadline / new-request branches
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_TimeoutImmediate(t *testing.T) {
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "0")
	c := newTestClient("http://upstream.invalid")
	c.Timeout = 1 // ns
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "request timeout exceeded") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestCreateChatCompletionNonStream_BackoffExceedsDeadlineBreak(t *testing.T) {
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 503, `{"error":"upstream overloaded"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.Timeout = 200 * time.Millisecond
	c.RetryCount = 1
	c.RetryBackoffBase = time.Hour // attempt 1 backoff = 1h, far past the deadline
	c.RetryBackoffMax = time.Hour // must not cap the 1h backoff to the default 10ms
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "all retry attempts failed") {
		t.Fatalf("expected backoff-abort error, got %v", err)
	}
}

func TestCreateChatCompletionNonStream_NewRequestError(t *testing.T) {
	c := newTestClient("http://%zz")
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to create request") {
		t.Fatalf("expected URL parse error, got %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — request header branches
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_HeadersFull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api-version"); got != "2024-06-01" {
			t.Errorf("expected api-version query param, got %q", got)
		}
		if got := r.Header.Get("X-Custom-Test"); got != "custom-value" {
			t.Errorf("expected custom header, got %q", got)
		}
		betas := r.Header.Values("anthropic-beta")
		if len(betas) != 2 || betas[0] != "beta-a" || betas[1] != "beta-b" {
			t.Errorf("expected 2 beta headers, got %v", betas)
		}
		chatReqJSON(w, 200, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.APIVersion = "2024-06-01"
	c.CustomHeaders = map[string]string{"X-Custom-Test": "custom-value"}
	c.BetaHeaders = []string{"beta-a", "beta-b"}
	resp, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Errorf("unexpected content: %q", resp.Choices[0].Message.Content)
	}
}

func TestCreateChatCompletionNonStream_SessionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "sess-123" {
			t.Errorf("expected explicit session header, got %q", got)
		}
		chatReqJSON(w, 200, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.SessionID = "sess-123"
	if _, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateChatCompletionNonStream_OpencodeSyntheticHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "sess_cfg-opencode_m" {
			t.Errorf("expected synthetic opencode session header, got %q", got)
		}
		chatReqJSON(w, 200, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL + "/v1/opencode.ai")
	c.ConfigID = "cfg-opencode"
	if _, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — connection failure (Do err + loop exhaustion cleanup)
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_ConnectionErrorRetriesThenFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // dial fails on every attempt

	c := newTestClient(srv.URL)
	c.RetryCount = 1
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "all retry attempts failed, last error") {
		t.Fatalf("expected connection error to exhaust retries, got %v", err)
	}
}

func TestCreateChatCompletionNonStream_RetryCountNegative(t *testing.T) {
	c := newTestClient("http://upstream.invalid")
	c.RetryCount = -1 // loop body never runs → lastErr stays nil
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "all retry attempts failed") {
		t.Fatalf("expected bare all-retries error, got %v", err)
	}
	if strings.Contains(err.Error(), "last error") {
		t.Errorf("expected no last error for zero attempts, got %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — empty-body 503
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_EmptyBody503(t *testing.T) {
	t.Chdir(t.TempDir()) // saveDebugRequest writes data/debug into cwd
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.RetryCount = 0
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 503 error with no response body") {
		t.Fatalf("expected synthesized empty-body message, got %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — normalized-request remarshal failures (via override seam)
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_NormalizeToolCallIDMarshalFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 400, `{"error":{"message":"No tool output found for function call call_correct."}}`)
	}))
	defer srv.Close()

	marshalCalls := 0
	c := newTestClient(srv.URL)
	c.marshalOverride = func(v any) ([]byte, error) {
		marshalCalls++
		if marshalCalls == 2 {
			return nil, errors.New("injected second-marshal failure")
		}
		return json.Marshal(v)
	}
	req := &models.OpenAIRequest{
		Model: "m",
		Messages: []models.OpenAIMessage{
			{Role: "assistant", ToolCalls: []models.OpenAIToolCall{{ID: "call_correct"}}},
			{Role: "tool", ToolCallID: "call_wrong", Content: "result"},
		},
	}
	_, err := c.CreateChatCompletionNonStream(req)
	if err == nil || !strings.Contains(err.Error(), "failed to marshal normalized request") {
		t.Fatalf("expected normalized-marshal failure, got %v", err)
	}
	if marshalCalls != 2 {
		t.Errorf("expected 2 marshal calls (initial + normalized), got %d", marshalCalls)
	}
}

func TestCreateChatCompletionNonStream_NormalizeToolChoiceMarshalFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 400, `{"error":{"message":"tool_choice.name is required when type is function"}}`)
	}))
	defer srv.Close()

	marshalCalls := 0
	c := newTestClient(srv.URL)
	c.marshalOverride = func(v any) ([]byte, error) {
		marshalCalls++
		if marshalCalls == 2 {
			return nil, errors.New("injected second-marshal failure")
		}
		return json.Marshal(v)
	}
	req := &models.OpenAIRequest{
		Model:      "m",
		ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "my_func"}},
	}
	_, err := c.CreateChatCompletionNonStream(req)
	if err == nil || !strings.Contains(err.Error(), "failed to marshal normalized request") {
		t.Fatalf("expected normalized-marshal failure, got %v", err)
	}
	if marshalCalls != 2 {
		t.Errorf("expected 2 marshal calls, got %d", marshalCalls)
	}
}

// ---------------------------------------------------------------------
// NonStream — 429 smart-backoff outcomes (injected)
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_429TransientLoop(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		chatReqJSON(w, 429, `{"error":{"message":"rate limit exceeded, please slow down"}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.RetryCount = 1
	c.wait429Override = func(context.Context, string, string, ratelimit.RateLimit429Config) (*ratelimit.RateLimit429Result, error) {
		return &ratelimit.RateLimit429Result{Severity: ratelimit.SeverityTransient, Aborted: false}, nil
	}
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "exhausted 12 retries") {
		t.Fatalf("expected inner 429 budget exhaustion, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 13 { // 12 waits (1..12) + 1 over-budget probe
		t.Errorf("expected 13 upstream calls before giving up, got %d", got)
	}
}

func TestCreateChatCompletionNonStream_429Aborted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 429, `{"error":{"message":"rate limit exceeded, please slow down"}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.wait429Override = func(context.Context, string, string, ratelimit.RateLimit429Config) (*ratelimit.RateLimit429Result, error) {
		return &ratelimit.RateLimit429Result{Severity: ratelimit.SeverityTransient, Aborted: true, Reason: "outer context deadline"}, nil
	}
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "429 rate limit") {
		t.Fatalf("expected aborted-429 error, got %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — Responses endpoint conversion paths
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_ResponsesConvErrFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 200, `{not-valid-json`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.UpstreamEndpoint = "responses"
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil {
		t.Fatalf("expected error from malformed responses body + empty fallback")
	}
	if !strings.Contains(err.Error(), "empty choices/content") {
		t.Errorf("expected empty-content error from the assembled fallback, got %v", err)
	}
}

func TestCreateChatCompletionNonStream_ResponsesConvertSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 200, `{"id":"resp_1","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.UpstreamEndpoint = "responses"
	resp, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "hello" {
		t.Errorf("expected converted content, got %q", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 8 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

// ---------------------------------------------------------------------
// NonStream — decode-error retry continue
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_DecodeErrorRetryContinue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 200, `{not-valid-json`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.RetryCount = 1
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to decode response after 2 attempts") {
		t.Fatalf("expected decode error after all retries, got %v", err)
	}
}

// ---------------------------------------------------------------------
// NonStream — empty-content with API error, across retries
// ---------------------------------------------------------------------

func TestCreateChatCompletionNonStream_EmptyContentErrorAllAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 200, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":""}}],"error":{"message":"boom"}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.RetryCount = 1
	_, err := c.CreateChatCompletionNonStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "API returned empty choices/content after 2 attempts") {
		t.Fatalf("expected empty-content error with API error detail, got %v", err)
	}
}

// ---------------------------------------------------------------------
// Stream — misc setup / failure branches
// ---------------------------------------------------------------------

func TestCreateChatCompletionStream_StreamOptionsAlreadySet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	req := &models.OpenAIRequest{Model: "m", StreamOptions: &models.StreamOptions{IncludeUsage: false}}
	rc, err := c.CreateChatCompletionStream(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc.Close()
	if req.StreamOptions.IncludeUsage != true {
		t.Errorf("expected IncludeUsage forced to true in the else branch")
	}
}

func TestCreateChatCompletionStream_MarshalFail(t *testing.T) {
	c := newTestClient("http://upstream.invalid")
	c.marshalOverride = func(any) ([]byte, error) { return nil, errors.New("injected marshal failure") }
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to marshal request") {
		t.Fatalf("expected marshal failure, got %v", err)
	}
}

func TestCreateChatCompletionStream_PrepareFail(t *testing.T) {
	c := newTestClient("http://upstream.invalid")
	c.prepareOverride = func(*models.OpenAIRequest, []byte) ([]byte, error) {
		return nil, errors.New("injected prepare failure")
	}
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to prepare request body") {
		t.Fatalf("expected prepare failure, got %v", err)
	}
}

func TestCreateChatCompletionStream_TimeoutImmediate(t *testing.T) {
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "0")
	c := newTestClient("http://upstream.invalid")
	c.Timeout = 1 // ns
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "request timeout exceeded") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestCreateChatCompletionStream_BackoffExceedsDeadlineBreak(t *testing.T) {
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT_SEC", "0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 503, `{"error":"overloaded"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.Timeout = 200 * time.Millisecond
	c.RetryCount = 1
	c.RetryBackoffBase = time.Hour
	c.RetryBackoffMax = time.Hour // must not cap the 1h backoff to the default 10ms
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "all retry attempts failed") {
		t.Fatalf("expected backoff-abort error, got %v", err)
	}
}

func TestCreateChatCompletionStream_NewRequestError(t *testing.T) {
	c := newTestClient("http://%zz")
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "failed to create request") {
		t.Fatalf("expected URL parse error, got %v", err)
	}
}

func TestCreateChatCompletionStream_HeadersFull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api-version"); got != "2024-06-01" {
			t.Errorf("expected api-version query param, got %q", got)
		}
		if got := r.Header.Get("X-Custom-Test"); got != "custom-value" {
			t.Errorf("expected custom header, got %q", got)
		}
		betas := r.Header.Values("anthropic-beta")
		if len(betas) != 2 || betas[0] != "beta-a" || betas[1] != "beta-b" {
			t.Errorf("expected 2 beta headers, got %v", betas)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.APIVersion = "2024-06-01"
	c.CustomHeaders = map[string]string{"X-Custom-Test": "custom-value"}
	c.BetaHeaders = []string{"beta-a", "beta-b"}
	rc, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc.Close()
}

func TestCreateChatCompletionStream_SessionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "sess-123" {
			t.Errorf("expected explicit session header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.SessionID = "sess-123"
	rc, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc.Close()
}

func TestCreateChatCompletionStream_OpencodeSyntheticHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "sess_cfg-opencode_m" {
			t.Errorf("expected synthetic opencode session header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := newTestClient(srv.URL + "/v1/opencode.ai")
	c.ConfigID = "cfg-opencode"
	rc, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc.Close()
}

func TestCreateChatCompletionStream_EmptyBody503(t *testing.T) {
	t.Chdir(t.TempDir()) // saveDebugRequest writes data/debug into cwd
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.RetryCount = 0
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 503 error with no response body") {
		t.Fatalf("expected synthesized empty-body message, got %v", err)
	}
}

// ---------------------------------------------------------------------
// Stream — normalized tool_choice remarshal / re-prepare failures
// ---------------------------------------------------------------------

func TestCreateChatCompletionStream_NormalizeToolChoiceMarshalFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 400, `{"error":{"message":"tool_choice.name is required when type is function"}}`)
	}))
	defer srv.Close()

	marshalCalls := 0
	c := newTestClient(srv.URL)
	c.marshalOverride = func(v any) ([]byte, error) {
		marshalCalls++
		if marshalCalls == 2 {
			return nil, errors.New("injected second-marshal failure")
		}
		return json.Marshal(v)
	}
	req := &models.OpenAIRequest{
		Model:      "m",
		ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "my_func"}},
	}
	_, err := c.CreateChatCompletionStream(req)
	if err == nil || !strings.Contains(err.Error(), "failed to marshal normalized request") {
		t.Fatalf("expected normalized-marshal failure, got %v", err)
	}
	if marshalCalls != 2 {
		t.Errorf("expected 2 marshal calls, got %d", marshalCalls)
	}
}

func TestCreateChatCompletionStream_NormalizeToolChoicePrepareFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 400, `{"error":{"message":"tool_choice.name is required when type is function"}}`)
	}))
	defer srv.Close()

	prepareCalls := 0
	c := newTestClient(srv.URL)
	c.prepareOverride = func(*models.OpenAIRequest, []byte) ([]byte, error) {
		prepareCalls++
		if prepareCalls == 2 {
			return nil, errors.New("injected second-prepare failure")
		}
		return nil, nil
	}
	req := &models.OpenAIRequest{
		Model:      "m",
		ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "my_func"}},
	}
	_, err := c.CreateChatCompletionStream(req)
	if err == nil || !strings.Contains(err.Error(), "failed to prepare normalized request") {
		t.Fatalf("expected normalized-prepare failure, got %v", err)
	}
	if prepareCalls != 2 {
		t.Errorf("expected 2 prepare calls, got %d", prepareCalls)
	}
}

// ---------------------------------------------------------------------
// Stream — 429 smart-backoff outcomes (injected)
// ---------------------------------------------------------------------

func TestCreateChatCompletionStream_429TransientLoop(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		chatReqJSON(w, 429, `{"error":{"message":"rate limit exceeded, please slow down"}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.RetryCount = 1
	c.wait429Override = func(context.Context, string, string, ratelimit.RateLimit429Config) (*ratelimit.RateLimit429Result, error) {
		return &ratelimit.RateLimit429Result{Severity: ratelimit.SeverityTransient, Aborted: false}, nil
	}
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "exhausted 12 retries") {
		t.Fatalf("expected inner 429 budget exhaustion, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 13 {
		t.Errorf("expected 13 upstream calls before giving up, got %d", got)
	}
}

func TestCreateChatCompletionStream_429Aborted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 429, `{"error":{"message":"rate limit exceeded, please slow down"}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.wait429Override = func(context.Context, string, string, ratelimit.RateLimit429Config) (*ratelimit.RateLimit429Result, error) {
		return &ratelimit.RateLimit429Result{Severity: ratelimit.SeverityTransient, Aborted: true, Reason: "outer context deadline"}, nil
	}
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "429 rate limit") {
		t.Fatalf("expected aborted-429 error, got %v", err)
	}
}

func TestCreateChatCompletionStream_429WaitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatReqJSON(w, 429, `{"error":{"message":"rate limit exceeded, please slow down"}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.wait429Override = func(context.Context, string, string, ratelimit.RateLimit429Config) (*ratelimit.RateLimit429Result, error) {
		return nil, errors.New("injected backoff failure")
	}
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "cancelled while waiting for 429 backoff") {
		t.Fatalf("expected backoff wait error, got %v", err)
	}
}

func TestCreateChatCompletionStream_RetryCountNegative(t *testing.T) {
	c := newTestClient("http://upstream.invalid")
	c.RetryCount = -1 // loop body never runs → lastErr stays nil
	_, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "all retry attempts failed") {
		t.Fatalf("expected bare all-retries error, got %v", err)
	}
	if strings.Contains(err.Error(), "last error") {
		t.Errorf("expected no last error for zero attempts, got %v", err)
	}
}

// ---------------------------------------------------------------------
// Stream — pipe CloseWithError via a mid-stream read failure
// ---------------------------------------------------------------------

// responsesErrorBody serves enough bytes for the Responses-format peek, then
// fails on the next read — exercising convertResponsesStreamingToChat's scanner
// error, which must be propagated through pw.CloseWithError.
type responsesErrorBody struct {
	served bool
}

func (r *responsesErrorBody) Read(p []byte) (int, error) {
	if !r.served {
		r.served = true
		line := `data: {"type":"response.output_text.delta","delta":"` + strings.Repeat("A", 60) + `"}` + "\n\n"
		return copy(p, []byte(line)), nil
	}
	return 0, fmt.Errorf("mid-stream simulated failure")
}

func (r *responsesErrorBody) Close() error { return nil }

type stubRoundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return s.roundTrip(req)
}

func TestCreateChatCompletionStream_PipeCloseWithError(t *testing.T) {
	c := newTestClient("http://upstream.test")
	c.UpstreamEndpoint = "responses"
	c.httpClient = &http.Client{Transport: &stubRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       &responsesErrorBody{},
			}, nil
		},
	}}
	rc, err := c.CreateChatCompletionStream(&models.OpenAIRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err == nil {
		t.Fatalf("expected mid-stream read error to surface through the pipe, got data %q", data)
	}
	if !strings.Contains(err.Error(), "mid-stream simulated failure") {
		t.Errorf("expected the underlying read failure to surface, got %v", err)
	}
}

// ---------------------------------------------------------------------
// bytes import guard (bytes.NewBuffer used by many request paths above)
// ---------------------------------------------------------------------

var _ = bytes.NewBuffer
