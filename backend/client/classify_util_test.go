package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestClassifyOpenAIError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // "" means expect passthrough of input
	}{
		{"empty string", "", "Server error with no details provided. This may be a temporary issue, please retry."},
		{"whitespace only", "   ", "Server error with no details provided. This may be a temporary issue, please retry."},
		{"region restriction", "unsupported_country_region_territory", "OpenAI API is not available in your region. Consider using a VPN or Azure OpenAI service."},
		{"region restriction phrase", "Country, region, or territory not supported", "OpenAI API is not available in your region. Consider using a VPN or Azure OpenAI service."},
		{"invalid api key", "Error: invalid_api_key provided", "Invalid API key. Please check your OPENAI_API_KEY configuration."},
		{"unauthorized", "401 Unauthorized", "Invalid API key. Please check your OPENAI_API_KEY configuration."},
		{"rate limit", "rate_limit_exceeded", "Rate limit exceeded. Please wait and try again, or upgrade your API plan."},
		{"quota", "quota exceeded for this month", "Rate limit exceeded. Please wait and try again, or upgrade your API plan."},
		{"model not found", "model 'gpt-9' not found", "The requested model is temporarily unavailable. Please try again."},
		{"model does not exist", "the model does not exist", "The requested model is temporarily unavailable. Please try again."},
		{"unknown aliased model", "Unknown aliased model: foo", "The requested model is temporarily unavailable. Please try again."},
		{"no available channel", "no available channel for this key", "The requested model is temporarily unavailable. Please try again."},
		{"billing", "billing issue detected", "Billing issue. Please check your OpenAI account billing status."},
		{"payment", "payment required", "Billing issue. Please check your OpenAI account billing status."},
		{"unrelated error passthrough", "some totally unrelated error", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == "" {
				want = tt.input
			}
			if got := ClassifyOpenAIError(tt.input); got != want {
				t.Errorf("ClassifyOpenAIError(%q) = %q, want %q", tt.input, got, want)
			}
		})
	}
}

