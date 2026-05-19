package ratelimit

import (
	"context"
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
