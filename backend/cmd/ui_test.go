package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func TestGetEnvOrDefault(t *testing.T) {
	const key = "CMD_TEST_ENV_OR_DEFAULT"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

	if got := getEnvOrDefault(key, "fallback"); got != "fallback" {
		t.Errorf("getEnvOrDefault() = %q, want fallback", got)
	}

	os.Setenv(key, "explicit")
	if got := getEnvOrDefault(key, "fallback"); got != "explicit" {
		t.Errorf("getEnvOrDefault() = %q, want explicit", got)
	}
}

func TestGetEnvAsInt(t *testing.T) {
	const key = "CMD_TEST_ENV_AS_INT"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

	if got := getEnvAsInt(key, 42); got != 42 {
		t.Errorf("getEnvAsInt() = %d, want 42 (unset)", got)
	}

	os.Setenv(key, "7")
	if got := getEnvAsInt(key, 42); got != 7 {
		t.Errorf("getEnvAsInt() = %d, want 7", got)
	}

	os.Setenv(key, "not-a-number")
	if got := getEnvAsInt(key, 42); got != 42 {
		t.Errorf("getEnvAsInt() = %d, want 42 (invalid value falls back)", got)
	}
}

func TestBuildFrontendReturnsNotImplementedError(t *testing.T) {
	if err := buildFrontend(); err == nil {
		t.Fatal("expected buildFrontend() to return an error (manual build required)")
	}
}

func TestPrintUIStartupInfo(t *testing.T) {
	cfg := &config.Config{Host: "0.0.0.0"}
	out := captureStdout(t, func() {
		printUIStartupInfo(cfg, 54988)
	})
	for _, want := range []string{"Web UI enabled", "54988", "/v1/messages", "/api/configs"} {
		if !strings.Contains(out, want) {
			t.Errorf("printUIStartupInfo() output missing %q, got:\n%s", want, out)
		}
	}
}

func TestUICommandRegisteredAndFlags(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "ui" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ui command to be registered on rootCmd")
	}
	for _, flag := range []string{"host", "port", "log-level"} {
		if uiCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected ui command to have --%s flag", flag)
		}
	}
}
