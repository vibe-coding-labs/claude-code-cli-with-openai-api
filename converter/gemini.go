package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// GeminiConverter handles conversion between Gemini API format and internal format
type GeminiConverter struct{}

// NewGeminiConverter creates a new Gemini converter
func NewGeminiConverter() *GeminiConverter {
	return &GeminiConverter{}
}

// geminiSchemaAllowed - whitelist of supported JSON Schema fields for Gemini
var geminiSchemaAllowed = map[string]bool{
	"type":        true,
	"description": true,
	"properties":  true,
	"required":    true,
	"enum":        true,
	"items":       true,
	"anyOf":       true,
	"allOf":       true,
	"oneOf":       true,
	"nullable":    true,
	"format":      true,
	"title":       true,
	"default":     true,
	"minimum":     true,
	"maximum":     true,
	"minLength":   true,
	"maxLength":   true,
	"pattern":     true,
}

// stripUnsupportedSchemaFields removes unsupported JSON Schema fields for Gemini
func stripUnsupportedSchemaFields(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})
	for key, value := range schema {
		if geminiSchemaAllowed[key] {
			// Recursively process nested schemas
			switch v := value.(type) {
			case map[string]interface{}:
				result[key] = stripUnsupportedSchemaFields(v)
			case []interface{}:
				result[key] = stripUnsupportedSchemaArray(v)
			default:
				result[key] = value
			}
		}
	}
	return result
}

// stripUnsupportedSchemaArray processes schema arrays
func stripUnsupportedSchemaArray(arr []interface{}) []interface{} {
	result := make([]interface{}, len(arr))
	for i, item := range arr {
		switch v := item.(type) {
		case map[string]interface{}:
			result[i] = stripUnsupportedSchemaFields(v)
		default:
			result[i] = item
		}
	}
	return result
}

// ParseRequest parses a Gemini request into internal format
func (g *GeminiConverter) ParseRequest(body []byte) (*InternalRequest, error) {
	var geminiReq models.GeminiRequest
	if err := json.Unmarshal(body, &geminiReq); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini request: %w", err)
	}

	req := &InternalRequest{
		Messages: make([]InternalMessage, 0),
		Metadata: make(map[string]interface{}),
	}

	// Extract system instruction
	if geminiReq.SystemInstruction != nil && len(geminiReq.SystemInstruction.Parts) > 0 {
		var systemTexts []string
		for _, part := range geminiReq.SystemInstruction.Parts {
			if part.Text != "" {
				systemTexts = append(systemTexts, part.Text)
			}
		}
		req.System = strings.Join(systemTexts, "\n")
	}

	// Extract generation config
	if geminiReq.GenerationConfig != nil {
		if geminiReq.GenerationConfig.Temperature != nil {
			req.Temperature = geminiReq.GenerationConfig.Temperature
		}
		if geminiReq.GenerationConfig.TopP != nil {
			req.TopP = geminiReq.GenerationConfig.TopP
		}
		if geminiReq.GenerationConfig.TopK != nil {
			req.TopK = geminiReq.GenerationConfig.TopK
		}
		req.MaxTokens = geminiReq.GenerationConfig.MaxOutputTokens
		req.StopSeqs = geminiReq.GenerationConfig.StopSequences
	}

	// Convert messages
	for _, content := range geminiReq.Contents {
		role := mapGeminiRoleToInternal(content.Role)
		msg := InternalMessage{Role: role}

		for _, part := range content.Parts {
			cb := ContentBlock{}

			if part.Text != "" {
				cb.Type = "text"
				cb.Text = part.Text
				msg.Content = append(msg.Content, cb)
			} else if part.InlineData != nil {
				// Handle multimodal content
				mimeType := part.InlineData.MimeType
				if strings.HasPrefix(mimeType, "image/") {
					cb.Type = "image"
					cb.Source = &ImageSource{
						Type:      "base64",
						MediaType: mimeType,
						Data:      part.InlineData.Data,
					}
				} else if strings.HasPrefix(mimeType, "video/") {
					cb.Type = "video"
					cb.VideoSource = &VideoSource{
						Type:      "base64",
						MediaType: mimeType,
						Data:      part.InlineData.Data,
					}
				} else if strings.HasPrefix(mimeType, "audio/") {
					cb.Type = "audio"
					cb.AudioSource = &AudioSource{
						Type:      "base64",
						MediaType: mimeType,
						Data:      part.InlineData.Data,
					}
				}
				msg.Content = append(msg.Content, cb)
			} else if part.FunctionCall != nil {
				cb.Type = "tool_use"
				cb.ID = fmt.Sprintf("call_%s", part.FunctionCall.Name)
				cb.Name = part.FunctionCall.Name
				cb.Input = part.FunctionCall.Args
				msg.Content = append(msg.Content, cb)
			} else if part.FunctionResponse != nil {
				cb.Type = "tool_result"
				cb.ToolUseID = part.FunctionResponse.Name
				if resp, ok := part.FunctionResponse.Response.(string); ok {
					cb.Content = resp
				} else {
					cb.Content = fmt.Sprintf("%v", part.FunctionResponse.Response)
				}
				msg.Content = append(msg.Content, cb)
			}
		}

		req.Messages = append(req.Messages, msg)
	}

	// Convert tools
	for _, tool := range geminiReq.Tools {
		for _, fn := range tool.FunctionDeclarations {
			req.Tools = append(req.Tools, ToolDefinition{
				Name:        fn.Name,
				Description: fn.Description,
				Parameters:  stripUnsupportedSchemaFields(fn.Parameters),
			})
		}
	}

	// Convert tool config
	if geminiReq.ToolConfig != nil && geminiReq.ToolConfig.FunctionCallingConfig != nil {
		mode := geminiReq.ToolConfig.FunctionCallingConfig.Mode
		switch mode {
		case "ANY":
			req.ToolChoice = "any"
		case "NONE":
			req.ToolChoice = "none"
		default:
			req.ToolChoice = "auto"
		}
	}

	return req, nil
}

