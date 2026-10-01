package utils

import (
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func testCfg() *config.Config {
	return &config.Config{
		BigModel:    "big-model",
		MiddleModel: "middle-model",
		SmallModel:  "small-model",
	}
}

func TestMapClaudeModelToOpenAIWithConfig(t *testing.T) {
	cfg := testCfg()

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"openai gpt- prefix passthrough", "gpt-4o", "gpt-4o"},
		{"openai o1- prefix passthrough", "o1-preview", "o1-preview"},
		{"codex- prefix passthrough", "codex-mini", "codex-mini"},
		{"contains -codex passthrough", "gpt-5-codex", "gpt-5-codex"},
		{"ark ep- prefix passthrough", "ep-20241201000000-xxxxx", "ep-20241201000000-xxxxx"},
		{"doubao- prefix passthrough", "doubao-pro-4k", "doubao-pro-4k"},
		{"deepseek- prefix passthrough", "deepseek-chat", "deepseek-chat"},
		{"claude haiku maps to small model", "claude-3-haiku-20240307", cfg.SmallModel},
		{"claude sonnet maps to middle model", "claude-3-5-sonnet-20241022", cfg.MiddleModel},
		{"claude opus maps to big model", "claude-3-opus-20240229", cfg.BigModel},
		{"case insensitive haiku match", "Claude-3-HAIKU-20240307", cfg.SmallModel},
		{"unknown model defaults to big model", "some-unrecognized-model", cfg.BigModel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapClaudeModelToOpenAIWithConfig(tt.model, cfg)
			if got != tt.want {
				t.Errorf("MapClaudeModelToOpenAIWithConfig(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestMapClaudeModelToOpenAI_UsesGlobalConfig(t *testing.T) {
	orig := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = orig })

	config.GlobalConfig = testCfg()

	if got := MapClaudeModelToOpenAI("claude-3-opus-20240229"); got != config.GlobalConfig.BigModel {
		t.Errorf("MapClaudeModelToOpenAI() = %q, want %q", got, config.GlobalConfig.BigModel)
	}
	if got := MapClaudeModelToOpenAI("gpt-4o"); got != "gpt-4o" {
		t.Errorf("MapClaudeModelToOpenAI() = %q, want passthrough gpt-4o", got)
	}
}

func TestGetReasoningEffortForModel(t *testing.T) {
	base := func() *database.APIConfig {
		return &database.APIConfig{
			BigModel:        "big",
			MiddleModel:     "middle",
			SmallModel:      "small",
			ReasoningEffort: "global-default",
		}
	}

	t.Run("big model with specific effort", func(t *testing.T) {
		ac := base()
		ac.BigModelReasoningEffort = "high"
		if got := GetReasoningEffortForModel(ac, "big"); got != "high" {
			t.Errorf("got %q, want high", got)
		}
	})

	t.Run("big model without specific effort falls back to global", func(t *testing.T) {
		ac := base()
		if got := GetReasoningEffortForModel(ac, "big"); got != "global-default" {
			t.Errorf("got %q, want global-default", got)
		}
	})

	t.Run("middle model with specific effort", func(t *testing.T) {
		ac := base()
		ac.MiddleModelReasoningEffort = "medium"
		if got := GetReasoningEffortForModel(ac, "middle"); got != "medium" {
			t.Errorf("got %q, want medium", got)
		}
	})

	t.Run("middle model without specific effort falls back to global", func(t *testing.T) {
		ac := base()
		if got := GetReasoningEffortForModel(ac, "middle"); got != "global-default" {
			t.Errorf("got %q, want global-default", got)
		}
	})

	t.Run("small model with specific effort", func(t *testing.T) {
		ac := base()
		ac.SmallModelReasoningEffort = "low"
		if got := GetReasoningEffortForModel(ac, "small"); got != "low" {
			t.Errorf("got %q, want low", got)
		}
	})

	t.Run("small model without specific effort falls back to global", func(t *testing.T) {
		ac := base()
		if got := GetReasoningEffortForModel(ac, "small"); got != "global-default" {
			t.Errorf("got %q, want global-default", got)
		}
	})

	t.Run("model matching none of the categories falls back to global", func(t *testing.T) {
		ac := base()
		if got := GetReasoningEffortForModel(ac, "unrelated-model"); got != "global-default" {
			t.Errorf("got %q, want global-default", got)
		}
	})
}
