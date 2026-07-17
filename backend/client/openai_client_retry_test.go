package client

import "testing"

// TestIsRetryableHTTPStatus 锁定可重试状态码白名单，特别覆盖 424 上游过载码。
// 429 走专属分支（按 errorBody 区分配额耗尽 vs 临时限流），其余状态码查白名单。
func TestIsRetryableHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorBody  string
		want       bool
	}{
		{"424 upstream overload retryable", 424, "Service temporarily unavailable", true},
		{"408 timeout retryable", 408, "", true},
		{"429 transient rate limit retryable", 429, "rate limit", true},
		{"429 quota exhausted not retryable", 429, "insufficient_quota", false},
		{"500 retryable", 500, "", true},
		{"502 retryable", 502, "", true},
		{"503 retryable", 503, "", true},
		{"400 not retryable", 400, "", false},
		{"401 not retryable", 401, "", false},
		{"404 not retryable", 404, "", false},
		{"422 not retryable", 422, "", false},
	}
	for _, tt := range tests {
		got := isRetryableHTTPStatus(tt.statusCode, tt.errorBody)
		if got != tt.want {
			t.Errorf("%s: isRetryableHTTPStatus(%d, %q) = %v, want %v",
				tt.name, tt.statusCode, tt.errorBody, got, tt.want)
		}
	}
}
