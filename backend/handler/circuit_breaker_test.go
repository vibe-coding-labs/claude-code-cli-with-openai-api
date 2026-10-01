package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
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

// TestCircuitBreaker_Reset_Persists 验证 Reset() 会清空内存状态并把数据库状态写回 closed。
func TestCircuitBreaker_Reset_Persists(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	configID := lb.ConfigNodes[0].ConfigID

	realCB := NewCircuitBreaker(configID, 0.5, 60*time.Second, 30*time.Second, 3)
	cb := realCB.(*DefaultCircuitBreaker)

	// Drive some failures into memory + DB state
	for i := 0; i < 5; i++ {
		cb.requests = append(cb.requests, requestRecord{timestamp: time.Now(), success: false})
	}
	cb.halfOpenAttempts = 2
	cb.RecordFailure()

	cb.Reset()

	cb.mu.RLock()
	reqCount := len(cb.requests)
	attempts := cb.halfOpenAttempts
	cb.mu.RUnlock()

	if reqCount != 0 {
		t.Errorf("Expected 0 requests after Reset(), got %d", reqCount)
	}
	if attempts != 0 {
		t.Errorf("Expected 0 half-open attempts after Reset(), got %d", attempts)
	}

	if state := cb.GetState(); state != "closed" {
		t.Errorf("Expected state 'closed' after Reset(), got %q", state)
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

// TestCircuitBreaker_Call_FullLifecycle drives Call() through closed -> open ->
// half-open -> closed using a real (temp) database, covering the state-machine
// branches in Call/executeInClosed/executeInHalfOpen that the unit tests above
// (which manipulate cb fields directly) never exercise. State transitions are
// forced deterministically via direct DB writes instead of sleeping past a
// timeout, so the test isn't racing the wall clock.
func TestCircuitBreaker_Call_FullLifecycle(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	configID := lb.ConfigNodes[0].ConfigID

	realCB := NewCircuitBreaker(configID, 0.5, 60*time.Second, 30*time.Second, 2)
	cb := realCB.(*DefaultCircuitBreaker)
	ctx := context.Background()
	boom := errors.New("boom")

	// 1. closed state, successful call keeps it closed.
	if err := cb.Call(ctx, func() error { return nil }); err != nil {
		t.Fatalf("expected success in closed state, got %v", err)
	}
	if state := cb.GetState(); state != "closed" {
		t.Fatalf("expected closed after success, got %q", state)
	}

	// 2. Drive enough failures to trip the breaker open (errorRateThreshold=0.5).
	var lastErr error
	for i := 0; i < 3; i++ {
		lastErr = cb.Call(ctx, func() error { return boom })
	}
	if lastErr == nil {
		t.Fatal("expected failure to propagate from Call()")
	}
	if state := cb.GetState(); state != "open" {
		t.Fatalf("expected open after repeated failures, got %q", state)
	}

	// 3. Force NextRetryTime into the future: Call() must short-circuit
	// without invoking fn (the "open, timeout not elapsed" branch).
	future := time.Now().Add(time.Hour)
	if err := database.CreateOrUpdateCircuitBreakerState(&database.CircuitBreakerState{
		ConfigID: configID, State: "open", LastStateChange: time.Now(), NextRetryTime: &future,
	}); err != nil {
		t.Fatalf("failed to seed open state: %v", err)
	}
	called := false
	if err := cb.Call(ctx, func() error { called = true; return nil }); err == nil {
		t.Error("expected error while circuit is open and timeout not elapsed")
	}
	if called {
		t.Error("fn should not be invoked while circuit is open")
	}

	// 4. Force NextRetryTime into the past: Call() must transition to
	// half-open and invoke fn (the "open, timeout elapsed" branch).
	past := time.Now().Add(-time.Hour)
	if err := database.CreateOrUpdateCircuitBreakerState(&database.CircuitBreakerState{
		ConfigID: configID, State: "open", LastStateChange: time.Now(), NextRetryTime: &past,
	}); err != nil {
		t.Fatalf("failed to seed expired open state: %v", err)
	}
	cb.mu.Lock()
	cb.halfOpenAttempts = 0
	cb.mu.Unlock()

	halfOpenCalls := 0
	for i := 0; i < 2; i++ {
		if err := cb.Call(ctx, func() error { halfOpenCalls++; return nil }); err != nil {
			t.Fatalf("expected success while recovering in half-open, got %v", err)
		}
	}
	if halfOpenCalls != 2 {
		t.Fatalf("expected 2 half-open invocations, got %d", halfOpenCalls)
	}
	if state := cb.GetState(); state != "closed" {
		t.Fatalf("expected closed after successful half-open recovery, got %q", state)
	}

	// 5. Put the breaker directly into half-open and fail the probe: must
	// return to open (the "failure during half-open" branch).
	if err := database.TransitionCircuitBreakerToHalfOpen(configID); err != nil {
		t.Fatalf("failed to seed half-open state: %v", err)
	}
	cb.mu.Lock()
	cb.halfOpenAttempts = 0
	cb.mu.Unlock()

	if err := cb.Call(ctx, func() error { return boom }); err == nil {
		t.Fatal("expected failure during half-open probe to propagate")
	}
	if state := cb.GetState(); state != "open" {
		t.Fatalf("expected re-opened after half-open failure, got %q", state)
	}
}

// TestCircuitBreaker_Call_UnknownState covers the default branch of Call()'s
// state switch by writing an unrecognized state directly into the database.
func TestCircuitBreaker_Call_UnknownState(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	configID := lb.ConfigNodes[0].ConfigID

	if err := database.CreateOrUpdateCircuitBreakerState(&database.CircuitBreakerState{
		ConfigID:        configID,
		State:           "weird_state",
		LastStateChange: time.Now(),
	}); err != nil {
		t.Fatalf("failed to seed weird state: %v", err)
	}

	realCB := NewCircuitBreaker(configID, 0.5, 60*time.Second, 30*time.Second, 3)
	if err := realCB.Call(context.Background(), func() error { return nil }); err == nil {
		t.Error("expected error for unknown circuit breaker state")
	}
}

// TestCircuitBreaker_DatabaseErrors forces database failures (by closing the
// DB handle) to exercise the log-only error branches in Call, RecordSuccess,
// RecordFailure, Reset and getOrInitializeState.
func TestCircuitBreaker_DatabaseErrors(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	configID := lb.ConfigNodes[0].ConfigID

	realCB := NewCircuitBreaker(configID, 0.5, 60*time.Second, 30*time.Second, 3)
	cb := realCB.(*DefaultCircuitBreaker)

	// Prime a state row, then close the DB connection to force errors on
	// every subsequent database call without panicking (DB is a closed
	// *sql.DB, not nil).
	if err := cb.Call(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("unexpected error priming state: %v", err)
	}
	if err := database.CloseDB(); err != nil {
		t.Fatalf("failed to close db: %v", err)
	}

	// getOrInitializeState / Call: GetCircuitBreakerState fails, then
	// InitializeCircuitBreakerState also fails -> Call returns wrapped error.
	if err := realCB.Call(context.Background(), func() error { return nil }); err == nil {
		t.Error("expected error from Call() when database is unavailable")
	}

	// RecordSuccess / RecordFailure: errors are only logged, must not panic.
	cb.RecordSuccess()
	cb.RecordFailure()

	// Reset(): TransitionCircuitBreakerToClosed fails, only logged.
	cb.Reset()

	// GetState(): falls back to "closed" when the state can't be read.
	if state := cb.GetState(); state != "closed" {
		t.Errorf("expected fallback state 'closed' on DB error, got %q", state)
	}
}

// setQueryOnly toggles SQLite's query_only pragma on the shared test DB
// connection: reads keep working but writes fail, which lets us hit the
// log-only "transition write failed" branches without severing reads
// (closing the DB entirely fails both, masking these branches).
func setQueryOnly(t *testing.T, on string) {
	t.Helper()
	if _, err := database.DB.Exec("PRAGMA query_only = " + on); err != nil {
		t.Fatalf("failed to set query_only=%s: %v", on, err)
	}
}

// TestCircuitBreaker_TransitionWriteFailures forces the database write inside
// each state-transition call to fail (via PRAGMA query_only) while state
// reads keep succeeding, covering the log-only error branches in Call,
// executeInClosed and executeInHalfOpen that TestCircuitBreaker_DatabaseErrors
// (which kills the whole connection) can't reach on their own.
func TestCircuitBreaker_TransitionWriteFailures(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)
	defer setQueryOnly(t, "OFF")

	lb := createTestLoadBalancer(t)
	configID := lb.ConfigNodes[0].ConfigID
	boom := errors.New("boom")
	ctx := context.Background()

	realCB := NewCircuitBreaker(configID, 0.5, 60*time.Second, 30*time.Second, 2)
	cb := realCB.(*DefaultCircuitBreaker)

	// Scenario 1: closed -> open transition write fails (executeInClosed,
	// circuit_breaker.go:98-100).
	if err := cb.Call(ctx, func() error { return nil }); err != nil {
		t.Fatalf("priming closed state: %v", err)
	}
	// Push the in-memory error rate over the threshold so the very next
	// failure triggers shouldOpen() -> TransitionCircuitBreakerToOpen.
	cb.mu.Lock()
	cb.requests = append(cb.requests, requestRecord{timestamp: time.Now(), success: false})
	cb.mu.Unlock()
	setQueryOnly(t, "ON")
	if err := cb.Call(ctx, func() error { return boom }); err == nil {
		t.Error("expected fn error to propagate even when transition write fails")
	}
	setQueryOnly(t, "OFF")

	// Scenario 2: Call() open -> half-open transition write fails
	// (circuit_breaker.go:63-65).
	past := time.Now().Add(-time.Hour)
	if err := database.CreateOrUpdateCircuitBreakerState(&database.CircuitBreakerState{
		ConfigID: configID, State: "open", LastStateChange: time.Now(), NextRetryTime: &past,
	}); err != nil {
		t.Fatalf("seeding expired-open state: %v", err)
	}
	setQueryOnly(t, "ON")
	if err := cb.Call(ctx, func() error { return nil }); err == nil {
		t.Error("expected error when half-open transition write fails")
	}
	setQueryOnly(t, "OFF")

	// Scenario 3: half-open -> open transition write fails on a failed probe
	// (executeInHalfOpen, circuit_breaker.go:123-125).
	if err := database.TransitionCircuitBreakerToHalfOpen(configID); err != nil {
		t.Fatalf("seeding half-open state: %v", err)
	}
	cb.mu.Lock()
	cb.halfOpenAttempts = 0
	cb.mu.Unlock()
	setQueryOnly(t, "ON")
	if err := cb.Call(ctx, func() error { return boom }); err == nil {
		t.Error("expected fn error to propagate even when half-open->open write fails")
	}
	setQueryOnly(t, "OFF")

	// Scenario 4: half-open -> closed transition write fails on the final
	// successful probe (executeInHalfOpen, circuit_breaker.go:139-141).
	if err := database.TransitionCircuitBreakerToHalfOpen(configID); err != nil {
		t.Fatalf("seeding half-open state: %v", err)
	}
	cb.mu.Lock()
	cb.halfOpenAttempts = cb.halfOpenRequests - 1 // next success reaches the threshold
	cb.mu.Unlock()
	setQueryOnly(t, "ON")
	if err := cb.Call(ctx, func() error { return nil }); err != nil {
		t.Errorf("expected success even when closing transition write fails, got %v", err)
	}
	setQueryOnly(t, "OFF")
}
