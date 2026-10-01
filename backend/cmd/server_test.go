package cmd

import (
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func TestPrintStartupInfoWithAnthropicKeyAndHeaders(t *testing.T) {
	cfg := &config.Config{
		OpenAIBaseURL:   "https://api.openai.com/v1",
		BigModel:        "gpt-4o",
		MiddleModel:     "gpt-4o",
		SmallModel:      "gpt-4o-mini",
		MaxTokensLimit:  4096,
		RequestTimeout:  1800,
		Host:            "0.0.0.0",
		AnthropicAPIKey: "ak-secret",
		CustomHeaders:   map[string]string{"X-Test": "1"},
	}

	out := captureStdout(t, func() {
		printStartupInfo(cfg, 54988)
	})

	for _, want := range []string{
		"Configuration loaded successfully",
		"gpt-4o-mini",
		"Client API Key Validation:",
		"Enabled",
		"Custom Headers:",
		"1 configured",
		"your-matching-key",
		"54988",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printStartupInfo() output missing %q, got:\n%s", want, out)
		}
	}
}

func TestPrintStartupInfoWithoutAnthropicKeyOrHeaders(t *testing.T) {
	cfg := &config.Config{
		OpenAIBaseURL:  "https://api.openai.com/v1",
		BigModel:       "gpt-4o",
		MiddleModel:    "gpt-4o",
		SmallModel:     "gpt-4o-mini",
		MaxTokensLimit: 4096,
		RequestTimeout: 1800,
		Host:           "0.0.0.0",
	}

	out := captureStdout(t, func() {
		printStartupInfo(cfg, 54988)
	})

	if !strings.Contains(out, "Disabled") {
		t.Errorf("printStartupInfo() expected 'Disabled' for missing anthropic key, got:\n%s", out)
	}
	if !strings.Contains(out, "any-value") {
		t.Errorf("printStartupInfo() expected any-value usage hint, got:\n%s", out)
	}
	if strings.Contains(out, "Custom Headers:") {
		t.Errorf("printStartupInfo() should not print Custom Headers section when empty, got:\n%s", out)
	}
}

func TestServerCommandRegisteredAndFlags(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "server" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected server command to be registered on rootCmd")
	}
	for _, flag := range []string{"host", "port", "log-level", "openai-url", "big-model", "middle-model", "small-model"} {
		if serverCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected server command to have --%s flag", flag)
		}
	}
}
