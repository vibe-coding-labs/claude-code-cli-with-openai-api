package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// RetryHandler interface defines retry operations
type RetryHandler interface {
	ExecuteWithRetry(ctx context.Context, fn func(config *database.APIConfig) error) error
	IsRetryableError(err error) bool
	CalculateBackoff(retryCount int) time.Duration
}

// DefaultRetryHandler implements the RetryHandler interface
type DefaultRetryHandler struct {
	maxRetries        int
	initialDelay      time.Duration
	maxDelay          time.Duration
	selector          Selector
	circuitBreakerMgr *CircuitBreakerManager
}

// NewRetryHandler creates a new retry handler instance
func NewRetryHandler(maxRetries int, initialDelay, maxDelay time.Duration, selector Selector, cbMgr *CircuitBreakerManager) RetryHandler {
	return &DefaultRetryHandler{
		maxRetries:        maxRetries,
		initialDelay:      initialDelay,
		maxDelay:          maxDelay,
		selector:          selector,
		circuitBreakerMgr: cbMgr,
	}
}

// ExecuteWithRetry executes a function with retry logic
func (rh *DefaultRetryHandler) ExecuteWithRetry(ctx context.Context, fn func(config *database.APIConfig) error) error {
	var lastErr error
	var lastConfig *database.APIConfig

	for attempt := 0; attempt <= rh.maxRetries; attempt++ {
		// Select a config node
		config, err := rh.selector.SelectConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to select config: %w", err)
		}

		lastConfig = config

		// Get circuit breaker for this config
		var cb CircuitBreaker
		if rh.circuitBreakerMgr != nil {
			cb = rh.circuitBreakerMgr.GetCircuitBreaker(config.ID)
		}

		// Execute with circuit breaker protection
		if cb != nil {
			err = cb.Call(ctx, func() error {
				return fn(config)
			})
		} else {
			err = fn(config)
		}

		// Success
		if err == nil {
			if attempt > 0 {
				log.Printf("Request succeeded after %d retries using config %s", attempt, config.ID)
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !rh.IsRetryableError(err) {
			log.Printf("Non-retryable error encountered: %v", err)
			return err
		}

		// Check if we've exhausted retries
		if attempt >= rh.maxRetries {
			log.Printf("Max retries (%d) exceeded, last error: %v", rh.maxRetries, err)
			break
		}

		// Calculate backoff delay
		delay := rh.CalculateBackoff(attempt)
		log.Printf("Retry attempt %d/%d after %v (error: %v)", attempt+1, rh.maxRetries, delay, err)

		// Wait before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next retry
		}
	}

	// All retries exhausted
	return fmt.Errorf("request failed after %d retries (config: %s): %w", rh.maxRetries, lastConfig.ID, lastErr)
}

// IsRetryableError determines if an error is retryable
func (rh *DefaultRetryHandler) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Network errors are retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for specific error messages
	errMsg := strings.ToLower(err.Error())

	// Connection errors are retryable
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "connection timeout") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "network is unreachable") ||
		strings.Contains(errMsg, "broken pipe") ||
		strings.Contains(errMsg, "i/o timeout") {
		return true
	}

	// Circuit breaker open is retryable (will try different node)
	if strings.Contains(errMsg, "circuit breaker is open") {
		return true
	}

	// Check for HTTP status codes
	if strings.Contains(errMsg, "status code") {
		// 5xx errors are retryable
		if strings.Contains(errMsg, "500") ||
			strings.Contains(errMsg, "502") ||
			strings.Contains(errMsg, "503") ||
			strings.Contains(errMsg, "504") {
			return true
		}

		// 429 (rate limit) is retryable
		if strings.Contains(errMsg, "429") {
			return true
		}

		// 4xx errors (except 429) are not retryable
		if strings.Contains(errMsg, "400") ||
			strings.Contains(errMsg, "401") ||
			strings.Contains(errMsg, "403") ||
			strings.Contains(errMsg, "404") {
			return false
		}
	}

	// Default to not retryable for unknown errors
	return false
}

// CalculateBackoff calculates the backoff delay for a retry attempt
func (rh *DefaultRetryHandler) CalculateBackoff(retryCount int) time.Duration {
	// Exponential backoff: initialDelay * 2^retryCount
	delay := rh.initialDelay * time.Duration(1<<uint(retryCount))

	// Cap at max delay
	if delay > rh.maxDelay {
		delay = rh.maxDelay
	}

	return delay
}

// HTTPStatusCodeError represents an HTTP error with status code
type HTTPStatusCodeError struct {
	StatusCode int
	Message    string
}

func (e *HTTPStatusCodeError) Error() string {
	return fmt.Sprintf("HTTP status code %d: %s", e.StatusCode, e.Message)
}

// NewHTTPStatusCodeError creates a new HTTP status code error
func NewHTTPStatusCodeError(statusCode int, message string) error {
	return &HTTPStatusCodeError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// IsRetryableHTTPStatus checks if an HTTP status code is retryable
func IsRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError,     // 500
		http.StatusBadGateway,               // 502
		http.StatusServiceUnavailable,       // 503
		http.StatusGatewayTimeout:           // 504
		return true
	default:
		return false
	}
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
	}
}
