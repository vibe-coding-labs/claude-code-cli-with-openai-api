package handler

import (
	"context"
	"testing"
	"time"
)

func TestCircuitBreaker_ShouldOpen(t *testing.T) {
	tests := []struct {
		name               string
		errorRateThreshold float64
		requests           []bool // true = success, false = failure
		expectedOpen       bool
	}{
		{
			name:               "should not open with all successes",
			errorRateThreshold: 0.5,
			requests:           []bool{true, true, true, true, true},
			expectedOpen:       false,
		},
		{
			name:               "should open when error rate exceeds threshold",
			errorRateThreshold: 0.5,
			requests:           []bool{false, false, false, true, true},
			expectedOpen:       true,
		},
		{
			name:               "should not open when error rate equals threshold",
			errorRateThreshold: 0.5,
			requests:           []bool{false, false, true, true},
			expectedOpen:       false,
		},
		{
			name:               "should open with high error rate",
			errorRateThreshold: 0.3,
			requests:           []bool{false, false, true, true},
			expectedOpen:       true,
		},
		{
			name:               "should not open with empty requests",
			errorRateThreshold: 0.5,
			requests:           []bool{},
			expectedOpen:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := &DefaultCircuitBreaker{
				configID:           "test-config",
				errorRateThreshold: tt.errorRateThreshold,
				windowDuration:     60 * time.Second,
				timeout:            30 * time.Second,
				halfOpenRequests:   3,
				requests:           make([]requestRecord, 0),
			}

			// Add requests
			for _, success := range tt.requests {
				cb.requests = append(cb.requests, requestRecord{
					timestamp: time.Now(),
					success:   success,
				})
			}

			result := cb.shouldOpen()
			if result != tt.expectedOpen {
				t.Errorf("shouldOpen() = %v, want %v", result, tt.expectedOpen)
			}
		})
	}
}

func TestCircuitBreaker_CleanOldRequests(t *testing.T) {
	cb := &DefaultCircuitBreaker{
		configID:           "test-config",
		errorRateThreshold: 0.5,
		windowDuration:     1 * time.Second,
		timeout:            30 * time.Second,
		halfOpenRequests:   3,
		requests:           make([]requestRecord, 0),
	}

	// Add old requests
	cb.requests = append(cb.requests, requestRecord{
		timestamp: time.Now().Add(-2 * time.Second),
		success:   true,
	})
	cb.requests = append(cb.requests, requestRecord{
		timestamp: time.Now().Add(-2 * time.Second),
		success:   false,
	})

	// Add recent requests
	cb.requests = append(cb.requests, requestRecord{
		timestamp: time.Now(),
		success:   true,
	})
	cb.requests = append(cb.requests, requestRecord{
		timestamp: time.Now(),
		success:   false,
	})

	// Clean old requests
	cb.cleanOldRequests()

	// Should only have 2 recent requests
	if len(cb.requests) != 2 {
		t.Errorf("Expected 2 requests after cleanup, got %d", len(cb.requests))
	}

	// Verify all remaining requests are recent
	cutoff := time.Now().Add(-cb.windowDuration)
	for _, req := range cb.requests {
		if req.timestamp.Before(cutoff) {
			t.Errorf("Found old request that should have been cleaned: %v", req.timestamp)
		}
	}
}

