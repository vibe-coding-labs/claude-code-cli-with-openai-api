package handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// TestNewRetryHandler tests retry handler creation
func TestNewRetryHandler(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	rh := NewRetryHandler(3, 100*time.Millisecond, 5*time.Second, selector, cbMgr)

	if rh == nil {
		t.Fatal("Retry handler should not be nil")
	}
}

// TestExecuteWithRetry_Success tests successful execution without retries
func TestExecuteWithRetry_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	// Mark all configs as healthy
	for _, node := range lb.ConfigNodes {
		database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
	}

	rh := NewRetryHandler(3, 100*time.Millisecond, 5*time.Second, selector, cbMgr)

	ctx := context.Background()
	callCount := 0

	err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		callCount++
		return nil // Success on first try
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

// TestExecuteWithRetry_SuccessAfterRetries tests success after retries
func TestExecuteWithRetry_SuccessAfterRetries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	// Mark all configs as healthy
	for _, node := range lb.ConfigNodes {
		database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
	}

	rh := NewRetryHandler(3, 10*time.Millisecond, 100*time.Millisecond, selector, cbMgr)

	ctx := context.Background()
	callCount := 0

	err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		callCount++
		if callCount < 3 {
			return errors.New("connection timeout") // Retryable error
		}
		return nil // Success on third try
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

// TestExecuteWithRetry_MaxRetriesExceeded tests max retries exceeded
func TestExecuteWithRetry_MaxRetriesExceeded(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	// Mark all configs as healthy
	for _, node := range lb.ConfigNodes {
		database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
	}

	maxRetries := 3
	rh := NewRetryHandler(maxRetries, 10*time.Millisecond, 100*time.Millisecond, selector, cbMgr)

	ctx := context.Background()
	callCount := 0

	err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		callCount++
		return errors.New("connection timeout") // Always fail
	})

	if err == nil {
		t.Error("Expected error after max retries")
	}

	// Should be called maxRetries + 1 times (initial + retries)
	expectedCalls := maxRetries + 1
	if callCount != expectedCalls {
		t.Errorf("Expected %d calls, got %d", expectedCalls, callCount)
	}
}

// TestExecuteWithRetry_NonRetryableError tests non-retryable error
func TestExecuteWithRetry_NonRetryableError(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	// Mark all configs as healthy
	for _, node := range lb.ConfigNodes {
		database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
	}

	rh := NewRetryHandler(3, 10*time.Millisecond, 100*time.Millisecond, selector, cbMgr)

	ctx := context.Background()
	callCount := 0

	err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		callCount++
		return errors.New("status code 404") // Non-retryable error
	})

	if err == nil {
		t.Error("Expected error for non-retryable error")
	}

	// Should only be called once (no retries for non-retryable errors)
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

// TestExecuteWithRetry_ContextCanceled tests context cancellation
func TestExecuteWithRetry_ContextCanceled(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	// Mark all configs as healthy
	for _, node := range lb.ConfigNodes {
		database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
	}

	rh := NewRetryHandler(3, 100*time.Millisecond, 1*time.Second, selector, cbMgr)

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	// Cancel context after first failure
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		callCount++
		return errors.New("connection timeout") // Retryable error
	})

	if err == nil {
		t.Error("Expected error for canceled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}

	// Should be called at least once
	if callCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", callCount)
	}
}

// TestIsRetryableError tests error classification
func TestIsRetryableError(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	rh := NewRetryHandler(3, 100*time.Millisecond, 5*time.Second, selector, cbMgr).(*DefaultRetryHandler)

	testCases := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "connection refused",
			err:       errors.New("connection refused"),
			retryable: true,
		},
		{
			name:      "connection reset",
			err:       errors.New("connection reset by peer"),
			retryable: true,
		},
		{
			name:      "connection timeout",
			err:       errors.New("connection timeout"),
			retryable: true,
		},
		{
			name:      "i/o timeout",
			err:       errors.New("i/o timeout"),
			retryable: true,
		},
		{
			name:      "no such host",
			err:       errors.New("no such host"),
			retryable: true,
		},
		{
			name:      "network unreachable",
			err:       errors.New("network is unreachable"),
			retryable: true,
		},
		{
			name:      "broken pipe",
			err:       errors.New("broken pipe"),
			retryable: true,
		},
		{
			name:      "circuit breaker open",
			err:       errors.New("circuit breaker is open"),
			retryable: true,
		},
		{
			name:      "HTTP 500",
			err:       errors.New("status code 500"),
			retryable: true,
		},
		{
			name:      "HTTP 502",
			err:       errors.New("status code 502"),
			retryable: true,
		},
		{
			name:      "HTTP 503",
			err:       errors.New("status code 503"),
			retryable: true,
		},
		{
			name:      "HTTP 504",
			err:       errors.New("status code 504"),
			retryable: true,
		},
		{
			name:      "HTTP 429",
			err:       errors.New("status code 429"),
			retryable: true,
		},
		{
			name:      "HTTP 400",
			err:       errors.New("status code 400"),
			retryable: false,
		},
		{
			name:      "HTTP 401",
			err:       errors.New("status code 401"),
			retryable: false,
		},
		{
			name:      "HTTP 403",
			err:       errors.New("status code 403"),
			retryable: false,
		},
		{
			name:      "HTTP 404",
			err:       errors.New("status code 404"),
			retryable: false,
		},
		{
			name:      "context canceled",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "context deadline exceeded",
			err:       context.DeadlineExceeded,
			retryable: false,
		},
		{
			name:      "unknown error",
			err:       errors.New("some unknown error"),
			retryable: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rh.IsRetryableError(tc.err)
			if result != tc.retryable {
				t.Errorf("Expected retryable=%v for error '%v', got %v", tc.retryable, tc.err, result)
			}
		})
	}
}

