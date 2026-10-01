package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestRateLimiter_CheckLimit_FixedWindow(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	limit := database.RateLimit{ID: "rl-1", Dimension: "api_key", Algorithm: "fixed_window", Limit: 2, Window: 60}

	allowed, _, err := rl.CheckLimit(ctx, "key-fw", limit)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, _, err = rl.CheckLimit(ctx, "key-fw", limit)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Third request within the window exceeds the limit of 2
	allowed, retryAfter, err := rl.CheckLimit(ctx, "key-fw", limit)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Greater(t, retryAfter, time.Duration(0))
}

func TestRateLimiter_CheckLimit_SlidingWindow(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	limit := database.RateLimit{ID: "rl-2", Dimension: "api_key", Algorithm: "sliding_window", Limit: 2, Window: 60}

	allowed, _, err := rl.CheckLimit(ctx, "key-sw", limit)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, _, err = rl.CheckLimit(ctx, "key-sw", limit)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, retryAfter, err := rl.CheckLimit(ctx, "key-sw", limit)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Greater(t, retryAfter, time.Duration(0))
}

func TestRateLimiter_CheckLimit_TokenBucket(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	limit := database.RateLimit{ID: "rl-3", Dimension: "api_key", Algorithm: "token_bucket", Limit: 2, Window: 60}

	allowed, _, err := rl.CheckLimit(ctx, "key-tb", limit)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, _, err = rl.CheckLimit(ctx, "key-tb", limit)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Bucket exhausted (2 tokens consumed, negligible time elapsed to refill)
	allowed, retryAfter, err := rl.CheckLimit(ctx, "key-tb", limit)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Greater(t, retryAfter, time.Duration(0))
}