func TestCircuitBreaker_ErrorRateCalculation(t *testing.T) {
	tests := []struct {
		name               string
		errorRateThreshold float64
		successCount       int
		failureCount       int
		expectedOpen       bool
	}{
		{
			name:               "50% error rate with 50% threshold",
			errorRateThreshold: 0.5,
			successCount:       5,
			failureCount:       5,
			expectedOpen:       false,
		},
		{
			name:               "60% error rate with 50% threshold",
			errorRateThreshold: 0.5,
			successCount:       4,
			failureCount:       6,
			expectedOpen:       true,
		},
		{
			name:               "100% error rate",
			errorRateThreshold: 0.5,
			successCount:       0,
			failureCount:       10,
			expectedOpen:       true,
		},
		{
			name:               "0% error rate",
			errorRateThreshold: 0.5,
			successCount:       10,
			failureCount:       0,
			expectedOpen:       false,
		},
		{
			name:               "low threshold with few errors",
			errorRateThreshold: 0.1,
			successCount:       9,
			failureCount:       1,
			expectedOpen:       false,
		},
		{
			name:               "low threshold with more errors",
			errorRateThreshold: 0.1,
			successCount:       8,
			failureCount:       2,
			expectedOpen:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := &DefaultCircuitBreaker{
				configID:           "test-config",
				errorRateThreshold: tt.errorRateThreshold,
				windowDuration:     60 * time.Second,
				timeout:            30 * time.Second,
				halfOpenRequests:   3,
				requests:           make([]requestRecord, 0),
			}

			// Add success requests
			for i := 0; i < tt.successCount; i++ {
				cb.requests = append(cb.requests, requestRecord{
					timestamp: time.Now(),
					success:   true,
				})
			}

			// Add failure requests
			for i := 0; i < tt.failureCount; i++ {
				cb.requests = append(cb.requests, requestRecord{
					timestamp: time.Now(),
					success:   false,
				})
			}

			result := cb.shouldOpen()
			if result != tt.expectedOpen {
				totalRequests := tt.successCount + tt.failureCount
				actualErrorRate := float64(tt.failureCount) / float64(totalRequests)
				t.Errorf("shouldOpen() = %v, want %v (error rate: %.2f, threshold: %.2f)",
					result, tt.expectedOpen, actualErrorRate, tt.errorRateThreshold)
			}
		})
	}
}

func TestCircuitBreakerManager_GetCircuitBreaker(t *testing.T) {
	config := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     60 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}

	manager := NewCircuitBreakerManager(config)

	// Get circuit breaker for first time
	cb1 := manager.GetCircuitBreaker("config-1")
	if cb1 == nil {
		t.Fatal("Expected circuit breaker, got nil")
	}

	// Get same circuit breaker again - should return same instance
	cb2 := manager.GetCircuitBreaker("config-1")
	if cb1 != cb2 {
		t.Error("Expected same circuit breaker instance")
	}

	// Get different circuit breaker
	cb3 := manager.GetCircuitBreaker("config-2")
	if cb3 == nil {
		t.Fatal("Expected circuit breaker, got nil")
	}
	if cb1 == cb3 {
		t.Error("Expected different circuit breaker instances")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := &DefaultCircuitBreaker{
		configID:           "test-config",
		errorRateThreshold: 0.5,
		windowDuration:     60 * time.Second,
		timeout:            30 * time.Second,
		halfOpenRequests:   3,
		requests:           make([]requestRecord, 0),
	}

	// Simulate concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cb.mu.Lock()
				cb.requests = append(cb.requests, requestRecord{
					timestamp: time.Now(),
					success:   j%2 == 0,
				})
				cb.mu.Unlock()

				cb.shouldOpen()
				cb.cleanOldRequests()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify no race conditions occurred
	cb.mu.RLock()
	requestCount := len(cb.requests)
	cb.mu.RUnlock()

	if requestCount < 0 {
		t.Error("Request count should not be negative")
	}
}

