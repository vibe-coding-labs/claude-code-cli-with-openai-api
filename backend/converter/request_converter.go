package converter

import (
	"encoding/json"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
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
		utils.GetLogger().Error("[converter] request conversion: failed to marshal Claude request, falling back: %v", err)
		return &ConversionResult{
			Request: &models.OpenAIRequest{Model: cfg.BigModel},
		}
	}

	// Claude -> Internal -> OpenAI
	openAIBody, internalReq, err := factory.ConvertClaudeToOpenAI(body, cfg)
	if err != nil {
		utils.GetLogger().Error("[converter] request conversion: factory failed for model=%q, falling back to legacyConvert: %v", claudeReq.Model, err)
		return &ConversionResult{
			Request: legacyConvert(claudeReq, cfg),
		}
	}

	// Store beta headers in internal request for upstream propagation
	internalReq.BetaHeaders = betaHeaders

	// Unmarshal to OpenAI request
	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
		utils.GetLogger().Error("[converter] request conversion: failed to unmarshal converted body, falling back: %v", err)
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
		// Match utils.MapClaudeModelToOpenAIWithConfig's keyword-based
		// detection (the canonical, primary-path implementation). The
		// original byte-offset slicing here (modelLower[:5]=="claude",
		// modelLower[6:10]=="haiku", modelLower[6:12]=="sonnet") never
		// matched any real Claude model name — "claude" is 6 bytes, not 5,
		// and real names have a "-" separator at index 6 (e.g.
		// "claude-3-haiku-20240307"), so the outer condition was always
		// false and this fallback silently ignored the requested tier,
		// routing every request to cfg.BigModel regardless of whether
		// haiku/sonnet/opus was requested.
		modelLower := strings.ToLower(claudeReq.Model)
		if strings.Contains(modelLower, "haiku") {
			openAIModel = cfg.SmallModel
		} else if strings.Contains(modelLower, "sonnet") {
			openAIModel = cfg.MiddleModel
		} else if strings.Contains(modelLower, "opus") {
			openAIModel = cfg.BigModel
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
