package handler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// CircuitBreaker interface defines circuit breaker operations
type CircuitBreaker interface {
	Call(ctx context.Context, fn func() error) error
	GetState() string
	RecordSuccess()
	RecordFailure()
	Reset()
}

// DefaultCircuitBreaker implements the CircuitBreaker interface
type DefaultCircuitBreaker struct {
	configID          string
	errorRateThreshold float64
	windowDuration    time.Duration
	timeout           time.Duration
	halfOpenRequests  int
	mu                sync.RWMutex
	requests          []requestRecord
	halfOpenAttempts  int
}

type requestRecord struct {
	timestamp time.Time
	success   bool
}

// NewCircuitBreaker creates a new circuit breaker instance
func NewCircuitBreaker(configID string, errorRateThreshold float64, windowDuration, timeout time.Duration, halfOpenRequests int) CircuitBreaker {
	return &DefaultCircuitBreaker{
		configID:           configID,
		errorRateThreshold: errorRateThreshold,
		windowDuration:     windowDuration,
		timeout:            timeout,
		halfOpenRequests:   halfOpenRequests,
		requests:           make([]requestRecord, 0),
	}
}

// Call executes a function with circuit breaker protection
func (cb *DefaultCircuitBreaker) Call(ctx context.Context, fn func() error) error {
	state, err := cb.getOrInitializeState()
	if err != nil {
		return fmt.Errorf("failed to get circuit breaker state: %w", err)
	}

	switch state.State {
	case "open":
		// Check if timeout has passed
		if state.NextRetryTime != nil && time.Now().After(*state.NextRetryTime) {
			// Transition to half-open
			if err := database.TransitionCircuitBreakerToHalfOpen(cb.configID); err != nil {
				return fmt.Errorf("failed to transition to half-open: %w", err)
			}
			log.Printf("Circuit breaker for %s transitioned to half-open", cb.configID)
			return cb.executeInHalfOpen(ctx, fn)
		}
		return fmt.Errorf("circuit breaker is open")

	case "half_open":
		return cb.executeInHalfOpen(ctx, fn)

	case "closed":
		return cb.executeInClosed(ctx, fn)

	default:
		return fmt.Errorf("unknown circuit breaker state: %s", state.State)
	}
}

// executeInClosed executes the function in closed state
func (cb *DefaultCircuitBreaker) executeInClosed(ctx context.Context, fn func() error) error {
	err := fn()

	cb.mu.Lock()
	cb.requests = append(cb.requests, requestRecord{
		timestamp: time.Now(),
		success:   err == nil,
	})
	cb.cleanOldRequests()
	cb.mu.Unlock()

	if err != nil {
		cb.RecordFailure()
		// Check if we should open the circuit
		if cb.shouldOpen() {
			if err := database.TransitionCircuitBreakerToOpen(cb.configID, int(cb.timeout.Seconds())); err != nil {
				log.Printf("Failed to transition circuit breaker to open: %v", err)
			} else {
				log.Printf("Circuit breaker for %s opened due to high error rate", cb.configID)
			}
		}
		return err
	}

	cb.RecordSuccess()
	return nil
}

// executeInHalfOpen executes the function in half-open state
func (cb *DefaultCircuitBreaker) executeInHalfOpen(ctx context.Context, fn func() error) error {
	cb.mu.Lock()
	cb.halfOpenAttempts++
	attempts := cb.halfOpenAttempts
	cb.mu.Unlock()

	err := fn()

	if err != nil {
		// Failure in half-open state - go back to open
		cb.RecordFailure()
		if err := database.TransitionCircuitBreakerToOpen(cb.configID, int(cb.timeout.Seconds())); err != nil {
			log.Printf("Failed to transition circuit breaker to open: %v", err)
		} else {
			log.Printf("Circuit breaker for %s returned to open state after failure", cb.configID)
		}
		cb.mu.Lock()
		cb.halfOpenAttempts = 0
		cb.mu.Unlock()
		return err
	}

	// Success in half-open state
	cb.RecordSuccess()

	// Check if we've had enough successful attempts to close the circuit
	if attempts >= cb.halfOpenRequests {
		if err := database.TransitionCircuitBreakerToClosed(cb.configID); err != nil {
			log.Printf("Failed to transition circuit breaker to closed: %v", err)
		} else {
			log.Printf("Circuit breaker for %s closed after successful recovery", cb.configID)
		}
		cb.mu.Lock()
		cb.halfOpenAttempts = 0
		cb.requests = make([]requestRecord, 0)
		cb.mu.Unlock()
	}

	return nil
}

