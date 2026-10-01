package converter

import (
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func testConfig() *config.Config {
	return &config.Config{
		OpenAIBaseURL: "https://api.openai.com/v1",
		BigModel:      "gpt-4o",
		MiddleModel:   "gpt-4o",
		SmallModel:    "gpt-4o-mini",
	}
}

func TestConvertClaudeToOpenAIWithConfigAndMapping_HappyPath(t *testing.T) {
	cfg := testConfig()
	req := &models.ClaudeMessagesRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	result := ConvertClaudeToOpenAIWithConfigAndMapping(req, cfg, []string{"beta-1"})
	if result == nil || result.Request == nil {
		t.Fatal("expected a non-nil conversion result and request")
	}
	if result.Request.Model == "" {
		t.Error("expected a mapped model name")
	}
	if len(result.Request.Messages) == 0 {
		t.Error("expected at least one converted message")
	}
}

func TestConvertClaudeToOpenAIWithConfigAndMapping_ToolNameTruncation(t *testing.T) {
	cfg := testConfig()
	longName := strings.Repeat("a", 80) // > maxToolNameLength (64)
	req := &models.ClaudeMessagesRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []models.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []models.ClaudeTool{
			{Name: longName, Description: "a tool with a very long name", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	result := ConvertClaudeToOpenAIWithConfigAndMapping(req, cfg, nil)
	if result == nil || result.Request == nil {
		t.Fatal("expected a non-nil conversion result and request")
	}
	if len(result.Request.Tools) != 1 {
		t.Fatalf("expected 1 tool in converted request, got %d", len(result.Request.Tools))
	}
	truncatedName := result.Request.Tools[0].Function.Name
	if truncatedName == longName {
		t.Fatal("expected the tool name to be truncated (> 64 chars)")
	}
	original, ok := result.ToolNameMapping[truncatedName]
	if !ok {
		t.Fatalf("expected ToolNameMapping to record truncated -> original, got %v", result.ToolNameMapping)
	}
	if original != longName {
		t.Errorf("expected original name %q, got %q", longName, original)
	}
}

// TestConvertClaudeToOpenAIWithConfigAndMapping_MarshalError covers the first
// fallback branch: when the incoming *models.ClaudeMessagesRequest can't be
// re-marshaled to JSON (e.g. its System field holds a channel, which
// encoding/json can never serialize), the function must not panic and must
// fall back to a minimal request using cfg.BigModel.
func TestConvertClaudeToOpenAIWithConfigAndMapping_MarshalError(t *testing.T) {
	cfg := testConfig()
	req := &models.ClaudeMessagesRequest{
		Model:  "claude-3-5-sonnet-20241022",
		System: make(chan int), // unmarshalable
	}

	result := ConvertClaudeToOpenAIWithConfigAndMapping(req, cfg, nil)
	if result == nil || result.Request == nil {
		t.Fatal("expected a non-nil fallback result")
	}
	if result.Request.Model != cfg.BigModel {
		t.Errorf("expected fallback model %q, got %q", cfg.BigModel, result.Request.Model)
	}
}

func TestConvertClaudeToOpenAI_Wrapper(t *testing.T) {
	old := config.GlobalConfig
	config.GlobalConfig = testConfig()
	defer func() { config.GlobalConfig = old }()

	req := &models.ClaudeMessagesRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 100,
		Messages:  []models.ClaudeMessage{{Role: "user", Content: "hi"}},
	}
	got := ConvertClaudeToOpenAI(req)
	if got == nil {
		t.Fatal("expected a non-nil *models.OpenAIRequest")
	}
}

func TestConvertClaudeToOpenAIWithConfig_Wrapper(t *testing.T) {
	cfg := testConfig()
	req := &models.ClaudeMessagesRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 100,
		Messages:  []models.ClaudeMessage{{Role: "user", Content: "hi"}},
	}
	got := ConvertClaudeToOpenAIWithConfig(req, cfg, []string{"beta-x"})
	if got == nil {
		t.Fatal("expected a non-nil *models.OpenAIRequest")
	}
}

// TestLegacyConvert_ModelTierMapping is a regression test for a real bug: the
// original byte-offset string slicing (modelLower[:5]=="claude",
// modelLower[6:10]=="haiku", modelLower[6:12]=="sonnet") never matched any
// real Claude model name, because "claude" is 6 bytes (not 5) and real names
// have a "-" separator at index 6 (e.g. "claude-3-haiku-20240307"). This
// silently broke the legacyConvert fallback path: whenever the primary
// converter failed and this safety net kicked in, every request — regardless
// of whether haiku/sonnet/opus was requested — was routed to cfg.BigModel.
// Fixed to reuse the same strings.Contains keyword matching as the canonical
// utils.MapClaudeModelToOpenAIWithConfig.
func TestLegacyConvert_ModelTierMapping(t *testing.T) {
	cfg := testConfig()
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-3-haiku-20240307", cfg.SmallModel},
		{"claude-3-5-sonnet-20241022", cfg.MiddleModel},
		{"claude-sonnet-4-5", cfg.MiddleModel},
		{"claude-3-opus-20240229", cfg.BigModel},
		{"claude-opus-4-1", cfg.BigModel},
		{"", cfg.BigModel},
		{"some-unknown-model", cfg.BigModel},
	}
	for _, tt := range tests {
		req := &models.ClaudeMessagesRequest{Model: tt.model, MaxTokens: 512}
		got := legacyConvert(req, cfg)
		if got.Model != tt.expected {
			t.Errorf("legacyConvert(model=%q).Model = %q, want %q", tt.model, got.Model, tt.expected)
		}
	}
}

func TestLegacyConvert_CarriesBasicFields(t *testing.T) {
	cfg := testConfig()
	cfg.ReasoningEffort = "high"
	temp := 0.7
	req := &models.ClaudeMessagesRequest{
		Model:       "claude-3-5-sonnet-20241022",
		MaxTokens:   2048,
		Temperature: temp,
		Stream:      true,
	}
	got := legacyConvert(req, cfg)
	if got.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", got.MaxTokens)
	}
	if got.Temperature != temp {
		t.Errorf("Temperature = %v, want %v", got.Temperature, temp)
	}
	if !got.Stream {
		t.Error("expected Stream to be true")
	}
	if got.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want %q", got.ReasoningEffort, "high")
	}
}
