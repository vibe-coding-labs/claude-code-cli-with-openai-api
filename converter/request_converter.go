package converter

import (
	"encoding/json"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// ConvertClaudeToOpenAI converts a Claude API request to OpenAI format
func ConvertClaudeToOpenAI(claudeReq *models.ClaudeMessagesRequest) *models.OpenAIRequest {
	return ConvertClaudeToOpenAIWithConfig(claudeReq, config.GlobalConfig)
}

// ConvertClaudeToOpenAIWithConfig converts a Claude API request to OpenAI format using specific config
func ConvertClaudeToOpenAIWithConfig(claudeReq *models.ClaudeMessagesRequest, cfg *config.Config) *models.OpenAIRequest {
	// Map model using the provided config
	openAIModel := utils.MapClaudeModelToOpenAIWithConfig(claudeReq.Model, cfg)

	// Convert messages
	openAIMessages := []models.OpenAIMessage{}

	// Add system message if present
	if claudeReq.System != nil {
		systemText := extractSystemText(claudeReq.System)
		if strings.TrimSpace(systemText) != "" {
			openAIMessages = append(openAIMessages, models.OpenAIMessage{
				Role:    models.RoleSystem,
				Content: strings.TrimSpace(systemText),
			})
		}
	}

	// Process Claude messages
	i := 0
	for i < len(claudeReq.Messages) {
		msg := claudeReq.Messages[i]

		if msg.Role == models.RoleUser {
			openAIMessage := convertClaudeUserMessage(&msg)
			openAIMessages = append(openAIMessages, openAIMessage)
		} else if msg.Role == models.RoleAssistant {
			openAIMessage := convertClaudeAssistantMessage(&msg)
			openAIMessages = append(openAIMessages, openAIMessage)

			// Check if next message contains tool results
			if i+1 < len(claudeReq.Messages) {
				nextMsg := claudeReq.Messages[i+1]
				if nextMsg.Role == models.RoleUser && hasToolResults(&nextMsg) {
					i++
					toolResults := convertClaudeToolResults(&nextMsg)
					openAIMessages = append(openAIMessages, toolResults...)
				}
			}
		}

		i++
	}

	// Clamp max tokens
	maxTokens := claudeReq.MaxTokens
	if maxTokens < cfg.MinTokensLimit {
		maxTokens = cfg.MinTokensLimit
	}
	if maxTokens > cfg.MaxTokensLimit {
		maxTokens = cfg.MaxTokensLimit
	}

	// Build OpenAI request
	openAIReq := &models.OpenAIRequest{
		Model:       openAIModel,
		Messages:    openAIMessages,
		MaxTokens:   maxTokens,
		Temperature: claudeReq.Temperature,
		Stream:      claudeReq.Stream,
	}

	// Add optional parameters
	if len(claudeReq.StopSequences) > 0 {
		openAIReq.Stop = claudeReq.StopSequences
	}
	if claudeReq.TopP != nil {
		openAIReq.TopP = claudeReq.TopP
	}

	// Convert tools
	if len(claudeReq.Tools) > 0 {
		openAITools := []models.OpenAITool{}
		for _, tool := range claudeReq.Tools {
			if strings.TrimSpace(tool.Name) != "" {
				openAITools = append(openAITools, models.OpenAITool{
					Type: models.ToolFunction,
					Function: models.OpenAIFunction{
						Name:        tool.Name,
						Description: tool.Description,
						Parameters:  tool.InputSchema,
					},
				})
			}
		}
		if len(openAITools) > 0 {
			openAIReq.Tools = openAITools
		}
	}

	// Convert tool choice
	if claudeReq.ToolChoice != nil {
		choiceType, _ := claudeReq.ToolChoice["type"].(string)
		if choiceType == "auto" || choiceType == "any" {
			openAIReq.ToolChoice = "auto"
		} else if choiceType == "tool" {
			if name, ok := claudeReq.ToolChoice["name"].(string); ok {
				openAIReq.ToolChoice = map[string]interface{}{
					"type": models.ToolFunction,
					"function": map[string]string{
						"name": name,
					},
				}
			}
		} else {
			openAIReq.ToolChoice = "auto"
		}
	}

	return openAIReq
}

func extractSystemText(system interface{}) string {
	if system == nil {
		return ""
	}

	// String type
	if str, ok := system.(string); ok {
		return str
	}

	// Array type
	if arr, ok := system.([]interface{}); ok {
		textParts := []string{}
		for _, item := range arr {
			if block, ok := item.(map[string]interface{}); ok {
				if blockType, _ := block["type"].(string); blockType == models.ContentText {
					if text, ok := block["text"].(string); ok {
						textParts = append(textParts, text)
					}
				}
			}
		}
		return strings.Join(textParts, "\n\n")
	}

	return ""
}

func convertClaudeUserMessage(msg *models.ClaudeMessage) models.OpenAIMessage {
	if msg.Content == nil {
		return models.OpenAIMessage{
			Role:    models.RoleUser,
			Content: "",
		}
	}

	// String content
	if str, ok := msg.Content.(string); ok {
		return models.OpenAIMessage{
			Role:    models.RoleUser,
			Content: str,
		}
	}

	// Array content (multimodal)
	if arr, ok := msg.Content.([]interface{}); ok {
		openAIContent := []models.OpenAIMessageContent{}
		for _, item := range arr {
			if block, ok := item.(map[string]interface{}); ok {
				blockType, _ := block["type"].(string)

				if blockType == models.ContentText {
					if text, ok := block["text"].(string); ok {
						openAIContent = append(openAIContent, models.OpenAIMessageContent{
							Type: "text",
							Text: text,
						})
					}
				} else if blockType == models.ContentImage {
					if source, ok := block["source"].(map[string]interface{}); ok {
						if sourceType, _ := source["type"].(string); sourceType == "base64" {
							if mediaType, ok := source["media_type"].(string); ok {
								if data, ok := source["data"].(string); ok {
									imageURL := "data:" + mediaType + ";base64," + data
									openAIContent = append(openAIContent, models.OpenAIMessageContent{
										Type: "image_url",
										ImageURL: &models.OpenAIImageURL{
											URL: imageURL,
										},
									})
								}
							}
						}
					}
				}
			}
		}

		// If only one text content, return as string
		if len(openAIContent) == 1 && openAIContent[0].Type == "text" {
			return models.OpenAIMessage{
				Role:    models.RoleUser,
				Content: openAIContent[0].Text,
			}
		}

		return models.OpenAIMessage{
			Role:    models.RoleUser,
			Content: openAIContent,
		}
	}

	return models.OpenAIMessage{
		Role:    models.RoleUser,
		Content: "",
	}
}

func convertClaudeAssistantMessage(msg *models.ClaudeMessage) models.OpenAIMessage {
	textParts := []string{}
	toolCalls := []models.OpenAIToolCall{}

	if msg.Content == nil {
		return models.OpenAIMessage{
			Role:    models.RoleAssistant,
			Content: nil,
		}
	}

	// String content
	if str, ok := msg.Content.(string); ok {
		return models.OpenAIMessage{
			Role:    models.RoleAssistant,
			Content: str,
		}
	}

	// Array content
	if arr, ok := msg.Content.([]interface{}); ok {
		for _, item := range arr {
			if block, ok := item.(map[string]interface{}); ok {
				blockType, _ := block["type"].(string)

				if blockType == models.ContentText {
					if text, ok := block["text"].(string); ok {
						textParts = append(textParts, text)
					}
				} else if blockType == models.ContentToolUse {
					id, _ := block["id"].(string)
					name, _ := block["name"].(string)
					input := block["input"]

					inputJSON, _ := json.Marshal(input)
					toolCalls = append(toolCalls, models.OpenAIToolCall{
						ID:   id,
						Type: models.ToolFunction,
						Function: models.OpenAIFunctionCall{
							Name:      name,
							Arguments: string(inputJSON),
						},
					})
				}
			}
		}
	}

	openAIMessage := models.OpenAIMessage{
		Role: models.RoleAssistant,
	}

	// Set content
	if len(textParts) > 0 {
		openAIMessage.Content = strings.Join(textParts, "")
	} else {
		openAIMessage.Content = nil
	}

	// Set tool calls
	if len(toolCalls) > 0 {
		openAIMessage.ToolCalls = toolCalls
	}

	return openAIMessage
}

func hasToolResults(msg *models.ClaudeMessage) bool {
	if arr, ok := msg.Content.([]interface{}); ok {
		for _, item := range arr {
			if block, ok := item.(map[string]interface{}); ok {
				if blockType, _ := block["type"].(string); blockType == models.ContentToolResult {
					return true
				}
			}
		}
	}
	return false
}

func convertClaudeToolResults(msg *models.ClaudeMessage) []models.OpenAIMessage {
	toolMessages := []models.OpenAIMessage{}

	if arr, ok := msg.Content.([]interface{}); ok {
		for _, item := range arr {
			if block, ok := item.(map[string]interface{}); ok {
				if blockType, _ := block["type"].(string); blockType == models.ContentToolResult {
					toolUseID, _ := block["tool_use_id"].(string)
					content := parseToolResultContent(block["content"])

					toolMessages = append(toolMessages, models.OpenAIMessage{
						Role:       models.RoleTool,
						ToolCallID: toolUseID,
						Content:    content,
					})
				}
			}
		}
	}

	return toolMessages
}

func parseToolResultContent(content interface{}) string {
	if content == nil {
		return "No content provided"
	}

	// String content
	if str, ok := content.(string); ok {
		return str
	}

	// Array content
	if arr, ok := content.([]interface{}); ok {
		resultParts := []string{}
		for _, item := range arr {
			if block, ok := item.(map[string]interface{}); ok {
				if blockType, _ := block["type"].(string); blockType == models.ContentText {
					if text, ok := block["text"].(string); ok {
						resultParts = append(resultParts, text)
					}
				} else if text, ok := block["text"].(string); ok {
					resultParts = append(resultParts, text)
				} else {
					jsonBytes, _ := json.Marshal(block)
					resultParts = append(resultParts, string(jsonBytes))
				}
			} else if str, ok := item.(string); ok {
				resultParts = append(resultParts, str)
			}
		}
		return strings.TrimSpace(strings.Join(resultParts, "\n"))
	}

	// Map content
	if block, ok := content.(map[string]interface{}); ok {
		if blockType, _ := block["type"].(string); blockType == models.ContentText {
			if text, ok := block["text"].(string); ok {
				return text
			}
		}
		jsonBytes, _ := json.Marshal(block)
		return string(jsonBytes)
	}

	// Default to string representation
	return toString(content)
}

func toString(v interface{}) string {
	if v == nil {
		return "Unparseable content"
	}
	if str, ok := v.(string); ok {
		return str
	}
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "Unparseable content"
	}
	return string(jsonBytes)
}
