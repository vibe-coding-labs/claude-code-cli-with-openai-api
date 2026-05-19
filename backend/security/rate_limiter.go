package security

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// RateLimiter defines the interface for rate limiting operations
type RateLimiter interface {
	CheckLimit(ctx context.Context, key string, limit database.RateLimit) (allowed bool, retryAfter time.Duration, err error)
	GetLimits(ctx context.Context, tenantID string) ([]database.RateLimit, error)
	SetLimit(ctx context.Context, limit database.RateLimit) error
	DeleteLimit(ctx context.Context, limitID string) error
}

// rateLimiter implements the RateLimiter interface
type rateLimiter struct {
	db      *sql.DB
	cache   *sync.Map // key -> *rateLimitCounter
	mu      sync.RWMutex
}

// rateLimitCounter represents a rate limit counter for fixed window
type rateLimitCounter struct {
	count      int
	windowStart time.Time
	mu         sync.Mutex
}

// slidingWindowCounter represents a rate limit counter for sliding window
type slidingWindowCounter struct {
	requests []time.Time
	mu       sync.Mutex
}

// tokenBucketCounter represents a rate limit counter for token bucket
type tokenBucketCounter struct {
	tokens       float64
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter creates a new RateLimiter instance
func NewRateLimiter(db *sql.DB) RateLimiter {
	rl := &rateLimiter{
		db:    db,
		cache: &sync.Map{},
	}

	// Start cleanup goroutine to remove expired counters
	go rl.cleanupExpiredCounters()

	return rl
}

// CheckLimit checks if a request is within the rate limit
func (rl *rateLimiter) CheckLimit(ctx context.Context, key string, limit database.RateLimit) (allowed bool, retryAfter time.Duration, err error) {
	// Validate rate limit
	if err := limit.Validate(); err != nil {
		return false, 0, fmt.Errorf("invalid rate limit: %w", err)
	}

	// Use appropriate algorithm based on configuration
	switch limit.Algorithm {
	case "fixed_window":
		return rl.checkFixedWindow(key, limit)
	case "sliding_window":
		return rl.checkSlidingWindow(key, limit)
	case "token_bucket":
		return rl.checkTokenBucket(key, limit)
	default:
		return false, 0, fmt.Errorf("unsupported rate limit algorithm: %s", limit.Algorithm)
	}
}

// checkFixedWindow implements the fixed window rate limiting algorithm
func (rl *rateLimiter) checkFixedWindow(key string, limit database.RateLimit) (allowed bool, retryAfter time.Duration, err error) {
	now := time.Now()
	windowDuration := time.Duration(limit.Window) * time.Second

	// Get or create counter
	counterInterface, _ := rl.cache.LoadOrStore(key, &rateLimitCounter{
		count:      0,
		windowStart: now,
	})
	counter := counterInterface.(*rateLimitCounter)

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Check if we're in a new window
	if now.Sub(counter.windowStart) >= windowDuration {
		// Reset counter for new window
		counter.count = 0
		counter.windowStart = now
	}

	// Check if limit is exceeded
	if counter.count >= limit.Limit {
		// Calculate retry after
		windowEnd := counter.windowStart.Add(windowDuration)
		retryAfter = windowEnd.Sub(now)
		return false, retryAfter, nil
	}

	// Increment counter
	counter.count++

	return true, 0, nil
}

// GetLimits retrieves all rate limits for a tenant
func (rl *rateLimiter) GetLimits(ctx context.Context, tenantID string) ([]database.RateLimit, error) {
	query := `
		SELECT id, tenant_id, dimension, algorithm, [limit], window, created_at
		FROM rate_limits
		WHERE tenant_id = ? OR tenant_id IS NULL
		ORDER BY created_at DESC
	`
	rows, err := rl.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limits: %w", err)
	}
	defer rows.Close()

	var limits []database.RateLimit
	for rows.Next() {
		limit := database.RateLimit{}
		var tenantIDNull sql.NullString

		err := rows.Scan(
			&limit.ID, &tenantIDNull, &limit.Dimension, &limit.Algorithm,
			&limit.Limit, &limit.Window, &limit.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rate limit: %w", err)
		}

		if tenantIDNull.Valid {
			limit.TenantID = tenantIDNull.String
		}

		limits = append(limits, limit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rate limits: %w", err)
	}

	return limits, nil
}

// SetLimit creates or updates a rate limit
func (rl *rateLimiter) SetLimit(ctx context.Context, limit database.RateLimit) error {
	// Generate ID if not provided
	if limit.ID == "" {
		limit.ID = uuid.New().String()
	}

	// Set timestamp
	limit.CreatedAt = time.Now()

	// Validate rate limit
	if err := limit.Validate(); err != nil {
		return fmt.Errorf("invalid rate limit: %w", err)
	}

	// Check if limit already exists
	var count int
	checkQuery := `SELECT COUNT(*) FROM rate_limits WHERE id = ?`
	err := rl.db.QueryRowContext(ctx, checkQuery, limit.ID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check rate limit existence: %w", err)
	}

	if count > 0 {
		// Update existing limit
		query := `
			UPDATE rate_limits
			SET tenant_id = ?, dimension = ?, algorithm = ?, [limit] = ?, window = ?
			WHERE id = ?
		`
		_, err := rl.db.ExecContext(ctx, query,
			limit.TenantID, limit.Dimension, limit.Algorithm,
			limit.Limit, limit.Window, limit.ID)
		if err != nil {
			return fmt.Errorf("failed to update rate limit: %w", err)
		}
	} else {
		// Insert new limit
		query := `
			INSERT INTO rate_limits (id, tenant_id, dimension, algorithm, [limit], window, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		_, err := rl.db.ExecContext(ctx, query,
			limit.ID, limit.TenantID, limit.Dimension, limit.Algorithm,
			limit.Limit, limit.Window, limit.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to create rate limit: %w", err)
		}
	}

	return nil
}

// DeleteLimit deletes a rate limit
func (rl *rateLimiter) DeleteLimit(ctx context.Context, limitID string) error {
	query := `DELETE FROM rate_limits WHERE id = ?`
	result, err := rl.db.ExecContext(ctx, query, limitID)
	if err != nil {
		return fmt.Errorf("failed to delete rate limit: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("rate limit not found: %s", limitID)
	}

	return nil
}

// cleanupExpiredCounters periodically removes expired counters from the cache
func (rl *rateLimiter) cleanupExpiredCounters() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		rl.cache.Range(func(key, value interface{}) bool {
			counter := value.(*rateLimitCounter)
			counter.mu.Lock()
			// Remove counters that haven't been used in the last 10 minutes
			if now.Sub(counter.windowStart) > 10*time.Minute {
				rl.cache.Delete(key)
			}
			counter.mu.Unlock()
			return true
		})
	}
}


// checkSlidingWindow implements the sliding window rate limiting algorithm
func (rl *rateLimiter) checkSlidingWindow(key string, limit database.RateLimit) (allowed bool, retryAfter time.Duration, err error) {
	now := time.Now()
	windowDuration := time.Duration(limit.Window) * time.Second

	// Get or create counter
	counterInterface, _ := rl.cache.LoadOrStore(key+"_sliding", &slidingWindowCounter{
		requests: []time.Time{},
	})
	counter := counterInterface.(*slidingWindowCounter)

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Remove requests outside the current window
	cutoff := now.Add(-windowDuration)
	validRequests := []time.Time{}
	for _, reqTime := range counter.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	counter.requests = validRequests

	// Check if limit is exceeded
	if len(counter.requests) >= limit.Limit {
		// Calculate retry after (time until oldest request expires)
		if len(counter.requests) > 0 {
			oldestRequest := counter.requests[0]
			retryAfter = oldestRequest.Add(windowDuration).Sub(now)
		}
		return false, retryAfter, nil
	}

	// Add current request
	counter.requests = append(counter.requests, now)

	return true, 0, nil
}

// checkTokenBucket implements the token bucket rate limiting algorithm
func (rl *rateLimiter) checkTokenBucket(key string, limit database.RateLimit) (allowed bool, retryAfter time.Duration, err error) {
	now := time.Now()
	windowDuration := time.Duration(limit.Window) * time.Second

	// Calculate refill rate (tokens per second)
	refillRate := float64(limit.Limit) / windowDuration.Seconds()

	// Get or create counter
	counterInterface, _ := rl.cache.LoadOrStore(key+"_bucket", &tokenBucketCounter{
		tokens:     float64(limit.Limit), // Start with full bucket
		lastRefill: now,
	})
	counter := counterInterface.(*tokenBucketCounter)

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Refill tokens based on time elapsed
	elapsed := now.Sub(counter.lastRefill).Seconds()
	tokensToAdd := elapsed * refillRate
	counter.tokens = min(counter.tokens+tokensToAdd, float64(limit.Limit))
	counter.lastRefill = now

	// Check if we have at least 1 token
	if counter.tokens < 1.0 {
		// Calculate retry after (time to get 1 token)
		timeToToken := (1.0 - counter.tokens) / refillRate
		retryAfter = time.Duration(timeToToken * float64(time.Second))
		return false, retryAfter, nil
	}

	// Consume 1 token
	counter.tokens -= 1.0

	return true, 0, nil
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
