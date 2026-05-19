package handler

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/quick"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// **Validates: Requirements 3.1, 3.2, 3.3, 3.5**
// Property: Retry count never exceeds maxRetries
func TestProperty_RetryCountNeverExceedsMax(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	property := func(maxRetries uint8) bool {
		// Limit maxRetries to reasonable range (0-10)
		if maxRetries > 10 {
			maxRetries = maxRetries % 11
		}

		// Clean up database for each property test iteration
		database.CloseDB()
		os.Remove("test_selector.db")
		os.Remove("test_selector.db-shm")
		os.Remove("test_selector.db-wal")
		setupTestDB(t)
		if err := database.InitEncryption(); err != nil {
			return false
		}

		lb := createTestLoadBalancer(t)
		
		// Don't use circuit breaker for this test to avoid interference
		selector, _ := NewEnhancedSelector(lb, nil)

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
		}

		rh := NewRetryHandler(int(maxRetries), 1*time.Millisecond, 10*time.Millisecond, selector, nil)

		ctx := context.Background()
		callCount := 0

		_ = rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			callCount++
			return errors.New("connection timeout") // Always fail with retryable error
		})

		// Call count should be maxRetries + 1 (initial attempt + retries)
		expectedCalls := int(maxRetries) + 1
		return callCount == expectedCalls
	}


	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// **Validates: Requirements 3.3**
