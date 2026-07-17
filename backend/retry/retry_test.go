package retry

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestClassifyError_RateLimit(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"status 429", fmt.Errorf("OpenAI API error (status 429): rate limit")},
		{"rate limit message", fmt.Errorf("rate limit exceeded")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != CategoryRateLimit {
				t.Errorf("got %v, want CategoryRateLimit", got)
			}
		})
	}
}

func TestClassifyError_PermanentQuota(t *testing.T) {
	err := fmt.Errorf("status 429: insufficient_quota")
	if got := ClassifyError(err); got != CategoryPermanentQuota {
		t.Errorf("got %v, want CategoryPermanentQuota", got)
	}
}

func TestClassifyError_ServerError(t *testing.T) {
	codes := []string{"424", "500", "502", "503", "504"}
	for _, code := range codes {
		err := fmt.Errorf("status %s error", code)
		if got := ClassifyError(err); got != CategoryServerError {
			t.Errorf("status %s: got %v, want CategoryServerError", code, got)
		}
	}
}

func TestClassifyError_Network(t *testing.T) {
	signals := []string{"connection refused", "connection reset", "i/o timeout", "broken pipe",
		"timeout awaiting response headers", "timeout awaiting response",
		"context deadline exceeded"}
	for _, sig := range signals {
		err := fmt.Errorf("%s something", sig)
		if got := ClassifyError(err); got != CategoryNetwork {
			t.Errorf("%q: got %v, want CategoryNetwork", sig, got)
		}
	}
}

func TestClassifyError_Auth(t *testing.T) {
	err := fmt.Errorf("status 401: unauthorized")
	if got := ClassifyError(err); got != CategoryAuth {
		t.Errorf("got %v, want CategoryAuth", got)
	}
}

func TestClassifyError_Protocol(t *testing.T) {
	err := fmt.Errorf("status 400: bad request")
	if got := ClassifyError(err); got != CategoryProtocol {
		t.Errorf("got %v, want CategoryProtocol", got)
	}
}

func TestClassifyError_Cancelled(t *testing.T) {
	if got := ClassifyError(context.Canceled); got != CategoryCancelled {
		t.Errorf("got %v, want CategoryCancelled", got)
	}
}

func TestClassifyError_Nil(t *testing.T) {
	if got := ClassifyError(nil); got != CategoryUnknown {
		t.Errorf("got %v, want CategoryUnknown", got)
	}
}

func TestGetStrategy_RateLimitRetryable(t *testing.T) {
	s := GetStrategy(CategoryRateLimit)
	if !s.Retryable {
		t.Error("rate limit should be retryable")
	}
	if s.MaxRetries != 20 {
		t.Errorf("expected 20 retries, got %d", s.MaxRetries)
	}
}

func TestGetStrategy_ProtocolNotRetryable(t *testing.T) {
	s := GetStrategy(CategoryProtocol)
	if s.Retryable {
		t.Error("protocol errors should not be retryable")
	}
}

func TestGetStrategy_AuthNotRetryable(t *testing.T) {
	s := GetStrategy(CategoryAuth)
	if s.Retryable {
		t.Error("auth errors should not be retryable")
	}
}

func TestCalculateBackoff_Increases(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second}
	for attempt := 1; attempt <= 6; attempt++ {
		delay := CalculateBackoff(s, attempt)
		baseDelay := s.BaseDelay * time.Duration(1<<uint(attempt-1))
		if baseDelay > s.MaxDelay {
			baseDelay = s.MaxDelay
		}
		if delay < baseDelay {
			t.Errorf("attempt %d: delay %v < base %v", attempt, delay, baseDelay)
		}
	}
}

func TestCalculateBackoff_CappedAtMax(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 10 * time.Second}
	for attempt := 1; attempt <= 20; attempt++ {
		delay := CalculateBackoff(s, attempt)
		if delay > s.MaxDelay+5*time.Second {
			t.Errorf("attempt %d: delay %v exceeds max+tolerance", attempt, delay)
		}
	}
}

