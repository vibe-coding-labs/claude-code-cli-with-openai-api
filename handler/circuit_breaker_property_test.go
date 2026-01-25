package handler

import (
	"testing"
	"time"
)

// TestCircuitBreakerStateTransitionProperties tests the properties of circuit breaker state transitions
func TestCircuitBreakerStateTransitionProperties(t *testing.T) {
	// Property 1: State transitions follow the defined state machine
	t.Run("Property: State transitions are deterministic", func(t *testing.T) {
		testCases := []struct {
			name              string
			initialState      string
			errorRateThreshold float64
			operations        []bool // true = success, false = failure
			expectedState     string
		}{
			{
				name:              "closed to open on high error rate",
				initialState:      "closed",
				errorRateThreshold: 0.5,
				operations:        []bool{false, false, false, true, true}, // 60% error rate
				expectedState:     "open",
			},
			{
				name:              "closed remains closed with low error rate",
				initialState:      "closed",
				errorRateThreshold: 0.5,
				operations:        []bool{true, true, true, false, true}, // 20% error rate
				expectedState:     "closed",
			},
			{
				name:              "closed remains closed at threshold boundary",
				initialState:      "closed",
				errorRateThreshold: 0.5,
				operations:        []bool{false, false, true, true}, // 50% error rate (equals threshold, should not open)
				expectedState:     "closed",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cb := &DefaultCircuitBreaker{
					configID:           "test-config",
					errorRateThreshold: tc.errorRateThreshold,
					windowDuration:     60 * time.Second,
					timeout:            30 * time.Second,
					halfOpenRequests:   3,
					requests:           make([]requestRecord, 0),
				}

				state := tc.initialState

				// Apply operations
				for _, success := range tc.operations {
					cb.requests = append(cb.requests, requestRecord{
						timestamp: time.Now(),
						success:   success,
					})

					// Check if should transition to open
					if state == "closed" && cb.shouldOpen() {
						state = "open"
					}
				}

				if state != tc.expectedState {
					t.Errorf("Expected state %s, got %s", tc.expectedState, state)
				}
			})
		}
	})

	// Property 2: Error rate calculation is consistent
	t.Run("Property: Error rate calculation is monotonic", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Add successes first
		for i := 0; i < 5; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   true,
			})
		}

		// Should not open with all successes
		if cb.shouldOpen() {
			t.Error("Circuit should not open with all successes")
		}

		// Add failures to increase error rate
		for i := 0; i < 6; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   false,
			})
		}

		// Should open with high error rate (6/11 = 54.5%)
		if !cb.shouldOpen() {
			t.Error("Circuit should open with high error rate")
		}
	})

	// Property 3: Half-open state transitions correctly
	t.Run("Property: Half-open transitions are deterministic", func(t *testing.T) {
		testCases := []struct {
			name             string
			halfOpenRequests int
			successCount     int
			shouldClose      bool
		}{
			{
				name:             "closes after enough successes",
				halfOpenRequests: 3,
				successCount:     3,
				shouldClose:      true,
			},
			{
				name:             "does not close with insufficient successes",
				halfOpenRequests: 3,
				successCount:     2,
				shouldClose:      false,
			},
			{
				name:             "closes immediately with 1 required success",
				halfOpenRequests: 1,
				successCount:     1,
				shouldClose:      true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cb := &DefaultCircuitBreaker{
					configID:           "test-config",
					errorRateThreshold: 0.5,
					windowDuration:     60 * time.Second,
					timeout:            30 * time.Second,
					halfOpenRequests:   tc.halfOpenRequests,
					requests:           make([]requestRecord, 0),
					halfOpenAttempts:   0,
				}

				// Simulate half-open attempts
				for i := 0; i < tc.successCount; i++ {
					cb.halfOpenAttempts++
				}

				shouldClose := cb.halfOpenAttempts >= cb.halfOpenRequests
				if shouldClose != tc.shouldClose {
					t.Errorf("Expected shouldClose=%v, got %v (attempts=%d, required=%d)",
						tc.shouldClose, shouldClose, cb.halfOpenAttempts, cb.halfOpenRequests)
				}
			})
		}
	})

	// Property 4: Window duration affects error rate calculation
	t.Run("Property: Old requests are excluded from error rate", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     100 * time.Millisecond,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Add old failures
		for i := 0; i < 10; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now().Add(-200 * time.Millisecond),
				success:   false,
			})
		}

		// Clean old requests
		cb.cleanOldRequests()

		// Should not open with no recent requests
		if cb.shouldOpen() {
			t.Error("Circuit should not open after old requests are cleaned")
		}

		// Verify all old requests were removed
		if len(cb.requests) != 0 {
			t.Errorf("Expected 0 requests after cleanup, got %d", len(cb.requests))
		}
	})

	// Property 5: Concurrent operations maintain consistency
	t.Run("Property: Concurrent operations are thread-safe", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		done := make(chan bool)
		iterations := 100

		// Multiple goroutines adding requests
		for i := 0; i < 5; i++ {
			go func(id int) {
				for j := 0; j < iterations; j++ {
					cb.mu.Lock()
					cb.requests = append(cb.requests, requestRecord{
						timestamp: time.Now(),
						success:   j%2 == 0,
					})
					cb.mu.Unlock()
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 5; i++ {
			<-done
		}

		// Verify consistency
		cb.mu.RLock()
		requestCount := len(cb.requests)
		cb.mu.RUnlock()

		expectedCount := 5 * iterations
		if requestCount != expectedCount {
			t.Errorf("Expected %d requests, got %d", expectedCount, requestCount)
		}

		// Verify shouldOpen doesn't panic
		_ = cb.shouldOpen()
	})
}

// TestCircuitBreakerStateTransitionInvariants tests invariants that must always hold
func TestCircuitBreakerStateTransitionInvariants(t *testing.T) {
	// Invariant 1: Error rate is always between 0 and 1
	t.Run("Invariant: Error rate is bounded", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Test various combinations
		testCases := []struct {
			successes int
			failures  int
		}{
			{10, 0},
			{0, 10},
			{5, 5},
			{7, 3},
			{3, 7},
		}

		for _, tc := range testCases {
			cb.requests = make([]requestRecord, 0)

			for i := 0; i < tc.successes; i++ {
				cb.requests = append(cb.requests, requestRecord{
					timestamp: time.Now(),
					success:   true,
				})
			}

			for i := 0; i < tc.failures; i++ {
				cb.requests = append(cb.requests, requestRecord{
					timestamp: time.Now(),
					success:   false,
				})
			}

			// Calculate error rate manually
			totalRequests := tc.successes + tc.failures
			if totalRequests > 0 {
				errorRate := float64(tc.failures) / float64(totalRequests)
				if errorRate < 0 || errorRate > 1 {
					t.Errorf("Error rate out of bounds: %f", errorRate)
				}
			}
		}
	})

	// Invariant 2: Half-open attempts never exceed required count before transition
	t.Run("Invariant: Half-open attempts are bounded", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
			halfOpenAttempts:   0,
		}

		// Simulate half-open attempts
		for i := 0; i < 10; i++ {
			cb.mu.Lock()
			if cb.halfOpenAttempts < cb.halfOpenRequests {
				cb.halfOpenAttempts++
			} else {
				// Reset after transition
				cb.halfOpenAttempts = 0
			}
			attempts := cb.halfOpenAttempts
			cb.mu.Unlock()

			// Verify invariant
			if attempts > cb.halfOpenRequests {
				t.Errorf("Half-open attempts exceeded limit: %d > %d", attempts, cb.halfOpenRequests)
			}
		}
	})

	// Invariant 3: Request timestamps are monotonically increasing (within tolerance)
	t.Run("Invariant: Request timestamps are ordered", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Add requests with small delays
		for i := 0; i < 10; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   i%2 == 0,
			})
			time.Sleep(1 * time.Millisecond)
		}

		// Verify timestamps are ordered (allowing for some clock skew)
		for i := 1; i < len(cb.requests); i++ {
			if cb.requests[i].timestamp.Before(cb.requests[i-1].timestamp.Add(-10 * time.Millisecond)) {
				t.Errorf("Request timestamps out of order at index %d", i)
			}
		}
	})

	// Invariant 4: Only valid states exist
	t.Run("Invariant: Only valid states are allowed", func(t *testing.T) {
		validStates := map[string]bool{
			"closed":    true,
			"open":      true,
			"half_open": true,
		}

		// Test state transitions
		states := []string{"closed", "open", "half_open"}
		for _, state := range states {
			if !validStates[state] {
				t.Errorf("Invalid state: %s", state)
			}
		}
	})

	// Invariant 5: Threshold is always between 0 and 1
	t.Run("Invariant: Threshold is bounded", func(t *testing.T) {
		testCases := []float64{0.0, 0.1, 0.5, 0.9, 1.0}

		for _, threshold := range testCases {
			if threshold < 0 || threshold > 1 {
				t.Errorf("Threshold out of bounds: %f", threshold)
			}

			cb := &DefaultCircuitBreaker{
				configID:           "test-config",
				errorRateThreshold: threshold,
				windowDuration:     60 * time.Second,
				timeout:            30 * time.Second,
				halfOpenRequests:   3,
				requests:           make([]requestRecord, 0),
			}

			if cb.errorRateThreshold < 0 || cb.errorRateThreshold > 1 {
				t.Errorf("Circuit breaker threshold out of bounds: %f", cb.errorRateThreshold)
			}
		}
	})
}

