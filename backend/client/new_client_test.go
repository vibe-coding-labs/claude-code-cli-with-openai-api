package client

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func TestNewOpenAIClient_RetryCountClamping(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		want       int
	}{
		{"below minimum uses default", 1, DefaultRetryCount},
		{"zero uses default", 0, DefaultRetryCount},
		{"negative uses default", -5, DefaultRetryCount},
		{"within range preserved", 10, 10},
		{"exactly minimum preserved", MinRetryCount, MinRetryCount},
		{"exactly maximum preserved", MaxRetryCount, MaxRetryCount},
		{"above maximum clamped", MaxRetryCount + 100, MaxRetryCount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{RetryCount: tt.retryCount, OpenAIBaseURL: "https://api.example.com/v1"}
			c := NewOpenAIClient(cfg)
			if c.RetryCount != tt.want {
				t.Errorf("RetryCount = %d, want %d", c.RetryCount, tt.want)
			}
		})
	}
}

func TestNewOpenAIClient_BackoffDefaults(t *testing.T) {
	cfg := &config.Config{OpenAIBaseURL: "https://api.example.com/v1"}
	c := NewOpenAIClient(cfg)
	if c.RetryBackoffBase != 1*time.Second {
		t.Errorf("expected default RetryBackoffBase=1s, got %v", c.RetryBackoffBase)
	}
	if c.RetryBackoffMax != 60*time.Second {
		t.Errorf("expected default RetryBackoffMax=60s, got %v", c.RetryBackoffMax)
	}
}

func TestNewOpenAIClient_BackoffCustomValues(t *testing.T) {
	cfg := &config.Config{
		OpenAIBaseURL:    "https://api.example.com/v1",
		RetryBackoffBase: 2.5,
		RetryBackoffMax:  30,
	}
	c := NewOpenAIClient(cfg)
	if c.RetryBackoffBase != 2500*time.Millisecond {
		t.Errorf("expected RetryBackoffBase=2.5s, got %v", c.RetryBackoffBase)
	}
	if c.RetryBackoffMax != 30*time.Second {
		t.Errorf("expected RetryBackoffMax=30s, got %v", c.RetryBackoffMax)
	}
}

func TestNewOpenAIClient_FieldsCopiedFromConfig(t *testing.T) {
	cfg := &config.Config{
		ConfigID:         "cfg-1",
		ConfigName:       "My Config",
		OpenAIAPIKey:     "sk-test",
		OpenAIBaseURL:    "https://api.example.com/v1",
		RequestTimeout:   45,
		CustomHeaders:    map[string]string{"X-Test": "1"},
		AzureAPIVersion:  "2024-01-01",
		UpstreamEndpoint: "responses",
	}
	c := NewOpenAIClient(cfg)
	if c.ConfigID != "cfg-1" || c.ConfigName != "My Config" || c.APIKey != "sk-test" ||
		c.BaseURL != "https://api.example.com/v1" || c.Timeout != 45*time.Second ||
		c.CustomHeaders["X-Test"] != "1" || c.APIVersion != "2024-01-01" ||
		c.UpstreamEndpoint != "responses" {
		t.Errorf("unexpected client fields: %+v", c)
	}
	if c.httpClient == nil {
		t.Fatalf("expected non-nil httpClient")
	}
}

// TestNewOpenAIClient_TLSHandshakeTimeoutEnvOverride covers the
// PROXY_TLS_HANDSHAKE_TIMEOUT_SEC closure inside NewOpenAIClient (re-evaluated
// on every call, unlike the package-level upstreamResponseHeaderTimeout var
// which is fixed at process init and can't be exercised per-branch from a
// test). Verifies valid override, invalid string, non-positive value, and unset.
func TestNewOpenAIClient_TLSHandshakeTimeoutEnvOverride(t *testing.T) {
	const envKey = "PROXY_TLS_HANDSHAKE_TIMEOUT_SEC"
	old, hadOld := os.LookupEnv(envKey)
	defer func() {
		if hadOld {
			os.Setenv(envKey, old)
		} else {
			os.Unsetenv(envKey)
		}
	}()

	newClientTransport := func() *http.Transport {
		cfg := &config.Config{OpenAIBaseURL: "https://api.example.com/v1"}
		c := NewOpenAIClient(cfg)
		tr, ok := c.httpClient.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("expected *http.Transport, got %T", c.httpClient.Transport)
		}
		return tr
	}

	os.Setenv(envKey, "45")
	if tr := newClientTransport(); tr.TLSHandshakeTimeout != 45*time.Second {
		t.Errorf("expected 45s override, got %v", tr.TLSHandshakeTimeout)
	}

	os.Setenv(envKey, "not-a-number")
	if tr := newClientTransport(); tr.TLSHandshakeTimeout != 300*time.Second {
		t.Errorf("expected fallback to 300s on invalid value, got %v", tr.TLSHandshakeTimeout)
	}

	os.Setenv(envKey, "-5")
	if tr := newClientTransport(); tr.TLSHandshakeTimeout != 300*time.Second {
		t.Errorf("expected fallback to 300s on non-positive value, got %v", tr.TLSHandshakeTimeout)
	}

	os.Unsetenv(envKey)
	if tr := newClientTransport(); tr.TLSHandshakeTimeout != 300*time.Second {
		t.Errorf("expected fallback to 300s when unset, got %v", tr.TLSHandshakeTimeout)
	}
}

func TestRetryDeadline(t *testing.T) {
	c := &OpenAIClient{Timeout: 2000 * time.Second}
	start := time.Now()

	t.Run("generous timeout is a no-op (deadline == start+Timeout)", func(t *testing.T) {
		d := c.retryDeadline(start)
		want := start.Add(2000 * time.Second)
		if d.Before(want.Add(-time.Second)) || d.After(want.Add(time.Second)) {
			t.Errorf("retryDeadline = %v, want ~%v", d, want)
		}
	})

	t.Run("tight timeout is floored at 2x header timeout", func(t *testing.T) {
		tight := &OpenAIClient{Timeout: 1 * time.Second}
		d := tight.retryDeadline(start)
		floor := start.Add(upstreamResponseHeaderTimeout * 2)
		if d.Before(floor.Add(-time.Second)) || d.After(floor.Add(time.Second)) {
			t.Errorf("retryDeadline = %v, want floor ~%v", d, floor)
		}
	})
}