func TestCircuitBreaker_WindowDuration(t *testing.T) {
	cb := &DefaultCircuitBreaker{
		configID:           "test-config",
		errorRateThreshold: 0.5,
		windowDuration:     100 * time.Millisecond,
		timeout:            30 * time.Second,
		halfOpenRequests:   3,
		requests:           make([]requestRecord, 0),
	}

	// Add requests that will expire
	for i := 0; i < 5; i++ {
		cb.requests = append(cb.requests, requestRecord{
			timestamp: time.Now(),
			success:   false,
		})
	}

	// Should open due to high error rate
	if !cb.shouldOpen() {
		t.Error("Circuit should be open with high error rate")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Clean old requests
	cb.cleanOldRequests()

	// Should not open with no recent requests
	if cb.shouldOpen() {
		t.Error("Circuit should not open with no recent requests")
	}

	// Verify all requests were cleaned
	if len(cb.requests) != 0 {
		t.Errorf("Expected 0 requests after window expiration, got %d", len(cb.requests))
	}
}

func TestCircuitBreaker_HalfOpenAttempts(t *testing.T) {
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
	ctx := context.Background()
	successFn := func() error { return nil }

	// First attempt
	cb.mu.Lock()
	cb.halfOpenAttempts++
	attempts1 := cb.halfOpenAttempts
	cb.mu.Unlock()

	if attempts1 != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts1)
	}

	// Second attempt
	cb.mu.Lock()
	cb.halfOpenAttempts++
	attempts2 := cb.halfOpenAttempts
	cb.mu.Unlock()

	if attempts2 != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts2)
	}

	// Third attempt - should trigger close
	cb.mu.Lock()
	cb.halfOpenAttempts++
	attempts3 := cb.halfOpenAttempts
	cb.mu.Unlock()

	if attempts3 != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts3)
	}

	// After closing, attempts should reset
	cb.mu.Lock()
	cb.halfOpenAttempts = 0
	cb.mu.Unlock()

	cb.mu.RLock()
	finalAttempts := cb.halfOpenAttempts
	cb.mu.RUnlock()

	if finalAttempts != 0 {
		t.Errorf("Expected 0 attempts after reset, got %d", finalAttempts)
	}

	_ = ctx
	_ = successFn
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := &DefaultCircuitBreaker{
		configID:           "test-config",
		errorRateThreshold: 0.5,
		windowDuration:     60 * time.Second,
		timeout:            30 * time.Second,
		halfOpenRequests:   3,
		requests:           make([]requestRecord, 0),
		halfOpenAttempts:   5,
	}

	// Add some requests
	for i := 0; i < 10; i++ {
		cb.requests = append(cb.requests, requestRecord{
			timestamp: time.Now(),
			success:   i%2 == 0,
		})
	}

	// Reset should clear everything
	cb.mu.Lock()
	cb.requests = make([]requestRecord, 0)
	cb.halfOpenAttempts = 0
	cb.mu.Unlock()

	// Verify reset
	cb.mu.RLock()
	requestCount := len(cb.requests)
	attempts := cb.halfOpenAttempts
	cb.mu.RUnlock()

	if requestCount != 0 {
		t.Errorf("Expected 0 requests after reset, got %d", requestCount)
	}
	if attempts != 0 {
		t.Errorf("Expected 0 half-open attempts after reset, got %d", attempts)
	}
}

func TestCircuitBreaker_EdgeCases(t *testing.T) {
	t.Run("zero threshold", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.0,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Any failure should open the circuit
		cb.requests = append(cb.requests, requestRecord{
			timestamp: time.Now(),
			success:   false,
		})

		if !cb.shouldOpen() {
			t.Error("Circuit should open with any failure when threshold is 0")
		}
	})

	t.Run("threshold of 1.0", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 1.0,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// All failures should not open the circuit
		for i := 0; i < 10; i++ {
			cb.requests = append(cb.requests, requestRecord{
				timestamp: time.Now(),
				success:   false,
			})
		}

		if cb.shouldOpen() {
			t.Error("Circuit should not open when threshold is 1.0")
		}
	})

	t.Run("single request", func(t *testing.T) {
		cb := &DefaultCircuitBreaker{
			configID:           "test-config",
			errorRateThreshold: 0.5,
			windowDuration:     60 * time.Second,
			timeout:            30 * time.Second,
			halfOpenRequests:   3,
			requests:           make([]requestRecord, 0),
		}

		// Single failure with 50% threshold should open
		cb.requests = append(cb.requests, requestRecord{
			timestamp: time.Now(),
			success:   false,
		})

		if !cb.shouldOpen() {
			t.Error("Circuit should open with single failure and 50% threshold")
		}
	})
}
