package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_BASE_URL",
		"BIG_MODEL", "MIDDLE_MODEL", "SMALL_MODEL", "HOST", "PORT",
		"LOG_LEVEL", "MAX_TOKENS_LIMIT", "MIN_TOKENS_LIMIT", "REQUEST_TIMEOUT",
		"AZURE_API_VERSION", "RETRY_COUNT", "ENABLE_REQUEST_LOGGING",
		"UPSTREAM_ENDPOINT",
	}
	for _, v := range vars {
		orig, had := os.LookupEnv(v)
		t.Cleanup(func(v string, orig string, had bool) func() {
			return func() {
				if had {
					os.Setenv(v, orig)
				} else {
					os.Unsetenv(v)
				}
			}
		}(v, orig, had))
		os.Unsetenv(v)
	}
}

func TestMaskAPIKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "****"},
		{"short_8", "12345678", "****"},
		{"long", "sk-1234567890abcdef", "sk-1...cdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskAPIKey(tc.in); got != tc.want {
				t.Errorf("maskAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPrintConfigWithoutEnv(t *testing.T) {
	clearConfigEnv(t)

	t.Chdir(t.TempDir())

	out := captureStdout(t, func() {
		if err := printConfig(); err != nil {
			t.Fatalf("printConfig() error = %v", err)
		}
	})

	if !strings.Contains(out, "Configuration") {
		t.Errorf("printConfig() output missing header, got:\n%s", out)
	}
	if !strings.Contains(out, "Not configured") {
		t.Errorf("printConfig() expected 'Not configured' for missing OpenAI key, got:\n%s", out)
	}
	if !strings.Contains(out, "Not set (validation disabled)") {
		t.Errorf("printConfig() expected anthropic key not-set branch, got:\n%s", out)
	}
	if !strings.Contains(out, "Not found (using environment variables only)") {
		t.Errorf("printConfig() expected missing .env branch, got:\n%s", out)
	}
}

func TestPrintConfigWithEnvAndDotEnvAndHeaders(t *testing.T) {
	clearConfigEnv(t)

	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("# test env file\n"), 0o644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	os.Setenv("OPENAI_API_KEY", "sk-1234567890abcdef")
	os.Setenv("ANTHROPIC_API_KEY", "ak-1234567890abcdef")
	os.Setenv("CUSTOM_HEADER_X_TEST", "hello")
	t.Cleanup(func() { os.Unsetenv("CUSTOM_HEADER_X_TEST") })

	out := captureStdout(t, func() {
		if err := printConfig(); err != nil {
			t.Fatalf("printConfig() error = %v", err)
		}
	})

	if !strings.Contains(out, "Valid format") {
		t.Errorf("printConfig() expected valid openai key branch, got:\n%s", out)
	}
	if !strings.Contains(out, "Client validation enabled") {
		t.Errorf("printConfig() expected anthropic key set branch, got:\n%s", out)
	}
	if !strings.Contains(out, "Found") {
		t.Errorf("printConfig() expected .env found branch, got:\n%s", out)
	}
	if !strings.Contains(out, "Custom Headers") {
		t.Errorf("printConfig() expected custom headers section, got:\n%s", out)
	}
}

func TestShowEnvBasedConfig(t *testing.T) {
	clearConfigEnv(t)

	os.Setenv("OPENAI_API_KEY", "sk-1234567890abcdef")
	os.Setenv("HOST", "127.0.0.1")

	out := captureStdout(t, showEnvBasedConfig)

	if !strings.Contains(out, "OPENAI_API_KEY") {
		t.Errorf("showEnvBasedConfig() missing OPENAI_API_KEY, got:\n%s", out)
	}
	if !strings.Contains(out, "127.0.0.1") {
		t.Errorf("showEnvBasedConfig() missing HOST value, got:\n%s", out)
	}
	if !strings.Contains(out, "(not set)") {
		t.Errorf("showEnvBasedConfig() expected at least one (not set) var, got:\n%s", out)
	}
}

func TestConfigCommandRegisteredAndFlags(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "config" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected config subcommand to be registered on rootCmd")
	}
	if configCmd.Flags().Lookup("validate") == nil {
		t.Error("expected config command to have --validate flag")
	}
}

func TestConfigCommandRunE(t *testing.T) {
	clearConfigEnv(t)
	t.Chdir(t.TempDir())

	out := captureStdout(t, func() {
		if err := configCmd.RunE(configCmd, nil); err != nil {
			t.Fatalf("configCmd.RunE() error = %v", err)
		}
	})
	if !strings.Contains(out, "Configuration") {
		t.Errorf("configCmd.RunE() output missing content, got:\n%s", out)
	}
}