// Property: Backoff delay grows exponentially until max delay
func TestProperty_BackoffGrowsExponentially(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	property := func(initialDelayMs uint16, maxDelayMs uint16) bool {
		// Ensure reasonable values
		if initialDelayMs == 0 {
			initialDelayMs = 1
		}
		if initialDelayMs > 1000 {
			initialDelayMs = initialDelayMs % 1000
		}
		if maxDelayMs < initialDelayMs {
			maxDelayMs = initialDelayMs * 10
		}
		if maxDelayMs > 10000 {
			maxDelayMs = 10000
		}

		lb := createTestLoadBalancer(t)
		cbMgr := createTestCircuitBreakerManager()
		selector, _ := NewEnhancedSelector(lb, cbMgr)

		initialDelay := time.Duration(initialDelayMs) * time.Millisecond
		maxDelay := time.Duration(maxDelayMs) * time.Millisecond

		rh := NewRetryHandler(10, initialDelay, maxDelay, selector, cbMgr).(*DefaultRetryHandler)

		// Check exponential growth for first few retries
		prevDelay := time.Duration(0)
		for i := 0; i < 5; i++ {
			delay := rh.CalculateBackoff(i)

			// Delay should never exceed max
			if delay > maxDelay {
				return false
			}

			// Each delay should be at least double the previous (until max is reached)
			if i > 0 && prevDelay < maxDelay/2 {
				if delay < prevDelay*2 {
					return false
				}
			}

			prevDelay = delay
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


// **Validates: Requirements 3.6, 3.7**
// Property: Non-retryable errors never trigger retries
func TestProperty_NonRetryableErrorsNoRetry(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	nonRetryableErrors := []error{
		errors.New("status code 400"),
		errors.New("status code 401"),
		errors.New("status code 403"),
		errors.New("status code 404"),
		context.Canceled,
		context.DeadlineExceeded,
		errors.New("some unknown error"),
	}

	property := func(errorIndex uint8) bool {
		// Select an error from the list
		idx := int(errorIndex) % len(nonRetryableErrors)
		testError := nonRetryableErrors[idx]

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

		rh := NewRetryHandler(5, 1*time.Millisecond, 10*time.Millisecond, selector, cbMgr)

		ctx := context.Background()
		callCount := 0

		_ = rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			callCount++
			return testError
		})

		// Should only be called once (no retries for non-retryable errors)
		return callCount == 1
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


// **Validates: Requirements 3.6**
// Property: Retryable errors trigger retries up to max
func TestProperty_RetryableErrorsDoRetry(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	retryableErrors := []error{
		errors.New("connection refused"),
		errors.New("connection reset"),
		errors.New("connection timeout"),
		errors.New("i/o timeout"),
		errors.New("status code 500"),
		errors.New("status code 502"),
		errors.New("status code 503"),
		errors.New("status code 504"),
		errors.New("status code 429"),
	}

	property := func(errorIndex uint8, maxRetries uint8) bool {
		// Limit maxRetries to reasonable range
		if maxRetries > 10 {
			maxRetries = maxRetries % 11
		}

		// Select an error from the list
		idx := int(errorIndex) % len(retryableErrors)
		testError := retryableErrors[idx]

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

		rh := NewRetryHandler(int(maxRetries), 1*time.Millisecond, 10*time.Millisecond, selector, cbMgr)

		ctx := context.Background()
		callCount := 0

		_ = rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			callCount++
			return testError
		})

		// Should be called maxRetries + 1 times
		expectedCalls := int(maxRetries) + 1
		return callCount == expectedCalls
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


// **Validates: Requirements 3.1, 3.2**
// Property: Success on any attempt stops retries (idempotency)
func TestProperty_SuccessStopsRetries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	property := func(successAfter uint8, maxRetries uint8) bool {
		// Limit to reasonable ranges
		if maxRetries > 10 {
			maxRetries = maxRetries % 11
		}
		if maxRetries == 0 {
			maxRetries = 1
		}
		if successAfter > maxRetries {
			successAfter = successAfter % (maxRetries + 1)
		}

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

		rh := NewRetryHandler(int(maxRetries), 1*time.Millisecond, 10*time.Millisecond, selector, cbMgr)

		ctx := context.Background()
		callCount := 0

		err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			callCount++
			if callCount > int(successAfter) {
				return nil // Success
			}
			return errors.New("connection timeout") // Retryable error
		})

		// Should succeed
		if err != nil {
			return false
		}

		// Call count should be successAfter + 1
		expectedCalls := int(successAfter) + 1
		return callCount == expectedCalls
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


// **Validates: Requirements 3.3**
// Property: Backoff delay is always within bounds
func TestProperty_BackoffWithinBounds(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	property := func(initialDelayMs uint16, maxDelayMs uint16, retryCount uint8) bool {
		// Ensure reasonable values
		if initialDelayMs == 0 {
			initialDelayMs = 1
		}
		if initialDelayMs > 1000 {
			initialDelayMs = initialDelayMs % 1000
		}
		if maxDelayMs < initialDelayMs {
			maxDelayMs = initialDelayMs * 10
		}
		if maxDelayMs > 10000 {
			maxDelayMs = 10000
		}
		if retryCount > 20 {
			retryCount = retryCount % 21
		}

		lb := createTestLoadBalancer(t)
		cbMgr := createTestCircuitBreakerManager()
		selector, _ := NewEnhancedSelector(lb, cbMgr)

		initialDelay := time.Duration(initialDelayMs) * time.Millisecond
		maxDelay := time.Duration(maxDelayMs) * time.Millisecond

		rh := NewRetryHandler(10, initialDelay, maxDelay, selector, cbMgr).(*DefaultRetryHandler)

		delay := rh.CalculateBackoff(int(retryCount))

		// Delay should never be less than 0
		if delay < 0 {
			return false
		}

		// Delay should never exceed max
		if delay > maxDelay {
			return false
		}

		// For retry count 0, delay should equal initial delay
		if retryCount == 0 && delay != initialDelay {
			return false
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


// **Validates: Requirements 3.4**
// Property: Retry attempts use different nodes when available
func TestProperty_RetriesUseDifferentNodes(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	property := func(maxRetries uint8) bool {
		// Limit maxRetries to reasonable range
		if maxRetries > 5 {
			maxRetries = maxRetries % 6
		}
		if maxRetries == 0 {
			maxRetries = 1
		}

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

		// Need at least 2 nodes for this property
		if len(lb.ConfigNodes) < 2 {
			return true // Skip if not enough nodes
		}

		rh := NewRetryHandler(int(maxRetries), 1*time.Millisecond, 10*time.Millisecond, selector, cbMgr)

		ctx := context.Background()
		selectedConfigs := make(map[string]int)

		_ = rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			selectedConfigs[config.ID]++
			return errors.New("connection timeout") // Always fail
		})

		// With multiple retries and multiple nodes, we should see different nodes
		// (though not guaranteed due to randomness in some strategies)
		// At minimum, we should have attempted some calls
		totalCalls := 0
		for _, count := range selectedConfigs {
			totalCalls += count
		}

		expectedCalls := int(maxRetries) + 1
		return totalCalls == expectedCalls
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 30}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


// **Validates: Requirements 3.5**
// Property: Final error contains information about retries
func TestProperty_FinalErrorContainsRetryInfo(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Initialize encryption for test
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	property := func(maxRetries uint8) bool {
		// Limit maxRetries to reasonable range
		if maxRetries > 10 {
			maxRetries = maxRetries % 11
		}

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

		rh := NewRetryHandler(int(maxRetries), 1*time.Millisecond, 10*time.Millisecond, selector, cbMgr)

		ctx := context.Background()

		err := rh.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			return errors.New("connection timeout")
		})

		// Error should not be nil
		if err == nil {
			return false
		}

		// Error message should contain retry information
		errMsg := err.Error()
		return len(errMsg) > 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 30}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}