func TestRateLimiter_CheckLimit_InvalidLimit(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	_, _, err := rl.CheckLimit(ctx, "key-x", database.RateLimit{ID: "", Dimension: "api_key", Algorithm: "fixed_window", Limit: 1, Window: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid rate limit")
}

func TestRateLimiter_CheckLimit_UnsupportedAlgorithm(t *testing.T) {
	// CheckLimit's switch has a "default: unsupported algorithm" branch, but
	// RateLimit.Validate() (called first) already rejects any Algorithm value
	// outside {fixed_window, sliding_window, token_bucket}, so that branch is
	// unreachable through the public API. This documents that invariant.
	limit := database.RateLimit{ID: "rl-4", Dimension: "api_key", Algorithm: "quantum_window", Limit: 1, Window: 1}
	err := limit.Validate()
	require.Error(t, err, "Validate should reject unknown algorithms before CheckLimit's switch is ever reached")
}

func TestRateLimiter_GetLimits(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	require.NoError(t, rl.SetLimit(ctx, database.RateLimit{ID: "rl-t1", TenantID: "tenant-1", Dimension: "api_key", Algorithm: "fixed_window", Limit: 10, Window: 60}))
	require.NoError(t, rl.SetLimit(ctx, database.RateLimit{ID: "rl-global", Dimension: "ip", Algorithm: "fixed_window", Limit: 100, Window: 60}))
	require.NoError(t, rl.SetLimit(ctx, database.RateLimit{ID: "rl-t2", TenantID: "tenant-2", Dimension: "api_key", Algorithm: "fixed_window", Limit: 5, Window: 60}))

	limits, err := rl.GetLimits(ctx, "tenant-1")
	require.NoError(t, err)
	// tenant-1 scoped limit + the global limit, not tenant-2's
	assert.Len(t, limits, 2)
}

func TestRateLimiter_SetLimit_CreateAndUpdate(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	l := database.RateLimit{ID: "rl-upd", TenantID: "tenant-1", Dimension: "api_key", Algorithm: "fixed_window", Limit: 10, Window: 60}
	require.NoError(t, rl.SetLimit(ctx, l))

	limits, err := rl.GetLimits(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, limits, 1)
	assert.Equal(t, 10, limits[0].Limit)

	// Update same ID with a different limit value
	l.Limit = 20
	require.NoError(t, rl.SetLimit(ctx, l))

	limits, err = rl.GetLimits(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, limits, 1, "update should not create a duplicate row")
	assert.Equal(t, 20, limits[0].Limit)
}

func TestRateLimiter_SetLimit_ValidationError(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)

	err := rl.SetLimit(context.Background(), database.RateLimit{Dimension: "bogus-dimension", Algorithm: "fixed_window", Limit: 1, Window: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid rate limit")
}

func TestRateLimiter_DeleteLimit(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)
	ctx := context.Background()

	require.NoError(t, rl.SetLimit(ctx, database.RateLimit{ID: "rl-del", TenantID: "tenant-1", Dimension: "api_key", Algorithm: "fixed_window", Limit: 10, Window: 60}))

	require.NoError(t, rl.DeleteLimit(ctx, "rl-del"))

	limits, err := rl.GetLimits(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Empty(t, limits)
}

func TestRateLimiter_DeleteLimit_NotFound(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rl := NewRateLimiter(db)

	err := rl.DeleteLimit(context.Background(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit not found")
}

// TestRateLimiter_CleanupExpiredCounters_MixedAlgorithmsNoPanic is a
// regression test for a real bug: cleanupExpiredCountersOnce used to
// unconditionally type-assert every cached value to *rateLimitCounter, but
// the same cache also stores *slidingWindowCounter and *tokenBucketCounter
// entries (under "_sliding" / "_bucket" key suffixes) whenever more than one
// algorithm is used concurrently. That assertion panicked - and because
// cleanup runs in a detached background goroutine with no recover, the panic
// took down the entire process a maximum of one minute after any deployment
// mixing rate-limit algorithms went live. This exercises all three counter
// types together and confirms a cleanup sweep no longer panics and still
// expires old entries correctly.
func TestRateLimiter_CleanupExpiredCounters_MixedAlgorithmsNoPanic(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rlIface := NewRateLimiter(db)
	rl := rlIface.(*rateLimiter)
	ctx := context.Background()

	fixed := database.RateLimit{ID: "rl-mix-fixed", Dimension: "api_key", Algorithm: "fixed_window", Limit: 5, Window: 60}
	sliding := database.RateLimit{ID: "rl-mix-sliding", Dimension: "api_key", Algorithm: "sliding_window", Limit: 5, Window: 60}
	bucket := database.RateLimit{ID: "rl-mix-bucket", Dimension: "api_key", Algorithm: "token_bucket", Limit: 5, Window: 60}

	_, _, err := rl.CheckLimit(ctx, "mix-key", fixed)
	require.NoError(t, err)
	_, _, err = rl.CheckLimit(ctx, "mix-key", sliding)
	require.NoError(t, err)
	_, _, err = rl.CheckLimit(ctx, "mix-key", bucket)
	require.NoError(t, err)

	// All three counter types now coexist in rl.cache under different keys.
	assert.NotPanics(t, func() {
		rl.cleanupExpiredCountersOnce()
	})

	// Entries are fresh, so nothing should have been evicted yet.
	_, stillThere := rl.cache.Load("mix-key")
	assert.True(t, stillThere)
	_, stillThere = rl.cache.Load("mix-key_sliding")
	assert.True(t, stillThere)
	_, stillThere = rl.cache.Load("mix-key_bucket")
	assert.True(t, stillThere)
}

func TestRateLimiter_CleanupExpiredCounters_EvictsStaleEntries(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	rlIface := NewRateLimiter(db)
	rl := rlIface.(*rateLimiter)

	staleTime := time.Now().Add(-11 * time.Minute)
	rl.cache.Store("stale-fixed", &rateLimitCounter{count: 1, windowStart: staleTime})
	rl.cache.Store("stale-fixed_sliding", &slidingWindowCounter{requests: []time.Time{staleTime}})
	rl.cache.Store("stale-fixed_bucket", &tokenBucketCounter{tokens: 1, lastRefill: staleTime})
	rl.cache.Store("stale-empty_sliding", &slidingWindowCounter{requests: []time.Time{}})

	rl.cleanupExpiredCountersOnce()

	_, ok := rl.cache.Load("stale-fixed")
	assert.False(t, ok, "fixed window counter idle for 11 minutes should be evicted")
	_, ok = rl.cache.Load("stale-fixed_sliding")
	assert.False(t, ok, "sliding window counter idle for 11 minutes should be evicted")
	_, ok = rl.cache.Load("stale-fixed_bucket")
	assert.False(t, ok, "token bucket counter idle for 11 minutes should be evicted")
	_, ok = rl.cache.Load("stale-empty_sliding")
	assert.False(t, ok, "sliding window counter with no recorded requests should be evicted")
}