func TestCalculateBackoffWithRetryAfter(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second}
	retryAfter := 30 * time.Second
	delay := CalculateBackoffWithRetryAfter(s, 1, retryAfter)
	if delay != retryAfter {
		t.Errorf("expected Retry-After %v, got %v", retryAfter, delay)
	}
}

func TestCalculateBackoffWithRetryAfter_Fallback(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second}
	delay := CalculateBackoffWithRetryAfter(s, 1, 0)
	if delay == 0 {
		t.Error("expected non-zero backoff when Retry-After is 0")
	}
}

func TestExecute_Success(t *testing.T) {
	r := Execute(context.Background(), func() error { return nil })
	if !r.Succeeded {
		t.Error("expected success")
	}
	if r.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", r.Attempts)
	}
}

func TestExecute_NonRetryable(t *testing.T) {
	r := Execute(context.Background(), func() error {
		return fmt.Errorf("status 400: bad request")
	})
	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Attempts != 1 {
		t.Errorf("non-retryable should only attempt once, got %d", r.Attempts)
	}
}

func TestExecute_RateLimitRetries(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}
	callCount := 0
	r := ExecuteWithStrategies(context.Background(), testStrategies, func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("status 429: rate limit")
		}
		return nil
	})
	if !r.Succeeded {
		t.Error("expected success after retries")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestExecute_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := Execute(ctx, func() error {
		return fmt.Errorf("status 429: rate limit")
	})
	if r.Category != CategoryCancelled {
		t.Errorf("expected CategoryCancelled, got %v", r.Category)
	}
}

func TestExecute_CategorySwitchMidRetry(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}
	callCount := 0
	r := ExecuteWithStrategies(context.Background(), testStrategies, func() error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("status 429: rate limit")
		}
		return fmt.Errorf("status 400: bad request")
	})
	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Category != CategoryProtocol {
		t.Errorf("final category should be protocol, got %v", r.Category)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
		{fmt.Errorf("status 424: service temporarily unavailable"), true},
		{fmt.Errorf("status 429: rate limit"), true},
		{fmt.Errorf("status 500: internal error"), true},
		{fmt.Errorf("connection refused"), true},
		{fmt.Errorf("status 400: bad request"), false},
		{fmt.Errorf("status 401: unauthorized"), false},
		{fmt.Errorf("insufficient_quota exceeded"), false},
		{context.Canceled, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := IsRetryable(tt.err); got != tt.retryable {
			t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
		}
	}
}