// shouldOpen determines if the circuit should open based on error rate
func (cb *DefaultCircuitBreaker) shouldOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if len(cb.requests) == 0 {
		return false
	}

	totalRequests := len(cb.requests)
	failedRequests := 0
	for _, req := range cb.requests {
		if !req.success {
			failedRequests++
		}
	}

	errorRate := float64(failedRequests) / float64(totalRequests)
	return errorRate > cb.errorRateThreshold
}

// cleanOldRequests removes requests outside the time window
func (cb *DefaultCircuitBreaker) cleanOldRequests() {
	cutoff := time.Now().Add(-cb.windowDuration)
	validRequests := make([]requestRecord, 0)
	for _, req := range cb.requests {
		if req.timestamp.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	cb.requests = validRequests
}

// GetState returns the current circuit breaker state
func (cb *DefaultCircuitBreaker) GetState() string {
	state, err := database.GetCircuitBreakerState(cb.configID)
	if err != nil {
		return "closed" // Default to closed if not found
	}
	return state.State
}

// RecordSuccess records a successful request
func (cb *DefaultCircuitBreaker) RecordSuccess() {
	if err := database.RecordCircuitBreakerSuccess(cb.configID); err != nil {
		log.Printf("Failed to record circuit breaker success: %v", err)
	}
}

// RecordFailure records a failed request
func (cb *DefaultCircuitBreaker) RecordFailure() {
	if err := database.RecordCircuitBreakerFailure(cb.configID); err != nil {
		log.Printf("Failed to record circuit breaker failure: %v", err)
	}
}

// Reset resets the circuit breaker to closed state
func (cb *DefaultCircuitBreaker) Reset() {
	cb.mu.Lock()
	cb.requests = make([]requestRecord, 0)
	cb.halfOpenAttempts = 0
	cb.mu.Unlock()

	if err := database.TransitionCircuitBreakerToClosed(cb.configID); err != nil {
		log.Printf("Failed to reset circuit breaker: %v", err)
	}
}

// getOrInitializeState gets the circuit breaker state or initializes it
func (cb *DefaultCircuitBreaker) getOrInitializeState() (*database.CircuitBreakerState, error) {
	state, err := database.GetCircuitBreakerState(cb.configID)
	if err != nil {
		// Initialize if doesn't exist
		if err := database.InitializeCircuitBreakerState(cb.configID); err != nil {
			return nil, err
		}
		return database.GetCircuitBreakerState(cb.configID)
	}
	return state, nil
}

// CircuitBreakerManager manages circuit breakers for multiple configs
type CircuitBreakerManager struct {
	breakers map[string]CircuitBreaker
	mu       sync.RWMutex
	config   CircuitBreakerConfig
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	ErrorRateThreshold float64
	WindowDuration     time.Duration
	Timeout            time.Duration
	HalfOpenRequests   int
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(config CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]CircuitBreaker),
		config:   config,
	}
}

// GetCircuitBreaker gets or creates a circuit breaker for a config
func (cbm *CircuitBreakerManager) GetCircuitBreaker(configID string) CircuitBreaker {
	cbm.mu.RLock()
	cb, exists := cbm.breakers[configID]
	cbm.mu.RUnlock()

	if exists {
		return cb
	}

	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := cbm.breakers[configID]; exists {
		return cb
	}

	// Create new circuit breaker
	cb = NewCircuitBreaker(
		configID,
		cbm.config.ErrorRateThreshold,
		cbm.config.WindowDuration,
		cbm.config.Timeout,
		cbm.config.HalfOpenRequests,
	)
	cbm.breakers[configID] = cb

	return cb
}
