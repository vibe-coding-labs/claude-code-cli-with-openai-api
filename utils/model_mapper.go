package utils

import (
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

// MapClaudeModelToOpenAI maps Claude model names to OpenAI model names.
//
// This function converts Claude API model requests to the configured OpenAI-compatible models.
// It handles multiple model types and returns the appropriate mapped model name.
//
// Parameters:
//   - claudeModel: The model name from Claude API request
//     Examples:
//   - "claude-3-haiku-20240307" → returns SmallModel (e.g., "gpt-4o-mini")
//   - "claude-3-5-sonnet-20241022" → returns MiddleModel (e.g., "gpt-4o")
//   - "claude-3-opus-20240229" → returns BigModel (e.g., "gpt-4o")
//   - "gpt-4o" → returns "gpt-4o" (passed through as-is)
//   - "gpt-4-turbo" → returns "gpt-4-turbo" (passed through as-is)
//   - "o1-preview" → returns "o1-preview" (passed through as-is)
//   - "ep-20241201000000-xxxxx" → returns "ep-20241201000000-xxxxx" (ARK model, passed through)
//   - "doubao-pro-4k" → returns "doubao-pro-4k" (Doubao model, passed through)
//   - "deepseek-chat" → returns "deepseek-chat" (DeepSeek model, passed through)
//   - "unknown-model" → returns BigModel (e.g., "gpt-4o") (default fallback)
//
// Returns:
//   - string: The mapped OpenAI-compatible model name
//     The mapping rules are:
//   - Models containing "haiku" → SmallModel (default: "gpt-4o-mini")
//   - Models containing "sonnet" → MiddleModel (default: "gpt-4o")
//   - Models containing "opus" → BigModel (default: "gpt-4o")
//   - Models starting with "gpt-" or "o1-" → returned as-is (OpenAI models)
//   - Models starting with "ep-", "doubao-", or "deepseek-" → returned as-is (other providers)
//   - Unknown models → BigModel (default: "gpt-4o")
//
// Examples:
//
//	MapClaudeModelToOpenAI("claude-3-haiku-20240307")
//	// Returns: "gpt-4o-mini" (if SmallModel is configured as "gpt-4o-mini")
//
//	MapClaudeModelToOpenAI("claude-3-5-sonnet-20241022")
//	// Returns: "gpt-4o" (if MiddleModel is configured as "gpt-4o")
//
//	MapClaudeModelToOpenAI("claude-3-opus-20240229")
//	// Returns: "gpt-4o" (if BigModel is configured as "gpt-4o")
//
//	MapClaudeModelToOpenAI("gpt-4o")
//	// Returns: "gpt-4o" (passed through without mapping)
//
//	MapClaudeModelToOpenAI("unknown-model")
//	// Returns: "gpt-4o" (defaults to BigModel for unknown models)
func MapClaudeModelToOpenAI(claudeModel string) string {
	cfg := config.GlobalConfig

	modelLower := strings.ToLower(claudeModel)

	// If it's already an OpenAI model, return as-is
	// Examples: "gpt-4o", "gpt-4-turbo", "o1-preview", "o1-mini"
	if strings.HasPrefix(modelLower, "gpt-") || strings.HasPrefix(modelLower, "o1-") {
		return claudeModel
	}

	// If it's other supported models (ARK/Doubao/DeepSeek), return as-is
	// Examples: "ep-20241201000000-xxxxx", "doubao-pro-4k", "deepseek-chat"
	if strings.HasPrefix(modelLower, "ep-") || strings.HasPrefix(modelLower, "doubao-") ||
		strings.HasPrefix(modelLower, "deepseek-") {
		return claudeModel
	}

	// Map based on model naming patterns
	// Claude models are identified by keywords in their names
	if strings.Contains(modelLower, "haiku") {
		// Examples: "claude-3-haiku-20240307" → cfg.SmallModel (default: "gpt-4o-mini")
		return cfg.SmallModel
	} else if strings.Contains(modelLower, "sonnet") {
		// Examples: "claude-3-5-sonnet-20241022" → cfg.MiddleModel (default: "gpt-4o")
		return cfg.MiddleModel
	} else if strings.Contains(modelLower, "opus") {
		// Examples: "claude-3-opus-20240229" → cfg.BigModel (default: "gpt-4o")
		return cfg.BigModel
	}

	// Default to big model for unknown models
	// This ensures that unrecognized model names still get mapped to a valid model
	// Examples: "unknown-model", "test-model" → cfg.BigModel (default: "gpt-4o")
	return cfg.BigModel
}
