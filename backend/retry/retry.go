package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

// ErrorCategory classifies the type of error for retry strategy selection.
type ErrorCategory int

const (
	CategoryRateLimit      ErrorCategory = iota
	CategoryServerError
	CategoryNetwork
	CategoryProtocol
	CategoryAuth
	CategoryPermanentQuota
	CategoryCancelled
	CategoryUnknown
)

// Strategy defines retry parameters for a specific error category.
type Strategy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Retryable  bool
}

var defaultStrategies = map[ErrorCategory]Strategy{
	CategoryRateLimit: {
		MaxRetries: 20, BaseDelay: 5 * time.Second, MaxDelay: 120 * time.Second, Retryable: true,
	},
	CategoryServerError: {
		MaxRetries: 5, BaseDelay: 2 * time.Second, MaxDelay: 30 * time.Second, Retryable: true,
	},
	CategoryNetwork: {
		MaxRetries: 10, BaseDelay: 1 * time.Second, MaxDelay: 15 * time.Second, Retryable: true,
	},
	CategoryProtocol:       {MaxRetries: 0, Retryable: false},
	CategoryAuth:           {MaxRetries: 0, Retryable: false},
	CategoryPermanentQuota: {MaxRetries: 0, Retryable: false},
	CategoryCancelled:      {MaxRetries: 0, Retryable: false},
	CategoryUnknown:        {MaxRetries: 0, Retryable: false},
}

// ClassifyError determines the error category from an error.
func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return CategoryUnknown
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return CategoryCancelled
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return CategoryNetwork
	}

	errStr := strings.ToLower(err.Error())

	if isPermanentQuotaError(errStr) {
		return CategoryPermanentQuota
	}

	if strings.Contains(errStr, "status 429") || strings.Contains(errStr, "rate limit") {
		return CategoryRateLimit
	}

	if isServerErrorStatus(errStr) {
		return CategoryServerError
	}

	if isNetworkError(errStr) {
		return CategoryNetwork
	}

	if isAuthError(errStr) {
		return CategoryAuth
	}

	if isProtocolError(errStr) {
		return CategoryProtocol
	}

	if strings.Contains(errStr, "circuit breaker is open") {
		return CategoryNetwork
	}

	if strings.Contains(errStr, "empty choices") || strings.Contains(errStr, "decode response") {
		return CategoryServerError
	}

	return CategoryUnknown
}

// GetStrategy returns the retry strategy for an error category using defaults.
func GetStrategy(category ErrorCategory) Strategy {
	return getStrategy(defaultStrategies, category)
}

// getStrategy looks up strategy from the given map with fallback.
func getStrategy(strategies map[ErrorCategory]Strategy, category ErrorCategory) Strategy {
	if s, ok := strategies[category]; ok {
		return s
	}
	return Strategy{MaxRetries: 0, Retryable: false}
}

// IsRetryable returns true if the error category supports retries.
func IsRetryable(err error) bool {
	return GetStrategy(ClassifyError(err)).Retryable
}

// CalculateBackoff computes exponential backoff with jitter.
func CalculateBackoff(strategy Strategy, attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	delay := strategy.BaseDelay * time.Duration(1<<uint(attempt-1))
	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	jitter := time.Duration(rand.Int63n(int64(delay/2 + 1)))
	delay += jitter

	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	return delay
}

// CalculateBackoffWithRetryAfter uses Retry-After value if available.
func CalculateBackoffWithRetryAfter(strategy Strategy, attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	return CalculateBackoff(strategy, attempt)
}

// Result contains the outcome of a retry execution.
type Result struct {
	Attempts   int
	LastErr    error
	Category   ErrorCategory
	Succeeded  bool
	TotalDelay time.Duration
}

// Execute runs fn with retry logic using default strategies.
func Execute(ctx context.Context, fn func() error) *Result {
	return ExecuteWithStrategies(ctx, defaultStrategies, fn)
}

// ExecuteWithStrategies runs fn with retry logic using custom strategy overrides.
func ExecuteWithStrategies(ctx context.Context, strategies map[ErrorCategory]Strategy, fn func() error) *Result {
	result := &Result{}

	err := fn()
	result.Attempts = 1
	result.LastErr = err

	if err == nil {
		result.Succeeded = true
		return result
	}

	category := ClassifyError(err)
	result.Category = category
	strategy := getStrategy(strategies, category)

	if !strategy.Retryable {
		return result
	}

	for attempt := 1; attempt <= strategy.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			return result
		default:
		}

		delay := CalculateBackoff(strategy, attempt)
		result.TotalDelay += delay

		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			return result
		case <-time.After(delay):
		}

		err = fn()
		result.Attempts++
		result.LastErr = err

		if err == nil {
			result.Succeeded = true
			result.Category = category
			return result
		}

		newCategory := ClassifyError(err)
		if newCategory != category {
			newStrategy := getStrategy(strategies, newCategory)
			if !newStrategy.Retryable {
				result.Category = newCategory
				return result
			}
			category = newCategory
			strategy = newStrategy
			result.Category = category
		}
	}

	return result
}

var permanentQuotaSignals = []string{
	"daily_limit_exceeded",
	"daily usage limit exceeded",
	"usage_limit_exceeded",
	"insufficient_quota",
	"quota_exceeded",
	"billing_hard_limit_reached",
	"credit_balance_too_low",
}

func isPermanentQuotaError(errStr string) bool {
	for _, signal := range permanentQuotaSignals {
		if strings.Contains(errStr, signal) {
			return true
		}
	}
	return false
}

func isServerErrorStatus(errStr string) bool {
	codes := []string{"status 500", "status 502", "status 503", "status 504", "status 506", "status 507", "status 508"}
	for _, code := range codes {
		if strings.Contains(errStr, code) {
			return true
		}
	}
	return false
}

