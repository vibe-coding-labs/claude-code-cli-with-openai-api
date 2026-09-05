package converter

import (
	"encoding/json"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// Regression guard: BuildRequest must propagate the per-config
// ReasoningEffort into the upstream request. Claude requests carry no
// reasoning effort, so without the cfg fallback the reasoning_effort column
// on api_configs was silently ignored (provider default applied instead).
func TestBuildRequest_ReasoningEffortFromConfig(t *testing.T) {
	c := NewConverterFactory()
	c.SetOpenAIConfig(&config.Config{
		BigModel:        "muse-spark-1.3-contributor",
		ReasoningEffort: "medium",
	})

	internal := &InternalRequest{
		Model:  "claude-sonnet-4-6",
		Stream: false,
	}

	body, err := c.ConvertInternalToOpenAI(internal)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("failed to parse built request: %v", err)
	}
	if out.ReasoningEffort != "medium" {
		t.Errorf("reasoning_effort = %q, want %q from converter config", out.ReasoningEffort, "medium")
	}
}

// Explicit per-request values must keep precedence over the config default.
func TestBuildRequest_ReasoningEffortRequestOverridesConfig(t *testing.T) {
	c := NewConverterFactory()
	c.SetOpenAIConfig(&config.Config{
		ReasoningEffort: "medium",
	})

	effort := "low"
	internal := &InternalRequest{
		Model:           "gpt-5",
		Stream:          false,
		ReasoningEffort: &effort,
	}

	body, err := c.ConvertInternalToOpenAI(internal)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("failed to parse built request: %v", err)
	}
	if out.ReasoningEffort != "low" {
		t.Errorf("reasoning_effort = %q, want %q (request value wins)", out.ReasoningEffort, "low")
	}
}