func TestErrorCategory_String(t *testing.T) {
	tests := []struct {
		cat  ErrorCategory
		want string
	}{
		{CategoryRateLimit, "rate_limit"},
		{CategoryServerError, "server_error"},
		{CategoryNetwork, "network"},
		{CategoryProtocol, "protocol"},
		{CategoryAuth, "auth"},
		{CategoryCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if got := tt.cat.String(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestExecute_ServerErrorRetryCount(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryServerError: {MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}
	callCount := 0
	r := ExecuteWithStrategies(context.Background(), testStrategies, func() error {
		callCount++
		return fmt.Errorf("status 500: internal error")
	})
	if r.Succeeded {
		t.Error("should not succeed")
	}
	expected := testStrategies[CategoryServerError].MaxRetries + 1
	if callCount != expected {
		t.Errorf("expected %d calls, got %d", expected, callCount)
	}
}

func TestFormatRetryError(t *testing.T) {
	r := &Result{
		Attempts:   3,
		LastErr:    fmt.Errorf("original error"),
		Category:   CategoryRateLimit,
		TotalDelay: 15 * time.Second,
	}
	err := FormatRetryError(r)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if msg == "original error" {
		t.Error("should contain retry context")
	}
}

func TestEngine_PerCategoryBudget(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryServerError: {MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
		CategoryRateLimit:   {MaxRetries: 5, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	callCount := 0
	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		callCount++
		if callCount <= 2 {
			return fmt.Errorf("status 500: server error")
		}
		if callCount <= 4 {
			return fmt.Errorf("status 429: rate limit")
		}
		if callCount <= 5 {
			return fmt.Errorf("status 500: server error")
		}
		return nil
	})

	if !r.Succeeded {
		t.Errorf("expected success, got: %v", r.LastErr)
	}
	if r.Attempts != 6 {
		t.Errorf("expected 6 attempts, got %d", r.Attempts)
	}
}

func TestEngine_CategoryExhaustion(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryServerError: {MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		return fmt.Errorf("status 500: always fails")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Category != CategoryServerError {
		t.Errorf("expected CategoryServerError, got %v", r.Category)
	}
}

func TestEngine_StateTransition_StopsOnNonRetryable(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 10, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	callCount := 0
	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("status 429: rate limit")
		}
		return fmt.Errorf("status 400: bad request")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Attempts != 2 {
		t.Errorf("expected 2 attempts (429 then 400), got %d", r.Attempts)
	}
	if r.Category != CategoryProtocol {
		t.Errorf("final category should be protocol, got %v", r.Category)
	}
}

func TestEngine_AdaptiveBackoff(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 4, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Retryable: true},
	}

	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		return fmt.Errorf("status 429: rate limit")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.TotalDelay < 50*time.Millisecond {
		t.Errorf("total delay %v seems too low for adaptive backoff", r.TotalDelay)
	}
}

func TestEngine_SuccessAfterRetries(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 10, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	callCount := 0
	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		callCount++
		if callCount < 5 {
			return fmt.Errorf("status 429: rate limit")
		}
		return nil
	})

	if !r.Succeeded {
		t.Error("expected success")
	}
	if r.Attempts != 5 {
		t.Errorf("expected 5 attempts, got %d", r.Attempts)
	}
	if r.Category != CategoryRateLimit {
		t.Errorf("expected CategoryRateLimit, got %v", r.Category)
	}
}

func TestEngine_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := NewEngine()
	r := engine.Execute(ctx, func() error {
		return fmt.Errorf("status 429: rate limit")
	})

	if r.Category != CategoryCancelled {
		t.Errorf("expected CategoryCancelled, got %v", r.Category)
	}
}

func TestEngine_NonRetryableImmediateReturn(t *testing.T) {
	engine := NewEngine()
	r := engine.Execute(context.Background(), func() error {
		return fmt.Errorf("status 400: bad request")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Attempts != 1 {
		t.Errorf("should be 1 attempt for non-retryable, got %d", r.Attempts)
	}
}

func TestResult_ErrorSummary(t *testing.T) {
	r := &Result{Attempts: 1, LastErr: fmt.Errorf("simple error")}
	if !strings.Contains(r.ErrorSummary(), "simple error") {
		t.Error("single attempt should return raw error")
	}

	r2 := &Result{
		Attempts: 5, Category: CategoryRateLimit, TotalDelay: 10 * time.Second,
		LastErr: fmt.Errorf("rate limit"),
	}
	summary := r2.ErrorSummary()
	if !strings.Contains(summary, "5 attempts") || !strings.Contains(summary, "rate_limit") {
		t.Errorf("summary missing context: %s", summary)
	}
}

func TestClassifyError_WrappedTimeout(t *testing.T) {
	innerErr := fmt.Errorf("net/http: timeout awaiting response headers")
	wrappedErr := fmt.Errorf("all retry attempts failed, last error: %w", innerErr)
	if got := ClassifyError(wrappedErr); got != CategoryNetwork {
		t.Errorf("wrapped timeout error: got %v, want CategoryNetwork", got)
	}
}

func TestClassifyError_WrappedContextDeadline(t *testing.T) {
	innerErr := fmt.Errorf("context deadline exceeded")
	wrappedErr := fmt.Errorf("all retry attempts failed, last error: %w", innerErr)
	if got := ClassifyError(wrappedErr); got != CategoryNetwork {
		t.Errorf("wrapped context deadline: got %v, want CategoryNetwork", got)
	}
}