// TestIsRetryableError_NetworkError tests network error detection
func TestIsRetryableError_NetworkError(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	rh := NewRetryHandler(3, 100*time.Millisecond, 5*time.Second, selector, cbMgr).(*DefaultRetryHandler)

	// Create a network error
	netErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}

	if !rh.IsRetryableError(netErr) {
		t.Error("Network errors should be retryable")
	}
}

// TestCalculateBackoff tests backoff calculation
func TestCalculateBackoff(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	initialDelay := 100 * time.Millisecond
	maxDelay := 5 * time.Second

	rh := NewRetryHandler(10, initialDelay, maxDelay, selector, cbMgr).(*DefaultRetryHandler)

	testCases := []struct {
		retryCount    int
		expectedDelay time.Duration
	}{
		{0, 100 * time.Millisecond},  // 100 * 2^0 = 100ms
		{1, 200 * time.Millisecond},  // 100 * 2^1 = 200ms
		{2, 400 * time.Millisecond},  // 100 * 2^2 = 400ms
		{3, 800 * time.Millisecond},  // 100 * 2^3 = 800ms
		{4, 1600 * time.Millisecond}, // 100 * 2^4 = 1600ms
		{5, 3200 * time.Millisecond}, // 100 * 2^5 = 3200ms
		{6, 5 * time.Second},         // 100 * 2^6 = 6400ms, capped at 5000ms
		{7, 5 * time.Second},         // Capped at max
		{10, 5 * time.Second},        // Capped at max
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("retry_%d", tc.retryCount), func(t *testing.T) {
			delay := rh.CalculateBackoff(tc.retryCount)
			if delay != tc.expectedDelay {
				t.Errorf("Expected delay %v for retry %d, got %v", tc.expectedDelay, tc.retryCount, delay)
			}
		})
	}
}

// TestCalculateBackoff_ExponentialGrowth tests exponential growth
func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	initialDelay := 100 * time.Millisecond
	maxDelay := 10 * time.Second

	rh := NewRetryHandler(10, initialDelay, maxDelay, selector, cbMgr).(*DefaultRetryHandler)

	// Verify exponential growth
	prevDelay := time.Duration(0)
	for i := 0; i < 5; i++ {
		delay := rh.CalculateBackoff(i)

		// Each delay should be at least double the previous (until max)
		if i > 0 && delay < maxDelay {
			if delay < prevDelay*2 {
				t.Errorf("Delay not growing exponentially: retry %d delay %v, previous %v", i, delay, prevDelay)
			}
		}

		prevDelay = delay
	}
}

// TestIsRetryableHTTPStatus tests HTTP status code classification
func TestIsRetryableHTTPStatus(t *testing.T) {
	testCases := []struct {
		statusCode int
		retryable  bool
	}{
		{200, false}, // OK
		{201, false}, // Created
		{400, false}, // Bad Request
		{401, false}, // Unauthorized
		{403, false}, // Forbidden
		{404, false}, // Not Found
		{429, true},  // Too Many Requests
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status_%d", tc.statusCode), func(t *testing.T) {
			result := IsRetryableHTTPStatus(tc.statusCode)
			if result != tc.retryable {
				t.Errorf("Expected retryable=%v for status %d, got %v", tc.retryable, tc.statusCode, result)
			}
		})
	}
}

// TestHTTPStatusCodeError tests HTTP status code error
func TestHTTPStatusCodeError(t *testing.T) {
	err := NewHTTPStatusCodeError(500, "Internal Server Error")

	if err == nil {
		t.Fatal("Error should not be nil")
	}

	expectedMsg := "HTTP status code 500: Internal Server Error"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestDefaultRetryConfig tests default retry configuration
func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", config.MaxRetries)
	}

	if config.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay=100ms, got %v", config.InitialDelay)
	}

	if config.MaxDelay != 5*time.Second {
		t.Errorf("Expected MaxDelay=5s, got %v", config.MaxDelay)
	}
}

// TestExecuteWithRetry_DifferentNodes tests retry with different nodes
func TestExecuteWithRetry_DifferentNodes(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	cbMgr := createTestCircuitBreakerManager()
	selector, _ := NewEnhancedSelector(lb, cbMgr)

	// Mark all configs as healthy
	for _, node := range lb.ConfigNodes {
		database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
	}

	rh := NewRetryHandler(3, 10*time.Millisecond, 100*time.Millisecond, selector, cbMgr)

	ctx := context.Background()
	selectedConfigs := make(map[string]bool)

	err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		selectedConfigs[config.ID] = true
		if len(selectedConfigs) < 2 {
			return errors.New("connection timeout") // Fail until we've tried different nodes
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Should have tried at least 2 different configs
	if len(selectedConfigs) < 2 {
		t.Errorf("Expected at least 2 different configs, got %d", len(selectedConfigs))
	}
}
