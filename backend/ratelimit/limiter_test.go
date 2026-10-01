package ratelimit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		errBody  string
		wantMin  time.Duration
		wantZero bool
	}{
		{
			name:    "Gemini format",
			errBody: "Please retry in 54.037995075s.",
			wantMin: 54 * time.Second,
		},
		{
			name:    "OpenAI format",
			errBody: "Please retry after 20 seconds.",
			wantMin: 20 * time.Second,
		},
		{
			name:    "try again format",
			errBody: "Try again in 30 seconds",
			wantMin: 30 * time.Second,
		},
		{
			name:    "wait format",
			errBody: "Please wait 10s before retrying",
			wantMin: 10 * time.Second,
		},
		{
			name:     "no retry-after",
			errBody:  "rate limit exceeded",
			wantZero: true,
		},
		{
			name:     "empty body",
			errBody:  "",
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRetryAfter(tt.errBody)
			if tt.wantZero {
				if got != 0 {
					t.Errorf("ParseRetryAfter(%q) = %v, want 0", tt.errBody, got)
				}
			} else {
				if got < tt.wantMin {
					t.Errorf("ParseRetryAfter(%q) = %v, want >= %v", tt.errBody, got, tt.wantMin)
				}
			}
		})
	}
}

func TestClassify429(t *testing.T) {
	tests := []struct {
		name     string
		errBody  string
		want     Classify429Severity
	}{
		{
			name:    "transient - short retry-after",
			errBody: "Rate limit exceeded. Please retry after 5 seconds.",
			want:    SeverityTransient,
		},
		{
			name:    "moderate - medium retry-after",
			errBody: "Please retry in 30s.",
			want:    SeverityModerate,
		},
		{
			name:    "strict - long retry-after",
			errBody: "Please retry in 90.037995075s.",
			want:    SeverityStrict,
		},
		{
			name:    "quota exhausted - insufficient_quota",
			errBody: `{"error": {"message": "Insufficient quota", "type": "insufficient_quota"}}`,
			want:    SeverityQuotaExhausted,
		},
		{
			name:    "quota exhausted - daily_limit",
			errBody: "You have exceeded your daily_limit_exceeded",
			want:    SeverityQuotaExhausted,
		},
		{
			name:    "transient - no retry-after",
			errBody: "rate limit exceeded",
			want:    SeverityTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify429(tt.errBody)
			if got != tt.want {
				t.Errorf("Classify429(%q) = %d, want %d", tt.errBody, got, tt.want)
			}
		})
	}
}