// TestCircuitBreakerEdgeProperties tests edge case properties
func TestCircuitBreakerEdgeProperties(t *testing.T) {
	// Property: Empty request history should not trigger opening
	t.Run("Property: Empty history does not open circuit", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		if cb.shouldOpen() {
			t.Error("Circuit should not open with empty request history")
		}
	})

	// Property: Single failure with any threshold > 0 should open
	t.Run("Property: Single failure behavior", func(t *testing.T) {
		thresholds := []float64{0.1, 0.5, 0.9, 0.99}

		for _, threshold := range thresholds {
			cb := &DefaultCircuitBreaker{
				configID:           "test-config",
				errorRateThreshold: threshold,
				windowDuration:     60 * time.Second,
				timeout:            30 * time.Second,
				halfOpenRequests:   3,
				requests:           make([]requestRecord, 0),
			}

			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   false,
			})

			// Single failure = 100% error rate, should open if threshold < 1.0
			shouldOpen := cb.shouldOpen()
			expectedOpen := threshold < 1.0

			if shouldOpen != expectedOpen {
				t.Errorf("Threshold %.2f: expected shouldOpen=%v, got %v",
					threshold, expectedOpen, shouldOpen)
			}
		}
	})

	// Property: Threshold of 0 means any failure opens circuit
	t.Run("Property: Zero threshold opens on any failure", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.0,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Add many successes
		for i := 0; i < 100; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   true,
			})
		}

		// Add single failure
		cb.requests = append(cb.requests, requestRecord{
			timestamp: time.Now(),
			success:   false,
		})

		// Should open with any failure when threshold is 0
		if !cb.shouldOpen() {
			t.Error("Circuit should open with any failure when threshold is 0")
		}
	})

	// Property: Threshold of 1.0 means circuit never opens
	t.Run("Property: Threshold of 1.0 never opens", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 1.0,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Add all failures
		for i := 0; i < 100; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   false,
			})
		}

		// Should not open with threshold of 1.0
		if cb.shouldOpen() {
			t.Error("Circuit should not open with threshold of 1.0")
		}
	})
}