// BuildRequest builds a Gemini request from internal format
func (g *GeminiConverter) BuildRequest(req *InternalRequest) ([]byte, error) {
	geminiReq := models.GeminiRequest{
		Contents: make([]models.GeminiContent, 0),
	}

	// Add system instruction
	if req.System != "" {
		geminiReq.SystemInstruction = &models.GeminiContent{
			Parts: []models.GeminiPart{
				{Text: req.System},
			},
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		role := mapInternalRoleToGemini(msg.Role)
		content := models.GeminiContent{
			Role:  role,
			Parts: make([]models.GeminiPart, 0),
		}

		for _, cb := range msg.Content {
			switch cb.Type {
			case "text":
				content.Parts = append(content.Parts, models.GeminiPart{Text: cb.Text})
			case "image":
				if cb.Source != nil {
					content.Parts = append(content.Parts, models.GeminiPart{
						InlineData: &models.GeminiInlineData{
							MimeType: cb.Source.MediaType,
							Data:     cb.Source.Data,
						},
					})
				}
			case "video":
				if cb.VideoSource != nil {
					content.Parts = append(content.Parts, models.GeminiPart{
						InlineData: &models.GeminiInlineData{
							MimeType: cb.VideoSource.MediaType,
							Data:     cb.VideoSource.Data,
						},
					})
				}
			case "audio":
				if cb.AudioSource != nil {
					content.Parts = append(content.Parts, models.GeminiPart{
						InlineData: &models.GeminiInlineData{
							MimeType: cb.AudioSource.MediaType,
							Data:     cb.AudioSource.Data,
						},
					})
				}
			case "tool_use":
				content.Parts = append(content.Parts, models.GeminiPart{
					FunctionCall: &models.GeminiFunctionCall{
						Name: cb.Name,
						Args: cb.Input,
					},
				})
			case "tool_result":
				content.Parts = append(content.Parts, models.GeminiPart{
					FunctionResponse: &models.GeminiFunctionResponse{
						Name:     cb.ToolUseID,
						Response: cb.Content,
					},
				})
			}
		}

		geminiReq.Contents = append(geminiReq.Contents, content)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		fnDecls := make([]models.GeminiFunctionDeclaration, 0)
		for _, tool := range req.Tools {
			fnDecls = append(fnDecls, models.GeminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			})
		}
		geminiReq.Tools = []models.GeminiTool{{
			FunctionDeclarations: fnDecls,
		}}
	}

	// Convert generation config
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens > 0 || len(req.StopSeqs) > 0 {
		config := &models.GeminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			StopSequences:   req.StopSeqs,
		}
		if req.Temperature != nil {
			config.Temperature = req.Temperature
		}
		if req.TopP != nil {
			config.TopP = req.TopP
		}
		geminiReq.GenerationConfig = config
	}

	return json.Marshal(geminiReq)
}

