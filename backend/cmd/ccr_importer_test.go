package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestDetectProviderType(t *testing.T) {
	cases := []struct {
		name string
		p    ccrProvider
		want string
	}{
		{"transformer_gemini", ccrProvider{Transformer: &ccrTransformer{Use: []string{"Gemini"}}}, "gemini"},
		{"transformer_openai", ccrProvider{Transformer: &ccrTransformer{Use: []string{"openai"}}}, "openai"},
		{"transformer_anthropic", ccrProvider{Transformer: &ccrTransformer{Use: []string{"anthropic"}}}, "anthropic"},
		{"transformer_unknown_falls_through_to_url", ccrProvider{Transformer: &ccrTransformer{Use: []string{"mystery"}}, APIBaseURL: "https://api.mistral.ai/v1"}, "mistral"},
		{"url_gemini", ccrProvider{APIBaseURL: "https://generativelanguage.googleapis.com/v1beta"}, "gemini"},
		{"url_mistral", ccrProvider{APIBaseURL: "https://api.mistral.ai/v1"}, "mistral"},
		{"url_openai", ccrProvider{APIBaseURL: "https://api.openai.com/v1"}, "openai"},
		{"url_openai_compatible", ccrProvider{APIBaseURL: "https://example.com/v1"}, "openai-compatible"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectProviderType(tc.p); got != tc.want {
				t.Errorf("detectProviderType(%+v) = %q, want %q", tc.p, got, tc.want)
			}
		})
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		providerType string
		want         string
	}{
		{"gemini_generativelanguage", "https://generativelanguage.googleapis.com/v1beta", "gemini", "https://generativelanguage.googleapis.com/v1beta/openai/"},
		{"gemini_other_url_passthrough", "https://custom-gemini-proxy.example.com", "gemini", "https://custom-gemini-proxy.example.com"},
		{"mistral_strips_chat_completions", "https://api.mistral.ai/v1/chat/completions", "mistral", "https://api.mistral.ai/v1"},
		{"default_passthrough", "https://api.openai.com/v1", "openai", "https://api.openai.com/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeBaseURL(tc.raw, tc.providerType); got != tc.want {
				t.Errorf("normalizeBaseURL(%q, %q) = %q, want %q", tc.raw, tc.providerType, got, tc.want)
			}
		})
	}
}

func TestMaskKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"exactly_12", "123456789012", "****"},
		{"longer_than_12", "sk-1234567890abcdef", "sk-12345****cdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskKey(tc.in); got != tc.want {
				t.Errorf("maskKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// withFakeHome points os.UserHomeDir()'s underlying env var at a temp dir and
// restores it afterwards.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	origHome, hadHome := os.LookupEnv("HOME")
	t.Cleanup(func() {
		if hadHome {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
	})
	os.Setenv("HOME", home)
	return home
}

func setupCCRTestDB(t *testing.T) {
	t.Helper()
	tdb, err := database.InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB() error = %v", err)
	}
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("InitEncryption() error = %v", err)
	}
	t.Cleanup(func() {
		tdb.Close()
		database.DB = nil
	})
}

func TestImportCCRConfigNoFile(t *testing.T) {
	withFakeHome(t)

	imported, err := importCCRConfig()
	if err == nil {
		t.Fatal("expected error when CCR config file is missing")
	}
	if imported != 0 {
		t.Errorf("expected 0 imported, got %d", imported)
	}
}

func TestImportCCRConfigInvalidJSON(t *testing.T) {
	home := withFakeHome(t)
	dir := filepath.Join(home, ".claude-code-router")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := importCCRConfig()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func writeCCRConfig(t *testing.T, home string, cfg ccrConfig) {
	t.Helper()
	dir := filepath.Join(home, ".claude-code-router")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal ccr config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestImportCCRConfigSkipsMissingKeyOrURL(t *testing.T) {
	home := withFakeHome(t)
	setupCCRTestDB(t)

	writeCCRConfig(t, home, ccrConfig{
		Providers: []ccrProvider{
			{Name: "no-key", APIBaseURL: "https://example.com/v1"},
			{Name: "no-url", APIKey: "secret"},
		},
	})

	imported, err := importCCRConfig()
	if err != nil {
		t.Fatalf("importCCRConfig() error = %v", err)
	}
	if imported != 0 {
		t.Errorf("expected 0 imported (both providers invalid), got %d", imported)
	}
}

func TestImportCCRConfigSuccessAndDedup(t *testing.T) {
	home := withFakeHome(t)
	setupCCRTestDB(t)

	writeCCRConfig(t, home, ccrConfig{
		PORT: 3456,
		Providers: []ccrProvider{
			{
				Name:       "acme",
				APIBaseURL: "https://api.openai.com/v1",
				APIKey:     "sk-acme-0123456789",
				Models:     []string{"gpt-4o"},
			},
		},
	})

	imported, err := importCCRConfig()
	if err != nil {
		t.Fatalf("importCCRConfig() error = %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported, got %d", imported)
	}

	configs, err := database.GetAllAPIConfigs()
	if err != nil {
		t.Fatalf("GetAllAPIConfigs() error = %v", err)
	}
	found := false
	for _, c := range configs {
		if c.Name == "CCR: acme" {
			found = true
			if c.OpenAIBaseURL != "https://api.openai.com/v1" {
				t.Errorf("unexpected base url: %s", c.OpenAIBaseURL)
			}
			if c.BigModel != "gpt-4o" {
				t.Errorf("unexpected big model: %s", c.BigModel)
			}
		}
	}
	if !found {
		t.Fatal("expected imported config 'CCR: acme' to exist")
	}

	// Re-running the import should skip the already-imported provider.
	imported2, err := importCCRConfig()
	if err != nil {
		t.Fatalf("second importCCRConfig() error = %v", err)
	}
	if imported2 != 0 {
		t.Errorf("expected second import to skip existing provider, got imported=%d", imported2)
	}
}
