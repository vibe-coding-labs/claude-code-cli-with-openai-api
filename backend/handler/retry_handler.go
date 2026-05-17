package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/retry"

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
		// attempt starts from 0, so first retry is attempt=1, use CalculateBackoff(0)=initialDelay
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

// IsRetryableError delegates to the retry package for consistent error classification.
func (rh *DefaultRetryHandler) IsRetryableError(err error) bool {
	return retry.IsRetryable(err)
}

// CalculateBackoff calculates the backoff delay for a retry attempt using exponential backoff
// Formula: min(initialDelay * 2^retryCount, maxDelay)
// Example with initialDelay=1s: 1s, 2s, 4s, 8s, 16s, 32s, 60s, 60s, ... (capped at 60s)
func (rh *DefaultRetryHandler) CalculateBackoff(retryCount int) time.Duration {
	if retryCount < 0 {
		return 0
	}

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
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// RetryConfig holds retry configuration
const (
	DefaultMaxRetries = 10                     // 默认重试 10 次（覆盖网络抖动+上游超时）
	MinRetryCount     = 3                      // 最少重试 3 次
	MaxRetryCount     = 50                     // 最多重试 50 次
	BaseBackoffDelay  = 100 * time.Millisecond // 基础退避 100ms
	MaxBackoffDelay   = 5 * time.Second        // 最大退避 5 秒
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// DefaultRetryConfig returns default retry configuration with exponential backoff
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   DefaultMaxRetries,
		InitialDelay: BaseBackoffDelay,
		MaxDelay:     MaxBackoffDelay,
	}
}