// ParseResponse parses a Gemini response into internal format
func (g *GeminiConverter) ParseResponse(body []byte) (*InternalResponse, error) {
	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := geminiResp.Candidates[0]
	resp := &InternalResponse{
		ID:   fmt.Sprintf("gemini-%d", candidate.Index),
		Role: "assistant",
	}

	// Map finish reason
	switch candidate.FinishReason {
	case "STOP":
		resp.StopReason = "end_turn"
	case "MAX_TOKENS":
		resp.StopReason = "max_tokens"
	case "SAFETY":
		resp.StopReason = "content_filter"
	case "RECITATION":
		resp.StopReason = "content_filter"
	default:
		resp.StopReason = "end_turn"
	}

	// Convert content
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			resp.Content = append(resp.Content, ContentBlock{
				Type: "text",
				Text: part.Text,
			})
		} else if part.FunctionCall != nil {
			resp.Content = append(resp.Content, ContentBlock{
				Type:  "tool_use",
				ID:    fmt.Sprintf("call_%s", part.FunctionCall.Name),
				Name:  part.FunctionCall.Name,
				Input: part.FunctionCall.Args,
			})
		}
	}

	// Convert usage
	if geminiResp.UsageMetadata != nil {
		resp.Usage = &UsageInfo{
			InputTokens:  geminiResp.UsageMetadata.PromptTokenCount,
			OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	return resp, nil
}

// BuildResponse builds a Gemini response from internal format
func (g *GeminiConverter) BuildResponse(resp *InternalResponse) ([]byte, error) {
	candidate := models.GeminiCandidate{
		Content: models.GeminiContent{
			Role:  "model",
			Parts: make([]models.GeminiPart, 0),
		},
		Index: 0,
	}

	// Map stop reason
	switch resp.StopReason {
	case "max_tokens":
		candidate.FinishReason = "MAX_TOKENS"
	case "tool_use":
		candidate.FinishReason = "STOP"
	default:
		candidate.FinishReason = "STOP"
	}

	// Convert content
	for _, cb := range resp.Content {
		switch cb.Type {
		case "text":
			candidate.Content.Parts = append(candidate.Content.Parts, models.GeminiPart{
				Text: cb.Text,
			})
		case "tool_use":
			candidate.Content.Parts = append(candidate.Content.Parts, models.GeminiPart{
				FunctionCall: &models.GeminiFunctionCall{
					Name: cb.Name,
					Args: cb.Input,
				},
			})
		}
	}

	geminiResp := models.GeminiResponse{
		Candidates: []models.GeminiCandidate{candidate},
	}

	if resp.Usage != nil {
		geminiResp.UsageMetadata = &models.GeminiUsageMetadata{
			PromptTokenCount:     resp.Usage.InputTokens,
			CandidatesTokenCount: resp.Usage.OutputTokens,
			TotalTokenCount:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return json.Marshal(geminiResp)
}

// ParseStreamEvent parses a Gemini streaming event
func (g *GeminiConverter) ParseStreamEvent(line []byte) (*StreamEvent, error) {
	var geminiEvent models.GeminiStreamEvent
	if err := json.Unmarshal(line, &geminiEvent); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini stream event: %w", err)
	}

	event := &StreamEvent{}

	if len(geminiEvent.Candidates) > 0 {
		candidate := geminiEvent.Candidates[0]
		if len(candidate.Content.Parts) > 0 {
			part := candidate.Content.Parts[0]
			if part.Text != "" {
				event.Delta = &StreamDelta{
					Type: "text_delta",
					Text: part.Text,
				}
			} else if part.FunctionCall != nil {
				event.Delta = &StreamDelta{
					Type: "tool_call_delta",
					Text: fmt.Sprintf("%v", part.FunctionCall.Args),
				}
			}
		}

		// Map finish reason
		if candidate.FinishReason != "" {
			switch candidate.FinishReason {
			case "MAX_TOKENS":
				event.Delta.StopReason = "max_tokens"
			case "STOP":
				event.Delta.StopReason = "end_turn"
			case "SAFETY":
				event.Delta.StopReason = "content_filter"
			}
		}
	}

	if geminiEvent.UsageMetadata != nil {
		event.Usage = &UsageInfo{
			InputTokens:  geminiEvent.UsageMetadata.PromptTokenCount,
			OutputTokens: geminiEvent.UsageMetadata.CandidatesTokenCount,
		}
	}

	return event, nil
}

// BuildStreamEvent builds a Gemini streaming event from internal format
func (g *GeminiConverter) BuildStreamEvent(event *StreamEvent) ([]byte, error) {
	candidate := models.GeminiCandidate{
		Content: models.GeminiContent{
			Role:  "model",
			Parts: make([]models.GeminiPart, 0),
		},
	}

	if event.Delta != nil {
		if event.Delta.Text != "" {
			candidate.Content.Parts = append(candidate.Content.Parts, models.GeminiPart{
				Text: event.Delta.Text,
			})
		}

		// Map stop reason
		switch event.Delta.StopReason {
		case "max_tokens":
			candidate.FinishReason = "MAX_TOKENS"
		case "end_turn":
			candidate.FinishReason = "STOP"
		case "content_filter":
			candidate.FinishReason = "SAFETY"
		}
	}

	geminiEvent := models.GeminiStreamEvent{
		Candidates: []models.GeminiCandidate{candidate},
	}

	if event.Usage != nil {
		geminiEvent.UsageMetadata = &models.GeminiUsageMetadata{
			PromptTokenCount:     event.Usage.InputTokens,
			CandidatesTokenCount: event.Usage.OutputTokens,
		}
	}

	return json.Marshal(geminiEvent)
}

// mapGeminiRoleToInternal maps Gemini roles to internal roles
func mapGeminiRoleToInternal(role string) string {
	switch role {
	case "user":
		return "user"
	case "model":
		return "assistant"
	default:
		return "user"
	}
}

// mapInternalRoleToGemini maps internal roles to Gemini roles
func mapInternalRoleToGemini(role string) string {
	switch role {
	case "user":
		return "user"
	case "assistant":
		return "model"
	default:
		return "user"
	}
}