func TestIsModelRoutingError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"unknown aliased model", "Unknown aliased model: foo", true},
		{"no available channel", "no available channel", true},
		{"model not found", "model xyz not found", true},
		{"model does not exist", "the model does not exist here", true},
		{"unrelated", "some other error", false},
		{"model word without not-found phrase", "model is great", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsModelRoutingError(tt.input); got != tt.want {
				t.Errorf("IsModelRoutingError(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTransientNetworkError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"unexpected EOF", "unexpected EOF", true},
		{"bare eof substring", "responses streaming scanner: EOF", true},
		{"connection reset", "read: connection reset by peer", true},
		{"connection refused", "dial tcp: connection refused", true},
		{"broken pipe", "write: broken pipe", true},
		{"i/o timeout", "read tcp: i/o timeout", true},
		{"no such host", "dial tcp: no such host", true},
		{"network unreachable", "network is unreachable", true},
		{"context deadline exceeded", "context deadline exceeded", true},
		{"tls handshake timeout", "net/http: TLS handshake timeout", true},
		{"closed network connection", "use of closed network connection", true},
		{"unrelated application error", "invalid_request_error: bad param", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransientNetworkError(tt.input); got != tt.want {
				t.Errorf("IsTransientNetworkError(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	if !IsRetryableError(fmt.Errorf("status 429: rate limit")) {
		t.Errorf("expected 429 status error to be retryable")
	}
	if IsRetryableError(nil) {
		t.Errorf("expected nil error to not be retryable")
	}
	if IsRetryableError(errors.New("totally unknown error shape")) {
		t.Errorf("expected unknown error category to not be retryable")
	}
}

func TestCalculateBackoff(t *testing.T) {
	c := &OpenAIClient{
		RetryBackoffBase: 1 * time.Second,
		RetryBackoffMax:  10 * time.Second,
	}
	if got := c.CalculateBackoff(0); got != 0 {
		t.Errorf("attempt 0: got %v, want 0", got)
	}
	if got := c.CalculateBackoff(-1); got != 0 {
		t.Errorf("negative attempt: got %v, want 0", got)
	}
	if got := c.CalculateBackoff(1); got != 1*time.Second {
		t.Errorf("attempt 1: got %v, want 1s", got)
	}
	if got := c.CalculateBackoff(2); got != 2*time.Second {
		t.Errorf("attempt 2: got %v, want 2s", got)
	}
	if got := c.CalculateBackoff(3); got != 4*time.Second {
		t.Errorf("attempt 3: got %v, want 4s", got)
	}
	// Large attempt count must clamp at RetryBackoffMax, not overflow/panic.
	if got := c.CalculateBackoff(10); got != 10*time.Second {
		t.Errorf("attempt 10 (capped): got %v, want 10s (RetryBackoffMax)", got)
	}
}

func TestCanonicalToolCallID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"call_ prefix normalized", "call_abc123", "fcabc123"},
		{"fc_ prefix normalized", "fc_abc123", "fcabc123"},
		{"underscores stripped globally", "call_ab_c_123", "fcabc123"},
		{"uppercase not normalized to fc prefix (prefix match is case-sensitive)", "CALL_ABC", "callabc"},
		{"no recognized prefix passthrough lowered", "xyz_789", "xyz789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalToolCallID(tt.input); got != tt.want {
				t.Errorf("canonicalToolCallID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsRetryableModelRoutingError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"unknown aliased model", "unknown aliased model: foo", true},
		{"model not found exact phrase", "model not found", true},
		{"no available channel", "no available channel", true},
		{"unrelated", "bad request: missing field", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableModelRoutingError(tt.input); got != tt.want {
				t.Errorf("isRetryableModelRoutingError(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsRetryableHTTPStatus_AllCodes(t *testing.T) {
	retryable := []int{424, 408, 406, 502, 503, 504, 506, 507, 508, 509, 510, 511, 500, 599}
	for _, code := range retryable {
		if !isRetryableHTTPStatus(code, "") {
			t.Errorf("expected status %d to be retryable", code)
		}
	}
	notRetryable := []int{200, 201, 301, 400, 401, 403, 404, 405, 409, 410, 422}
	for _, code := range notRetryable {
		if isRetryableHTTPStatus(code, "") {
			t.Errorf("expected status %d to not be retryable", code)
		}
	}
}

func TestExtractNoToolOutputCallID(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"standard quoted call id",
			`{"error":{"message":"No tool output found for function call call_abc123."}}`,
			"call_abc123",
		},
		{
			"backtick quoted (regression: trailing backtick must not leak into the id)",
			"No tool output found for function call `call_xyz`.",
			"call_xyz",
		},
		{
			"terminated by comma",
			"No tool output found for function call call_1, please retry",
			"call_1",
		},
		{
			"terminated by closing bracket",
			"No tool output found for function call call_2]",
			"call_2",
		},
		{
			"no marker present",
			"some unrelated error message",
			"",
		},
		{
			"marker present but nothing after it",
			"No tool output found for function call ",
			"",
		},
		{
			"marker followed only by quote chars",
			"No tool output found for function call \"\"",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractNoToolOutputCallID(tt.body); got != tt.want {
				t.Errorf("extractNoToolOutputCallID(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestNormalizeToolCallIDsForRetry(t *testing.T) {
	errBody := `{"error":{"message":"No tool output found for function call call_missing."}}`

	t.Run("nil request", func(t *testing.T) {
		req, changed := normalizeToolCallIDsForRetry(nil, errBody)
		if changed || req != nil {
			t.Fatalf("expected no-op for nil request")
		}
	})

	t.Run("no marker in error body", func(t *testing.T) {
		req := &models.OpenAIRequest{}
		_, changed := normalizeToolCallIDsForRetry(req, "unrelated error")
		if changed {
			t.Fatalf("expected changed=false when no marker present")
		}
	})

	t.Run("missing id not in any assistant tool_calls", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{{ID: "call_other"}}},
			},
		}
		_, changed := normalizeToolCallIDsForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false when missing id is unknown to assistant")
		}
	})

	t.Run("single unmatched tool message gets rewritten", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{{ID: "call_missing"}}},
				{Role: "tool", ToolCallID: "call_wrong_id"},
			},
		}
		rewritten, changed := normalizeToolCallIDsForRetry(req, errBody)
		if !changed {
			t.Fatalf("expected changed=true")
		}
		if rewritten.Messages[1].ToolCallID != "call_missing" {
			t.Errorf("expected tool message ToolCallID rewritten to call_missing, got %q", rewritten.Messages[1].ToolCallID)
		}
		// original untouched
		if req.Messages[1].ToolCallID != "call_wrong_id" {
			t.Errorf("original request must not be mutated")
		}
	})

	t.Run("already has target tool message, no rewrite needed", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{{ID: "call_missing"}}},
				{Role: "tool", ToolCallID: "call_missing"},
			},
		}
		_, changed := normalizeToolCallIDsForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false when target tool message already exists")
		}
	})

	t.Run("no tool messages at all", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{{ID: "call_missing"}}},
			},
		}
		_, changed := normalizeToolCallIDsForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false when there are no tool messages")
		}
	})

	t.Run("canonical id match picks correct candidate among many", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{{ID: "call_missing"}}},
				{Role: "tool", ToolCallID: "fc_other"},
				{Role: "tool", ToolCallID: "fc_missing"}, // canonicalizes same as call_missing
			},
		}
		rewritten, changed := normalizeToolCallIDsForRetry(req, errBody)
		if !changed {
			t.Fatalf("expected changed=true via canonical id match")
		}
		if rewritten.Messages[2].ToolCallID != "call_missing" {
			t.Errorf("expected canonical-matched message rewritten, got %q", rewritten.Messages[2].ToolCallID)
		}
		if rewritten.Messages[1].ToolCallID != "fc_other" {
			t.Errorf("unrelated tool message must remain untouched, got %q", rewritten.Messages[1].ToolCallID)
		}
	})

	t.Run("multiple unmatched candidates with no canonical match and no unique unmatched -> no rewrite", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{
					{ID: "call_missing"}, {ID: "call_known_a"}, {ID: "call_known_b"},
				}},
				{Role: "tool", ToolCallID: "call_known_a"},
				{Role: "tool", ToolCallID: "call_known_b"},
			},
		}
		_, changed := normalizeToolCallIDsForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false: both tool messages match known assistant ids, none unmatched")
		}
	})

	t.Run("multiple candidates but exactly one unmatched to assistant ids", func(t *testing.T) {
		req := &models.OpenAIRequest{
			Messages: []models.OpenAIMessage{
				{Role: "assistant", ToolCalls: []models.OpenAIToolCall{
					{ID: "call_missing"}, {ID: "call_known_a"},
				}},
				{Role: "tool", ToolCallID: "call_known_a"},
				{Role: "tool", ToolCallID: "call_stray_unmatched"},
			},
		}
		rewritten, changed := normalizeToolCallIDsForRetry(req, errBody)
		if !changed {
			t.Fatalf("expected changed=true: exactly one tool message is unmatched to any assistant id")
		}
		if rewritten.Messages[2].ToolCallID != "call_missing" {
			t.Errorf("expected the unmatched tool message rewritten, got %q", rewritten.Messages[2].ToolCallID)
		}
	})
}

func TestMinHelper(t *testing.T) {
	if min(1, 2) != 1 {
		t.Errorf("min(1,2) should be 1")
	}
	if min(5, 3) != 3 {
		t.Errorf("min(5,3) should be 3")
	}
	if min(4, 4) != 4 {
		t.Errorf("min(4,4) should be 4")
	}
}

func TestToIntHelper(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want int
	}{
		{"float64", float64(42.7), 42},
		{"int", int(7), 7},
		{"int64", int64(99), 99},
		{"json.Number valid", json.Number("123"), 123},
		{"json.Number invalid", json.Number("not-a-number"), 0},
		{"unsupported type", "42", 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toInt(tt.in); got != tt.want {
				t.Errorf("toInt(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
