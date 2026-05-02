package converter

import (
	"encoding/json"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ConvertClaudeToOpenAI converts a Claude API request to OpenAI format
// DEPRECATED: Use GlobalFactory.ConvertClaudeToOpenAI instead
func ConvertClaudeToOpenAI(claudeReq *models.ClaudeMessagesRequest) *models.OpenAIRequest {
	result := ConvertClaudeToOpenAIWithConfigAndMapping(claudeReq, config.GlobalConfig, nil)
	return result.Request
}

// ConvertClaudeToOpenAIWithConfig converts a Claude API request to OpenAI format using specific config
// DEPRECATED: Use ConvertClaudeToOpenAIWithConfigAndMapping instead
func ConvertClaudeToOpenAIWithConfig(claudeReq *models.ClaudeMessagesRequest, cfg *config.Config, betaHeaders []string) *models.OpenAIRequest {
	result := ConvertClaudeToOpenAIWithConfigAndMapping(claudeReq, cfg, betaHeaders)
	return result.Request
}

// ConversionResult holds both the OpenAI request and any metadata from the conversion.
type ConversionResult struct {
	Request        *models.OpenAIRequest
	ToolNameMapping map[string]string // truncated → original tool name mapping
}

// ConvertClaudeToOpenAIWithConfigAndMapping converts a Claude API request to OpenAI format
// and returns both the request and tool name mapping for response restoration.
func ConvertClaudeToOpenAIWithConfigAndMapping(claudeReq *models.ClaudeMessagesRequest, cfg *config.Config, betaHeaders []string) *ConversionResult {
	// Use the new converter architecture
	factory := GlobalFactory
	factory.SetOpenAIConfig(cfg)

	// Marshal Claude request to bytes
	body, err := json.Marshal(claudeReq)
	if err != nil {
		return &ConversionResult{
			Request: &models.OpenAIRequest{Model: cfg.BigModel},
		}
	}

	// Claude -> Internal -> OpenAI
	openAIBody, internalReq, err := factory.ConvertClaudeToOpenAI(body, cfg)
	if err != nil {
		return &ConversionResult{
			Request: legacyConvert(claudeReq, cfg),
		}
	}

	// Store beta headers in internal request for upstream propagation
	internalReq.BetaHeaders = betaHeaders

	// Unmarshal to OpenAI request
	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
		return &ConversionResult{
			Request: legacyConvert(claudeReq, cfg),
		}
	}

	// Build tool name mapping from the request tools (litellm pattern)
	toolNameMapping := make(map[string]string)
	for _, tool := range openAIReq.Tools {
		truncatedName := tool.Function.Name
		// Check if any Claude tool name was longer than 64 chars
		for _, claudeTool := range claudeReq.Tools {
			claudeName := claudeTool.Name
			if claudeName != "" && truncateToolName(claudeName) == truncatedName && claudeName != truncatedName {
				toolNameMapping[truncatedName] = claudeName
			}
		}
	}

	return &ConversionResult{
		Request:        &openAIReq,
		ToolNameMapping: toolNameMapping,
	}
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