func TestWaitWith429Backoff_Transient(t *testing.T) {
	limiter := NewLimiter()
	ctx := context.Background()
	cfg := Default429Config()

	result, err := limiter.WaitWith429Backoff(ctx, "test-config", "Please retry in 5s.", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Aborted {
		t.Errorf("expected not aborted, got aborted: %s", result.Reason)
	}
	if result.TotalWaited < 5*time.Second {
		t.Errorf("expected wait >= 5s, got %v", result.TotalWaited)
	}
	if result.Severity != SeverityTransient {
		t.Errorf("expected severity Transient, got %d", result.Severity)
	}
}

func TestWaitWith429Backoff_QuotaExhausted(t *testing.T) {
	limiter := NewLimiter()
	ctx := context.Background()
	cfg := Default429Config()

	result, err := limiter.WaitWith429Backoff(ctx, "test-config", "insufficient_quota: you exceeded your limit", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aborted {
		t.Errorf("expected aborted for quota exhaustion, got not aborted")
	}
	if result.Severity != SeverityQuotaExhausted {
		t.Errorf("expected severity QuotaExhausted, got %d", result.Severity)
	}
	if result.TotalWaited != 0 {
		t.Errorf("expected 0 wait for quota exhaustion, got %v", result.TotalWaited)
	}
}

func TestWaitWith429Backoff_ContextCancellation(t *testing.T) {
	limiter := NewLimiter()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := Default429Config()

	// Cancel immediately
	cancel()

	result, err := limiter.WaitWith429Backoff(ctx, "test-config", "Please retry in 30s.", cfg)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !result.Aborted {
		t.Errorf("expected aborted, got not aborted")
	}
}

func TestAdapt429Config_Strict(t *testing.T) {
	limiter := NewLimiter()
	cfg := Default429Config()

	adapted := limiter.adapt429Config(cfg, SeverityStrict, "Please retry in 90s.")

	if adapted.MaxAttempts < 20 {
		t.Errorf("expected MaxAttempts >= 20 for strict, got %d", adapted.MaxAttempts)
	}
	if adapted.MaxDelay < 180*time.Second {
		t.Errorf("expected MaxDelay >= 180s for strict, got %v", adapted.MaxDelay)
	}
	if adapted.GrowthFactor != 1.3 {
		t.Errorf("expected GrowthFactor 1.3 for strict, got %v", adapted.GrowthFactor)
	}
}

func TestAdapt429Config_Moderate(t *testing.T) {
	limiter := NewLimiter()
	cfg := Default429Config()

	adapted := limiter.adapt429Config(cfg, SeverityModerate, "Please retry in 30s.")

	if adapted.MaxAttempts < 15 {
		t.Errorf("expected MaxAttempts >= 15 for moderate, got %d", adapted.MaxAttempts)
	}
	if adapted.MaxDelay < 120*time.Second {
		t.Errorf("expected MaxDelay >= 120s for moderate, got %v", adapted.MaxDelay)
	}
}

func TestStrict429Config(t *testing.T) {
	cfg := Strict429Config()
	if cfg.MaxAttempts != 20 {
		t.Errorf("expected MaxAttempts=20, got %d", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != 10*time.Second {
		t.Errorf("expected BaseDelay=10s, got %v", cfg.BaseDelay)
	}
	if cfg.TotalWaitCap != 300*time.Second {
		t.Errorf("expected TotalWaitCap=300s, got %v", cfg.TotalWaitCap)
	}
}

func TestCalculate429Delay_CustomGrowthFactor(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		BaseDelay:    5 * time.Second,
		MaxDelay:     300 * time.Second,
		GrowthFactor: 2.0,
		JitterFraction: 0,
	}

	// attempt 0: use upstream or base directly (no growth)
	d0 := limiter.calculate429Delay("", 0, cfg)
	if d0 != 5*time.Second {
		t.Errorf("attempt 0: expected 5s, got %v", d0)
	}

	// attempt 1: 5s * 2.0 = 10s
	d1 := limiter.calculate429Delay("", 1, cfg)
	if d1 != 10*time.Second {
		t.Errorf("attempt 1: expected 10s, got %v", d1)
	}

	// attempt 2: 5s * 4.0 = 20s
	d2 := limiter.calculate429Delay("", 2, cfg)
	if d2 != 20*time.Second {
		t.Errorf("attempt 2: expected 20s, got %v", d2)
	}
}

func TestWait_ExpiredEntryIsCleared(t *testing.T) {
	limiter := NewLimiter()
	ctx := context.Background()

	// Manually plant an already-expired cooldown entry (bypasses Cooldown's
	// dur<=0 guard so we can exercise the "exists but expired" branch of Wait).
	limiter.mu.Lock()
	limiter.cooldowns["stale"] = &cooldownEntry{until: time.Now().Add(-1 * time.Second), reason: "old"}
	limiter.mu.Unlock()

	if err := limiter.Wait(ctx, "stale"); err != nil {
		t.Fatalf("Wait on expired entry should return nil, got %v", err)
	}

	limiter.mu.Lock()
	_, exists := limiter.cooldowns["stale"]
	limiter.mu.Unlock()
	if exists {
		t.Error("expired entry should have been deleted by Wait")
	}
}

func TestWait_ContextCancelledDuringWait(t *testing.T) {
	limiter := NewLimiter()
	limiter.Cooldown("slow", 500*time.Millisecond, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx, "slow")
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestCooldown_ZeroOrNegativeDurationIsNoop(t *testing.T) {
	limiter := NewLimiter()
	limiter.Cooldown("test", 0, "zero")
	if rem := limiter.GetCooldown("test"); rem != 0 {
		t.Errorf("zero duration should not set cooldown, got %v", rem)
	}

	limiter.Cooldown("test", -5*time.Second, "negative")
	if rem := limiter.GetCooldown("test"); rem != 0 {
		t.Errorf("negative duration should not set cooldown, got %v", rem)
	}
}

func TestCooldown_EmptyConfigIDIsNoop(t *testing.T) {
	limiter := NewLimiter()
	limiter.Cooldown("", 5*time.Second, "empty id")
	limiter.mu.Lock()
	n := len(limiter.cooldowns)
	limiter.mu.Unlock()
	if n != 0 {
		t.Errorf("empty configID should not create an entry, got %d entries", n)
	}
}

func TestCooldown_CappedAtTwoMinutes(t *testing.T) {
	limiter := NewLimiter()
	limiter.Cooldown("test", 10*time.Minute, "huge")
	rem := limiter.GetCooldown("test")
	if rem > 2*time.Minute {
		t.Errorf("cooldown should be capped at 2m, got %v", rem)
	}
	if rem < 110*time.Second {
		t.Errorf("cooldown should be close to the 2m cap, got %v", rem)
	}
}

func TestCooldown_DoesNotShortenLongerExistingCooldown(t *testing.T) {
	limiter := NewLimiter()
	limiter.Cooldown("test", 5*time.Second, "long")
	longRem := limiter.GetCooldown("test")

	// A shorter subsequent cooldown must not override the longer one.
	limiter.Cooldown("test", 1*time.Second, "short")
	shortRem := limiter.GetCooldown("test")

	if shortRem < longRem-100*time.Millisecond {
		t.Errorf("shorter cooldown should not have shortened existing one: long=%v short=%v", longRem, shortRem)
	}
}

func TestClean(t *testing.T) {
	limiter := NewLimiter()
	limiter.mu.Lock()
	limiter.cooldowns["expired"] = &cooldownEntry{until: time.Now().Add(-1 * time.Second), reason: "old"}
	limiter.cooldowns["active"] = &cooldownEntry{until: time.Now().Add(1 * time.Minute), reason: "fresh"}
	limiter.mu.Unlock()

	limiter.Clean()

	limiter.mu.Lock()
	_, expiredExists := limiter.cooldowns["expired"]
	_, activeExists := limiter.cooldowns["active"]
	limiter.mu.Unlock()

	if expiredExists {
		t.Error("expired entry should be removed by Clean")
	}
	if !activeExists {
		t.Error("active entry should survive Clean")
	}
}

func TestStatus_NoCooldowns(t *testing.T) {
	limiter := NewLimiter()
	if got := limiter.Status(); got != "no active cooldowns" {
		t.Errorf("expected 'no active cooldowns', got %q", got)
	}
}

func TestStatus_WithActiveCooldown(t *testing.T) {
	limiter := NewLimiter()
	limiter.Cooldown("my-config", 30*time.Second, "test reason")
	status := limiter.Status()
	if !strings.Contains(status, "my-config") || !strings.Contains(status, "test reason") {
		t.Errorf("status missing expected content: %q", status)
	}
}

func TestStatus_OnlyExpiredEntriesReportsNoCooldowns(t *testing.T) {
	limiter := NewLimiter()
	limiter.mu.Lock()
	limiter.cooldowns["expired"] = &cooldownEntry{until: time.Now().Add(-1 * time.Second), reason: "old"}
	limiter.mu.Unlock()

	if got := limiter.Status(); got != "no active cooldowns" {
		t.Errorf("expected 'no active cooldowns' when all entries are expired, got %q", got)
	}
}

func TestShouldReturn429Overload(t *testing.T) {
	limiter := NewLimiter()

	if should, rem := limiter.ShouldReturn429Overload("none"); should || rem != 0 {
		t.Errorf("no cooldown: expected (false, 0), got (%v, %v)", should, rem)
	}

	limiter.Cooldown("short", 5*time.Second, "short cooldown")
	if should, _ := limiter.ShouldReturn429Overload("short"); should {
		t.Error("short cooldown (<=60s) should not trigger overload return")
	}

	limiter.Cooldown("long", 90*time.Second, "long cooldown")
	should, rem := limiter.ShouldReturn429Overload("long")
	if !should {
		t.Error("long cooldown (>60s) should trigger overload return")
	}
	if rem <= 60*time.Second {
		t.Errorf("expected remaining > 60s, got %v", rem)
	}
}

func TestExecuteWith429Retry_ImmediateSuccess(t *testing.T) {
	limiter := NewLimiter()
	cfg := Default429Config()

	result := limiter.ExecuteWith429Retry(context.Background(), "cfg", cfg,
		func() error { return nil },
		func(err error) (bool, string) { return false, "" },
	)
	if !result.Succeeded {
		t.Error("expected success")
	}
	if result.Attempts != 0 {
		t.Errorf("expected 0 attempts for immediate success, got %d", result.Attempts)
	}
}

func TestExecuteWith429Retry_NonRateLimitErrorStopsImmediately(t *testing.T) {
	limiter := NewLimiter()
	cfg := Default429Config()
	callCount := 0

	result := limiter.ExecuteWith429Retry(context.Background(), "cfg", cfg,
		func() error {
			callCount++
			return fmt.Errorf("boom: not a rate limit")
		},
		func(err error) (bool, string) { return false, "" },
	)
	if result.Succeeded {
		t.Error("should not succeed")
	}
	if callCount != 1 {
		t.Errorf("should stop after first non-429 error, got %d calls", callCount)
	}
	if !strings.Contains(result.Reason, "non-429 error") {
		t.Errorf("expected non-429 reason, got %q", result.Reason)
	}
}

func TestExecuteWith429Retry_SucceedsAfterRetries(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		MaxAttempts:    5,
		BaseDelay:      5 * time.Millisecond,
		MaxDelay:       20 * time.Millisecond,
		TotalWaitCap:   1 * time.Second,
		JitterFraction: 0,
		GrowthFactor:   1.2,
	}
	callCount := 0

	result := limiter.ExecuteWith429Retry(context.Background(), "cfg", cfg,
		func() error {
			callCount++
			if callCount < 3 {
				return fmt.Errorf("429 rate limited")
			}
			return nil
		},
		func(err error) (bool, string) { return true, "Please retry in 0.01s." },
	)
	if !result.Succeeded {
		t.Errorf("expected eventual success, reason=%s", result.Reason)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestExecuteWith429Retry_ExhaustsAttempts(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		MaxAttempts:    2,
		BaseDelay:      2 * time.Millisecond,
		MaxDelay:       10 * time.Millisecond,
		TotalWaitCap:   1 * time.Second,
		JitterFraction: 0,
		GrowthFactor:   1.2,
	}

	result := limiter.ExecuteWith429Retry(context.Background(), "cfg", cfg,
		func() error { return fmt.Errorf("429 always") },
		func(err error) (bool, string) { return true, "" },
	)
	if !result.Aborted {
		t.Error("expected aborted after exhausting attempts")
	}
	if !strings.Contains(result.Reason, "exhausted") {
		t.Errorf("expected exhausted reason, got %q", result.Reason)
	}
}

func TestExecuteWith429Retry_ExceedsTotalWaitCap(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		MaxAttempts:    10,
		BaseDelay:      50 * time.Millisecond,
		MaxDelay:       50 * time.Millisecond,
		TotalWaitCap:   30 * time.Millisecond,
		JitterFraction: 0,
		GrowthFactor:   1.0,
	}

	result := limiter.ExecuteWith429Retry(context.Background(), "cfg", cfg,
		func() error { return fmt.Errorf("429 always") },
		func(err error) (bool, string) { return true, "" },
	)
	if !result.Aborted {
		t.Error("expected aborted when exceeding total wait cap")
	}
	if !strings.Contains(result.Reason, "would exceed cap") {
		t.Errorf("expected cap-exceeded reason, got %q", result.Reason)
	}
}

func TestExecuteWith429Retry_ContextCancelledBeforeAttempt(t *testing.T) {
	limiter := NewLimiter()
	cfg := Default429Config()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := limiter.ExecuteWith429Retry(ctx, "cfg", cfg,
		func() error {
			t.Error("fn should not be called when context is already cancelled")
			return nil
		},
		func(err error) (bool, string) { return true, "" },
	)
	if !result.Aborted || result.Reason != "context cancelled" {
		t.Errorf("expected aborted with 'context cancelled', got aborted=%v reason=%q", result.Aborted, result.Reason)
	}
}

func TestExecuteWith429Retry_ContextCancelledDuringWait(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		MaxAttempts:    10,
		BaseDelay:      500 * time.Millisecond,
		MaxDelay:       500 * time.Millisecond,
		TotalWaitCap:   10 * time.Second,
		JitterFraction: 0,
		GrowthFactor:   1.0,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result := limiter.ExecuteWith429Retry(ctx, "cfg", cfg,
		func() error { return fmt.Errorf("429 always") },
		func(err error) (bool, string) { return true, "" },
	)
	if !result.Aborted || !strings.Contains(result.Reason, "context cancelled during 429 backoff") {
		t.Errorf("expected aborted with cancellation-during-backoff reason, got aborted=%v reason=%q", result.Aborted, result.Reason)
	}
}

func TestCalculate429Delay_NoJitterWhenDelayIsZero(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		BaseDelay:      0,
		MaxDelay:       10 * time.Second,
		GrowthFactor:   1.5,
		JitterFraction: 0.5,
	}
	d := limiter.calculate429Delay("", 0, cfg)
	if d != 0 {
		t.Errorf("expected 0 delay when BaseDelay is 0 and no upstream wait, got %v", d)
	}
}

func TestCalculate429Delay_FinalCapAfterJitter(t *testing.T) {
	limiter := NewLimiter()
	cfg := RateLimit429Config{
		BaseDelay:      10 * time.Second,
		MaxDelay:       10 * time.Second,
		GrowthFactor:   1.5,
		JitterFraction: 0.9,
	}
	// BaseDelay == MaxDelay, so any non-zero jitter pushes the delay past
	// MaxDelay and must be clamped by the final cap after jitter is added.
	for i := 0; i < 50; i++ {
		d := limiter.calculate429Delay("", 0, cfg)
		if d > cfg.MaxDelay {
			t.Fatalf("delay %v exceeds MaxDelay %v after final cap", d, cfg.MaxDelay)
		}
	}
}

func TestCooldownAndWait(t *testing.T) {
	limiter := NewLimiter()
	ctx := context.Background()

	// No cooldown initially
	if rem := limiter.GetCooldown("test"); rem != 0 {
		t.Errorf("expected 0 cooldown, got %v", rem)
	}

	// Set a short cooldown
	limiter.Cooldown("test", 100*time.Millisecond, "test")
	if rem := limiter.GetCooldown("test"); rem == 0 {
		t.Error("expected non-zero cooldown after setting")
	}

	// Wait should succeed
	if err := limiter.Wait(ctx, "test"); err != nil {
		t.Errorf("Wait failed: %v", err)
	}

	// Cooldown should be cleared after waiting
	if rem := limiter.GetCooldown("test"); rem != 0 {
		t.Errorf("expected 0 cooldown after wait, got %v", rem)
	}
}
