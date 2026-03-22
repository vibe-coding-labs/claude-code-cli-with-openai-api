package converter

import (
	"encoding/json"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ConvertClaudeToOpenAI converts a Claude API request to OpenAI format
// DEPRECATED: Use GlobalFactory.ConvertClaudeToOpenAI instead
func ConvertClaudeToOpenAI(claudeReq *models.ClaudeMessagesRequest) *models.OpenAIRequest {
	return ConvertClaudeToOpenAIWithConfig(claudeReq, config.GlobalConfig, nil)
}

// ConvertClaudeToOpenAIWithConfig converts a Claude API request to OpenAI format using specific config
// This function now uses the new converter architecture internally
func ConvertClaudeToOpenAIWithConfig(claudeReq *models.ClaudeMessagesRequest, cfg *config.Config, betaHeaders []string) *models.OpenAIRequest {
	// Use the new converter architecture
	factory := GlobalFactory
	factory.SetOpenAIConfig(cfg)

	// Marshal Claude request to bytes
	body, err := json.Marshal(claudeReq)
	if err != nil {
		// Fallback to empty request
		return &models.OpenAIRequest{Model: cfg.BigModel}
	}

	// Claude -> Internal -> OpenAI
	openAIBody, internalReq, err := factory.ConvertClaudeToOpenAI(body, cfg)
	if err != nil {
		// Fallback: try direct conversion
		return legacyConvert(claudeReq, cfg)
	}

	// Store beta headers in internal request for upstream propagation
	internalReq.BetaHeaders = betaHeaders

	// Store internal request in context for later use (if needed)
	_ = internalReq

	// Unmarshal to OpenAI request
	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
		return legacyConvert(claudeReq, cfg)
	}

	return &openAIReq
}

// legacyConvert is the original conversion logic as fallback
func legacyConvert(claudeReq *models.ClaudeMessagesRequest, cfg *config.Config) *models.OpenAIRequest {
	// This is a minimal fallback implementation
	// In practice, the new converter should handle all cases
	openAIModel := cfg.BigModel
	if claudeReq.Model != "" {
		// Try to determine which model category this is
		modelLower := claudeReq.Model
		if len(modelLower) >= 5 && modelLower[:5] == "claude" {
			if len(modelLower) > 10 && modelLower[6:10] == "haiku" {
				openAIModel = cfg.SmallModel
			} else if len(modelLower) > 12 && modelLower[6:12] == "sonnet" {
				openAIModel = cfg.MiddleModel
			} else if len(modelLower) > 10 && modelLower[6:10] == "opus" {
				openAIModel = cfg.BigModel
			}
		}
	}

	// Create minimal request
	openAIReq := &models.OpenAIRequest{
		Model:       openAIModel,
		MaxTokens:   claudeReq.MaxTokens,
		Temperature: claudeReq.Temperature,
		Stream:      claudeReq.Stream,
	}

	// Add reasoning effort if present
	if cfg.ReasoningEffort != "" {
		openAIReq.ReasoningEffort = cfg.ReasoningEffort
	}

	return openAIReq
}
