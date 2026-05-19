package ratelimit

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limiter provides per-config adaptive rate limiting.
// It learns cooldown periods from 429 responses and enforces them
// so subsequent requests wait rather than hammer the upstream.
type Limiter struct {
	mu        sync.Mutex
	cooldowns map[string]*cooldownEntry
}

type cooldownEntry struct {
	until  time.Time
	reason string
}

// Global is the shared rate limiter instance.
var Global = NewLimiter()

func NewLimiter() *Limiter {
	return &Limiter{
		cooldowns: make(map[string]*cooldownEntry),
	}
}

// Wait blocks until the cooldown for this config has elapsed, or ctx is cancelled.
// Returns nil if we can proceed, or ctx.Err() if cancelled.
func (l *Limiter) Wait(ctx context.Context, configID string) error {
	l.mu.Lock()
	entry, exists := l.cooldowns[configID]
	if !exists || time.Now().After(entry.until) {
		delete(l.cooldowns, configID)
		l.mu.Unlock()
		return nil
	}
	waitDur := time.Until(entry.until)
	l.mu.Unlock()

	if waitDur <= 0 {
		return nil
	}

	t := time.NewTimer(waitDur)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Cooldown sets a cooldown period for a config, learned from a 429 response.
func (l *Limiter) Cooldown(configID string, dur time.Duration, reason string) {
	if dur <= 0 || configID == "" {
		return
	}
	// Cap at 2 minutes to prevent runaway waits
	if dur > 2*time.Minute {
		dur = 2 * time.Minute
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	until := time.Now().Add(dur)
	if existing, ok := l.cooldowns[configID]; ok && existing.until.After(until) {
		return
	}
	l.cooldowns[configID] = &cooldownEntry{until: until, reason: reason}
}

// GetCooldown returns the remaining cooldown duration, or 0 if none.
func (l *Limiter) GetCooldown(configID string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.cooldowns[configID]
	if !exists {
		return 0
	}
	rem := time.Until(entry.until)
	if rem <= 0 {
		delete(l.cooldowns, configID)
		return 0
	}
	return rem
}

// retryAfterPatterns matches "retry in Xs" or "Please retry in 54.037995075s." patterns.
var retryAfterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)retry\s+(?:after|in)\s+(\d+(?:\.\d+)?)\s*(?:s|sec|seconds?)`),
	regexp.MustCompile(`(?i)try\s+again\s+in\s+(\d+(?:\.\d+)?)\s*(?:s|sec|seconds?)`),
	regexp.MustCompile(`(?i)wait\s+(\d+(?:\.\d+)?)\s*(?:s|sec|seconds?)`),
}

// ParseRetryAfter extracts a duration from upstream 429 error messages.
// Gemini returns: "Please retry in 54.037995075s."
// OpenAI returns: "Please retry after 20 seconds."
func ParseRetryAfter(errBody string) time.Duration {
	for _, pat := range retryAfterPatterns {
		matches := pat.FindStringSubmatch(errBody)
		if len(matches) >= 2 {
			if secs, err := strconv.ParseFloat(matches[1], 64); err == nil && secs > 0 {
				dur := time.Duration(secs * float64(time.Second))
				// Add 2s buffer to account for network delay
				dur += 2 * time.Second
				return dur
			}
		}
	}
	return 0
}

// Clean removes expired cooldown entries.
func (l *Limiter) Clean() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for id, entry := range l.cooldowns {
		if now.After(entry.until) {
			delete(l.cooldowns, id)
		}
	}
}

// Status returns a human-readable status of all cooldowns.
func (l *Limiter) Status() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.cooldowns) == 0 {
		return "no active cooldowns"
	}

	s := ""
	for id, entry := range l.cooldowns {
		rem := time.Until(entry.until)
		if rem > 0 {
			s += fmt.Sprintf("  %s: cooldown %v (%s)\n", id, rem.Round(time.Second), entry.reason)
		}
	}
	if s == "" {
		return "no active cooldowns"
	}
	return s
}

// RateLimit429Config controls the 429-specific smart retry behavior.
type RateLimit429Config struct {
	MaxAttempts    int           // Max 429-specific retry attempts (default: 12)
	BaseDelay      time.Duration // Base delay when no Retry-After (default: 5s)
	MaxDelay       time.Duration // Max single delay cap (default: 120s)
	TotalWaitCap   time.Duration // Total wait budget across all 429 retries (default: 180s)
	JitterFraction float64       // Jitter fraction 0-1 (default: 0.15)
	GrowthFactor   float64       // Exponential growth per attempt (default: 1.5)
	// AdaptiveStrategy enables auto-detection of strict upstream rate limits.
	// When enabled, consecutive 429s with long Retry-After values will
	// automatically upgrade the strategy: longer delays, more patience.
	AdaptiveStrategy bool
}

// Default429Config returns sensible defaults for 429 retry.
// Designed for Gemini free tier (~20 RPM) where 429s are common but transient.
// 429 is worth more patience than other errors because it's almost always
// transient — the request will succeed once the rate window resets.
func Default429Config() RateLimit429Config {
	return RateLimit429Config{
		MaxAttempts:      12,
		BaseDelay:        5 * time.Second,
		MaxDelay:         120 * time.Second,
		TotalWaitCap:     180 * time.Second,
		JitterFraction:   0.15,
		GrowthFactor:     1.5,
		AdaptiveStrategy: true,
	}
}

// Strict429Config returns aggressive defaults for known strict rate limiters.
// Use this for providers with very low RPM (e.g., Gemini free tier 5 RPM).
func Strict429Config() RateLimit429Config {
	return RateLimit429Config{
		MaxAttempts:      20,
		BaseDelay:        10 * time.Second,
		MaxDelay:         180 * time.Second,
		TotalWaitCap:     300 * time.Second,
		JitterFraction:   0.2,
		GrowthFactor:     1.3,
		AdaptiveStrategy: true,
	}
}

// Classify429Severity analyzes a 429 error body to determine how strict
// the upstream rate limit is, and whether we should adopt a more patient strategy.
type Classify429Severity int

const (
	// SeverityTransient indicates a brief, transient rate limit.
	// Typical for OpenAI, Anthropic paid tiers. Retry-After is short (<10s).
	SeverityTransient Classify429Severity = iota
	// SeverityModerate indicates a moderate rate limit.
	// Typical for Gemini free tier (20 RPM). Retry-After is 20-60s.
	SeverityModerate
	// SeverityStrict indicates a very strict rate limit.
	// Typical for providers with 5 RPM or daily quotas. Retry-After >60s.
	SeverityStrict
	// SeverityQuotaExhausted indicates a permanent quota/billing limit.
	// Retrying is pointless — the account has hit a hard cap.
	SeverityQuotaExhausted
)

// Classify429 analyzes a 429 error body and determines the severity.
func Classify429(errBody string) Classify429Severity {
	lower := strings.ToLower(errBody)

	// Permanent quota/billing exhaustion — no point retrying
	quotaSignals := []string{
		"daily_limit_exceeded", "daily usage limit exceeded",
		"insufficient_quota", "quota_exceeded",
		"billing_hard_limit_reached", "credit_balance_too_low",
		"resource_exhausted", "monthly_limit",
	}
	for _, sig := range quotaSignals {
		if strings.Contains(lower, sig) {
			return SeverityQuotaExhausted
		}
	}

	// Parse Retry-After to gauge severity
	retryAfter := ParseRetryAfter(errBody)
	switch {
	case retryAfter > 60*time.Second:
		return SeverityStrict
	case retryAfter > 10*time.Second:
		return SeverityModerate
	default:
		return SeverityTransient
	}
}

// RateLimit429Result captures the outcome of a 429 smart retry cycle.
type RateLimit429Result struct {
	Attempts           int                // Number of 429 retries performed
	TotalWaited        time.Duration      // Total time spent waiting
	Succeeded          bool               // True if the request eventually succeeded
	Aborted            bool               // True if we gave up (total wait cap or ctx cancelled)
	Reason             string             // Why we aborted, if applicable
	UpstreamRetryAfter time.Duration      // Parsed Retry-After from upstream 429 response
	Severity           Classify429Severity // Detected severity of the 429
}

// WaitWith429Backoff performs smart 429-specific backoff waiting.
// Unlike ExecuteWith429Retry which wraps a function call, this method
// only handles the wait logic — the caller is responsible for making
// the actual request and detecting 429 errors.
//
// It parses Retry-After from the error body, classifies the 429 severity,
// and applies adaptive exponential growth with jitter. If the upstream
// appears to have strict rate limits (long Retry-After), it automatically
// adopts a more patient strategy.
//
// Returns (result, nil) on normal completion, or (nil, ctx.Err()) if cancelled.
func (l *Limiter) WaitWith429Backoff(
	ctx context.Context,
	configID string,
	errBody string,
	cfg RateLimit429Config,
) (*RateLimit429Result, error) {
	upstreamWait := ParseRetryAfter(errBody)
	severity := Classify429(errBody)

	// If quota is exhausted, no point waiting
	if severity == SeverityQuotaExhausted {
		return &RateLimit429Result{
			Aborted:            true,
			Reason:             "quota/billing limit exhausted — retrying is pointless",
			Attempts:           0,
			TotalWaited:        0,
			UpstreamRetryAfter: upstreamWait,
			Severity:           severity,
		}, nil
	}

	// Adaptive strategy: upgrade config for strict rate limiters
	effectiveCfg := cfg
	if cfg.AdaptiveStrategy {
		effectiveCfg = l.adapt429Config(cfg, severity, errBody)
	}

	delay := l.calculate429Delay(errBody, 0, effectiveCfg)

	l.Cooldown(configID, delay, fmt.Sprintf("429 backoff (severity=%d)", severity))

	t := time.NewTimer(delay)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return &RateLimit429Result{
			Aborted:            true,
			Reason:             "context cancelled during 429 backoff",
			Attempts:           1,
			TotalWaited:        0,
			UpstreamRetryAfter: upstreamWait,
			Severity:           severity,
		}, ctx.Err()
	case <-t.C:
		return &RateLimit429Result{
			Succeeded:          false,
			Attempts:           1,
			TotalWaited:        delay,
			UpstreamRetryAfter: upstreamWait,
			Severity:           severity,
		}, nil
	}
}

// adapt429Config adjusts the retry config based on detected 429 severity.
// This is the "auto-identify strict upstream" logic.
func (l *Limiter) adapt429Config(cfg RateLimit429Config, severity Classify429Severity, errBody string) RateLimit429Config {
	switch severity {
	case SeverityStrict:
		// Strict rate limiter detected (Retry-After >60s)
		// Use more patient strategy: more attempts, longer waits, gentler growth
		cfg.MaxAttempts = max(cfg.MaxAttempts, 20)
		cfg.MaxDelay = max(cfg.MaxDelay, 180*time.Second)
		cfg.TotalWaitCap = max(cfg.TotalWaitCap, 300*time.Second)
		cfg.GrowthFactor = 1.3 // Gentler growth to avoid overshooting
	case SeverityModerate:
		// Moderate rate limiter (Retry-After 10-60s)
		cfg.MaxAttempts = max(cfg.MaxAttempts, 15)
		cfg.MaxDelay = max(cfg.MaxDelay, 120*time.Second)
		cfg.TotalWaitCap = max(cfg.TotalWaitCap, 240*time.Second)
	case SeverityTransient:
		// Transient — keep defaults
	case SeverityQuotaExhausted:
		// No retry — handled before this point
	}
	return cfg
}

// ExecuteWith429Retry runs fn with smart 429-specific retry logic.
// When fn returns a 429 error (indicated by is429 callback), this function:
//  1. Parses Retry-After from the error body to get the upstream's suggested wait
//  2. Uses that as the base delay, with exponential growth on repeated 429s
//  3. Waits with jitter to avoid thundering herd
//  4. Updates the per-config cooldown so concurrent requests also benefit
//  5. Retries fn until it succeeds, returns non-429 error, or exhausts budget
//
// is429 should return (true, errorBody) if the result is a 429, (false, "") otherwise.
// fn should return nil on success, or an error on failure.
func (l *Limiter) ExecuteWith429Retry(
	ctx context.Context,
	configID string,
	cfg RateLimit429Config,
	fn func() error,
	is429 func(err error) (bool, string),
) *RateLimit429Result {
	result := &RateLimit429Result{}

	for attempt := 0; attempt <= cfg.MaxAttempts; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			result.Aborted = true
			result.Reason = "context cancelled"
			return result
		default:
		}

		// Execute the request
		err := fn()
		if err == nil {
			result.Succeeded = true
			result.Attempts = attempt
			return result
		}

		// Check if this is a 429 error
		isRateLimit, errBody := is429(err)
		if !isRateLimit {
			// Not a 429 — return the error, let outer retry handle it
			result.Attempts = attempt
			result.Reason = fmt.Sprintf("non-429 error: %v", err)
			return result
		}

		// It's a 429 — calculate smart backoff delay
		delay := l.calculate429Delay(errBody, attempt, cfg)

		// Check if total wait would exceed budget
		if result.TotalWaited+delay > cfg.TotalWaitCap {
			result.Aborted = true
			result.Reason = fmt.Sprintf("total wait %v would exceed cap %v",
				(result.TotalWaited + delay).Round(time.Second), cfg.TotalWaitCap)
			// Set cooldown so other requests also back off
			l.Cooldown(configID, cfg.TotalWaitCap-result.TotalWaited, "429 budget exhausted")
			result.Attempts = attempt + 1
			return result
		}

		// Update per-config cooldown so concurrent requests also wait
		l.Cooldown(configID, delay, fmt.Sprintf("429 smart backoff (attempt %d)", attempt+1))

		result.Attempts = attempt + 1
		result.TotalWaited += delay

		// Wait for the delay, respecting context cancellation
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			result.Aborted = true
			result.Reason = "context cancelled during 429 backoff"
			return result
		case <-t.C:
			// Continue to next retry attempt
		}
	}

	// Exhausted all attempts
	result.Aborted = true
	result.Reason = fmt.Sprintf("exhausted %d 429 retry attempts", cfg.MaxAttempts)
	return result
}

// calculate429Delay computes the wait delay for a 429 retry attempt.
// Uses upstream Retry-After as base, with configurable exponential growth and jitter.
func (l *Limiter) calculate429Delay(errBody string, attempt int, cfg RateLimit429Config) time.Duration {
	// Parse Retry-After from the error body
	upstreamWait := ParseRetryAfter(errBody)

	// Use upstream's suggestion as base, or fall back to BaseDelay
	delay := upstreamWait
	if delay <= 0 {
		delay = cfg.BaseDelay
	}

	// Apply exponential growth per attempt.
	// GrowthFactor defaults to 1.5 (gentler than 2x because 429s are transient).
	// Adaptive strategy may lower this to 1.3 for strict rate limiters.
	growth := cfg.GrowthFactor
	if growth <= 1.0 {
		growth = 1.5
	}
	if attempt > 0 {
		growthFactor := 1.0
		for i := 0; i < attempt; i++ {
			growthFactor *= growth
		}
		delay = time.Duration(float64(delay) * growthFactor)
	}

	// Cap at MaxDelay
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// Add jitter to avoid thundering herd
	if cfg.JitterFraction > 0 {
		jitterRange := time.Duration(float64(delay) * cfg.JitterFraction)
		if jitterRange > 0 {
			jitter := time.Duration(rand.Int63n(int64(jitterRange)))
			delay += jitter
		}
	}

	// Final cap
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	return delay
}

// ShouldReturn429Overload checks whether the current cooldown state suggests
// we should return an overloaded_error to the client instead of waiting.
// This prevents Claude Code from hanging on long waits.
func (l *Limiter) ShouldReturn429Overload(configID string) (shouldReturn bool, remaining time.Duration) {
	remaining = l.GetCooldown(configID)
	if remaining > 60*time.Second {
		return true, remaining
	}
	return false, remaining
}