var networkSignals = []string{
	"connection refused", "connection reset", "connection timeout",
	"no such host", "network is unreachable", "broken pipe",
	"i/o timeout", "temporary failure", "dns error",
	"tls handshake timeout", "temporary error",
}

func isNetworkError(errStr string) bool {
	for _, sig := range networkSignals {
		if strings.Contains(errStr, sig) {
			return true
		}
	}
	return false
}

func isAuthError(errStr string) bool {
	return strings.Contains(errStr, "status 401") ||
		strings.Contains(errStr, "status 403") ||
		strings.Contains(errStr, "invalid_api_key") ||
		strings.Contains(errStr, "unauthorized")
}

func isProtocolError(errStr string) bool {
	return strings.Contains(errStr, "status 400") ||
		strings.Contains(errStr, "status 404") ||
		strings.Contains(errStr, "status 422")
}

// String returns human-readable error category name.
func (c ErrorCategory) String() string {
	switch c {
	case CategoryRateLimit:
		return "rate_limit"
	case CategoryServerError:
		return "server_error"
	case CategoryNetwork:
		return "network"
	case CategoryProtocol:
		return "protocol"
	case CategoryAuth:
		return "auth"
	case CategoryPermanentQuota:
		return "permanent_quota"
	case CategoryCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// FormatRetryError returns a formatted error message with retry context.
func FormatRetryError(result *Result) error {
	if result.LastErr == nil {
		return nil
	}
	if result.Attempts <= 1 {
		return result.LastErr
	}
	return fmt.Errorf("failed after %d attempts (category: %s, total delay: %v): %w",
		result.Attempts, result.Category, result.TotalDelay, result.LastErr)
}

// RecoveryState tracks the state machine for a single request recovery process.
type RecoveryState struct {
	categoryAttempts        map[ErrorCategory]int
	errorHistory           []ErrorRecord
	totalDelay             time.Duration
	backoffMultiplier      float64
	consecutiveSameCategory int
}

// ErrorRecord captures a single error event during recovery.
type ErrorRecord struct {
	Attempt   int
	Category  ErrorCategory
	Error     error
	Timestamp time.Time
	Delay     time.Duration
}

// Engine provides advanced error recovery with stateful retry management.
type Engine struct {
	strategies map[ErrorCategory]Strategy
}

// NewEngine creates a recovery engine with default strategies.
func NewEngine() *Engine {
	return &Engine{strategies: defaultStrategies}
}

// NewEngineWithStrategies creates a recovery engine with custom strategy overrides.
func NewEngineWithStrategies(strategies map[ErrorCategory]Strategy) *Engine {
	merged := make(map[ErrorCategory]Strategy)
	for k, v := range defaultStrategies {
		merged[k] = v
	}
	for k, v := range strategies {
		merged[k] = v
	}
	return &Engine{strategies: merged}
}

// newRecoveryState creates a fresh recovery state.
func newRecoveryState() *RecoveryState {
	return &RecoveryState{
		categoryAttempts:        make(map[ErrorCategory]int),
		errorHistory:            nil,
		totalDelay:              0,
		backoffMultiplier:       1.0,
		consecutiveSameCategory: 0,
	}
}

// Execute runs fn with full state-machine retry logic.
func (e *Engine) Execute(ctx context.Context, fn func() error) *Result {
	state := newRecoveryState()
	result := &Result{}

	err := fn()
	result.Attempts = 1
	result.LastErr = err

	if err == nil {
		result.Succeeded = true
		return result
	}

	category := ClassifyError(err)
	result.Category = category
	state.errorHistory = append(state.errorHistory, ErrorRecord{
		Attempt: 1, Category: category, Error: err, Timestamp: time.Now(),
	})

	strategy := getStrategy(e.strategies, category)
	if !strategy.Retryable {
		return result
	}

	totalAttempt := 1
	for {
		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			result.TotalDelay = state.totalDelay
			return result
		default:
		}

		state.categoryAttempts[category]++
		if state.categoryAttempts[category] > strategy.MaxRetries {
			break
		}

		state.consecutiveSameCategory++
		state.backoffMultiplier = 1.0 + float64(state.consecutiveSameCategory-1)*0.2
		delay := CalculateBackoff(strategy, state.categoryAttempts[category])
		delay = time.Duration(float64(delay) * state.backoffMultiplier)
		if delay > strategy.MaxDelay*2 {
			delay = strategy.MaxDelay * 2
		}

		state.totalDelay += delay

		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			result.TotalDelay = state.totalDelay
			return result
		case <-time.After(delay):
		}

		err = fn()
		totalAttempt++
		result.Attempts = totalAttempt
		result.LastErr = err

		if err == nil {
			result.Succeeded = true
			result.Category = category
			result.TotalDelay = state.totalDelay
			return result
		}

		newCategory := ClassifyError(err)
		state.errorHistory = append(state.errorHistory, ErrorRecord{
			Attempt: totalAttempt, Category: newCategory, Error: err,
			Timestamp: time.Now(), Delay: delay,
		})

		if newCategory != category {
			state.consecutiveSameCategory = 0
			state.backoffMultiplier = 1.0
			category = newCategory
			strategy = getStrategy(e.strategies, category)
			result.Category = category
			if !strategy.Retryable {
				break
			}
		}
	}

	result.TotalDelay = state.totalDelay
	return result
}

// ErrorSummary returns a human-readable summary of all errors during recovery.
func (r *Result) ErrorSummary() string {
	if r.LastErr == nil {
		return ""
	}
	if r.Attempts <= 1 {
		return r.LastErr.Error()
	}
	return fmt.Sprintf("failed after %d attempts (category: %s, total delay: %v): %v",
		r.Attempts, r.Category, r.TotalDelay, r.LastErr)
}
