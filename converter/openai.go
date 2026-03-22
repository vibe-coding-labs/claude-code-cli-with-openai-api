package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// OpenAIConverter 处理 OpenAI API 格式与内部格式之间的转换
type OpenAIConverter struct {
	cfg *config.Config
}

// NewOpenAIConverter 创建新的 OpenAI 转换器
func NewOpenAIConverter(cfg *config.Config) *OpenAIConverter {
	return &OpenAIConverter{cfg: cfg}
}

// SetConfig 设置配置
func (o *OpenAIConverter) SetConfig(cfg *config.Config) {
	o.cfg = cfg
}

// ParseRequest 将 OpenAI 请求解析为内部格式
func (o *OpenAIConverter) ParseRequest(body []byte) (*InternalRequest, error) {
	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		return nil, fmt.Errorf("failed to parse openai request: %w", err)
	}

	req := &InternalRequest{
		Model:           openAIReq.Model,
		Temperature:     &openAIReq.Temperature,
		Stream:          openAIReq.Stream,
		ReasoningEffort: &openAIReq.ReasoningEffort,
		Metadata:        make(map[string]interface{}),
	}

	// Parse stop sequences - support string or []string
	if openAIReq.Stop != nil {
		switch s := openAIReq.Stop.(type) {
		case string:
			req.StopSeqs = []string{s}
		case []string:
			req.StopSeqs = s
		case []interface{}:
			for _, v := range s {
				if str, ok := v.(string); ok {
					req.StopSeqs = append(req.StopSeqs, str)
				}
			}
		}
	}

	// Support max_completion_tokens for o1/o3 models
	if openAIReq.MaxCompletionTokens > 0 {
		req.MaxTokens = openAIReq.MaxCompletionTokens
	} else if openAIReq.MaxTokens > 0 {
		req.MaxTokens = openAIReq.MaxTokens
	}

	if openAIReq.TopP != nil {
		req.TopP = openAIReq.TopP
	}

	// 解析 messages
	for _, msg := range openAIReq.Messages {
		// 处理系统消息 - 提取到 req.System
		if msg.Role == "system" {
			systemText := ""
			if msg.Content != nil {
				if str, ok := msg.Content.(string); ok {
					systemText = str
				}
			}
			if req.System != "" {
				req.System += "\n" + systemText
			} else {
				req.System = systemText
			}
			continue
		}

		internalMsg := o.convertOpenAIMessageToInternal(&msg)
		req.Messages = append(req.Messages, internalMsg)
	}

	// 解析 tools
	for _, tool := range openAIReq.Tools {
		req.Tools = append(req.Tools, ToolDefinition{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}

	// 解析 tool_choice
	if openAIReq.ToolChoice != nil {
		req.ToolChoice = openAIReq.ToolChoice
	}

	return req, nil
}

// convertOpenAIMessageToInternal 将 OpenAI 消息转换为内部格式
func (o *OpenAIConverter) convertOpenAIMessageToInternal(msg *models.OpenAIMessage) InternalMessage {
	internalMsg := InternalMessage{Role: msg.Role}

	// 处理 content
	if msg.Content != nil {
		switch content := msg.Content.(type) {
		case string:
			internalMsg.Content = []ContentBlock{
				{Type: "text", Text: content},
			}
		case []interface{}:
			// 多模态内容
			for _, item := range content {
				if blockMap, ok := item.(map[string]interface{}); ok {
					blockType, _ := blockMap["type"].(string)
					cb := ContentBlock{Type: blockType}

					switch blockType {
					case "text":
						cb.Text, _ = blockMap["text"].(string)
					case "image_url":
						if imageURL, ok := blockMap["image_url"].(map[string]interface{}); ok {
							urlStr, _ := imageURL["url"].(string)
							// 解析data URL或外部URL
							if strings.HasPrefix(urlStr, "data:") {
								// data:image/png;base64,<data>
								parts := strings.SplitN(urlStr, ",", 2)
								if len(parts) == 2 {
									mediaInfo := strings.TrimPrefix(parts[0], "data:")
									mediaType := strings.TrimSuffix(mediaInfo, ";base64")
									cb.Type = "image"
									cb.Source = &ImageSource{
										Type:      "base64",
										MediaType: mediaType,
										Data:      parts[1],
									}
								}
							} else {
								// 外部URL
								cb.Type = "image"
								cb.Source = &ImageSource{
									Type:      "url",
									MediaType: "image/url",
									Data:      urlStr,
								}
							}
						}
					case "video_url":
						if videoURL, ok := blockMap["video_url"].(map[string]interface{}); ok {
							urlStr, _ := videoURL["url"].(string)
							// 解析data URL或外部URL
							if strings.HasPrefix(urlStr, "data:") {
								parts := strings.SplitN(urlStr, ",", 2)
								if len(parts) == 2 {
									mediaInfo := strings.TrimPrefix(parts[0], "data:")
									mediaType := strings.TrimSuffix(mediaInfo, ";base64")
									cb.Type = "video"
									cb.VideoSource = &VideoSource{
										Type:      "base64",
										MediaType: mediaType,
										Data:      parts[1],
									}
								}
							} else {
								// 外部URL
								cb.Type = "video"
								cb.VideoSource = &VideoSource{
									Type:      "url",
									MediaType: "video/url",
									Data:      urlStr,
								}
							}
						}
					case "audio_url":
						if audioURL, ok := blockMap["audio_url"].(map[string]interface{}); ok {
							urlStr, _ := audioURL["url"].(string)
							// 解析data URL或外部URL
							if strings.HasPrefix(urlStr, "data:") {
								parts := strings.SplitN(urlStr, ",", 2)
								if len(parts) == 2 {
									mediaInfo := strings.TrimPrefix(parts[0], "data:")
									mediaType := strings.TrimSuffix(mediaInfo, ";base64")
									cb.Type = "audio"
									cb.AudioSource = &AudioSource{
										Type:      "base64",
										MediaType: mediaType,
										Data:      parts[1],
									}
								}
							} else {
								// 外部URL
								cb.Type = "audio"
								cb.AudioSource = &AudioSource{
									Type:      "url",
									MediaType: "audio/url",
									Data:      urlStr,
								}
							}
						}
					}

					internalMsg.Content = append(internalMsg.Content, cb)
				}
			}
		}
	}

	// 处理 tool_calls (assistant 消息)
	if len(msg.ToolCalls) > 0 {
		for _, tc := range msg.ToolCalls {
			var input map[string]interface{}
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			}
			internalMsg.Content = append(internalMsg.Content, ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
	}

	// 处理 tool_call_id (tool 消息)
	if msg.ToolCallID != "" {
		content := ""
		if msg.Content != nil {
			if str, ok := msg.Content.(string); ok {
				content = str
			}
		}
		// tool 消息转换为 user 角色的 tool_result
		internalMsg.Role = "user"
		internalMsg.Content = []ContentBlock{
			{Type: "tool_result", ToolUseID: msg.ToolCallID, Content: content},
		}
	}

	return internalMsg
}

// BuildRequest 将内部格式构建为 OpenAI 请求
func (o *OpenAIConverter) BuildRequest(req *InternalRequest) ([]byte, error) {
	// 映射模型
	openAIModel := req.Model
	if o.cfg != nil {
		openAIModel = utils.MapClaudeModelToOpenAIWithConfig(req.Model, o.cfg)
	}

	openAIReq := models.OpenAIRequest{
		Model:  openAIModel,
		Stream: req.Stream,
		Stop:   req.StopSeqs,
	}

	// Support max_completion_tokens for o1/o3 models
	if req.MaxTokens > 0 {
		openAIReq.MaxTokens = req.MaxTokens
	}

	if req.Temperature != nil {
		openAIReq.Temperature = *req.Temperature
	}

	if req.TopP != nil {
		openAIReq.TopP = req.TopP
	}

	if req.ReasoningEffort != nil && *req.ReasoningEffort != "" {
		openAIReq.ReasoningEffort = *req.ReasoningEffort
	}

	// Add stream_options for usage in streaming
	if req.Stream {
		openAIReq.StreamOptions = &models.StreamOptions{
			IncludeUsage: true,
		}
	}

	// 构建 messages
	// 首先添加 system 消息（如果有）
	if req.System != "" {
		openAIReq.Messages = append(openAIReq.Messages, models.OpenAIMessage{
			Role:    "system",
			Content: req.System,
		})
	}

	for _, msg := range req.Messages {
		openAIMsg := o.convertInternalMessageToOpenAI(&msg)
		openAIReq.Messages = append(openAIReq.Messages, openAIMsg)
	}

	// 构建 tools
	for _, tool := range req.Tools {
		openAIReq.Tools = append(openAIReq.Tools, models.OpenAITool{
			Type: "function",
			Function: models.OpenAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	// 构建 tool_choice
	if req.ToolChoice != nil {
		openAIReq.ToolChoice = o.convertInternalToolChoiceToOpenAI(req.ToolChoice)
	}

	return json.Marshal(openAIReq)
}

// convertInternalMessageToOpenAI 将内部消息转换为 OpenAI 格式
func (o *OpenAIConverter) convertInternalMessageToOpenAI(msg *InternalMessage) models.OpenAIMessage {
	openAIMsg := models.OpenAIMessage{Role: msg.Role}

	// 处理 content blocks
	if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
		// 简单文本消息
		openAIMsg.Content = msg.Content[0].Text
	} else {
		// 复杂内容块或 tool_use
		toolCalls := []models.OpenAIToolCall{}
		hasToolUse := false
		var contentParts []map[string]interface{}
		hasMultiModal := false
		var simpleText string

		for _, cb := range msg.Content {
			switch cb.Type {
			case "text":
				if !hasToolUse && !hasMultiModal && simpleText == "" {
					// 如果还没有遇到 tool_use 或多模态，先保存简单text
					simpleText = cb.Text
				} else {
					// 已经有复杂内容，使用parts数组
					contentParts = append(contentParts, map[string]interface{}{
						"type": "text",
						"text": cb.Text,
					})
				}
			case "image":
				// 处理图片 - 转换为OpenAI的image_url格式
				if cb.Source != nil {
					// 首次遇到多模态，将之前保存的简单text转为part
					if !hasMultiModal && simpleText != "" {
						contentParts = append(contentParts, map[string]interface{}{
							"type": "text",
							"text": simpleText,
						})
						simpleText = ""
					}
					// 标记有多模态内容
					hasMultiModal = true

					if cb.Source.Type == "base64" {
						// 构建data URL
						dataURL := fmt.Sprintf("data:%s;base64,%s", cb.Source.MediaType, cb.Source.Data)
						contentParts = append(contentParts, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]string{
								"url": dataURL,
							},
						})
					} else if cb.Source.Type == "url" {
						// 外部URL
						contentParts = append(contentParts, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]string{
								"url": cb.Source.Data,
							},
						})
					}
				}
			case "tool_use":
				hasToolUse = true
				// 转换 tool call ID 格式: call_ -> fc_
				toolCallID := cb.ID
				if strings.HasPrefix(toolCallID, "call_") {
					toolCallID = "fc_" + toolCallID[5:]
				}

				args := ""
				if cb.Input != nil {
					argsBytes, _ := json.Marshal(cb.Input)
					args = string(argsBytes)
				}
				toolCalls = append(toolCalls, models.OpenAIToolCall{
					ID:   toolCallID,
					Type: "function",
					Function: models.OpenAIFunctionCall{
						Name:      cb.Name,
						Arguments: args,
					},
				})
			case "tool_result":
				// tool_result 应该作为单独的消息处理
				openAIMsg.ToolCallID = cb.ToolUseID
				openAIMsg.Content = cb.Content
				return openAIMsg
			}
		}

		if hasToolUse {
			openAIMsg.ToolCalls = toolCalls
			openAIMsg.Content = nil
		} else if hasMultiModal {
			// 多模态内容，使用parts数组
			openAIMsg.Content = contentParts
		} else if simpleText != "" {
			// 只有简单text，使用直接content
			openAIMsg.Content = simpleText
		}
	}

	return openAIMsg
}

// convertInternalToolChoiceToOpenAI 转换 tool_choice 格式
func (o *OpenAIConverter) convertInternalToolChoiceToOpenAI(choice interface{}) interface{} {
	if choice == nil {
		return nil
	}

	// 如果是字符串
	if str, ok := choice.(string); ok {
		return str
	}

	// 如果是 map
	if choiceMap, ok := choice.(map[string]interface{}); ok {
		choiceType, _ := choiceMap["type"].(string)

		switch choiceType {
		case "auto", "any":
			return "auto"
		case "tool":
			if name, ok := choiceMap["name"].(string); ok {
				return map[string]interface{}{
					"type": "function",
					"function": map[string]string{
						"name": name,
					},
				}
			}
		}
	}

	return "auto"
}

// ParseResponse 将 OpenAI 响应解析为内部格式
func (o *OpenAIConverter) ParseResponse(body []byte) (*InternalResponse, error) {
	var openAIResp models.OpenAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse openai response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openAIResp.Choices[0]
	message := choice.Message

	resp := &InternalResponse{
		ID:   openAIResp.ID,
		Role: message.Role,
		Usage: &UsageInfo{
			InputTokens:  openAIResp.Usage.PromptTokens,
			OutputTokens: openAIResp.Usage.CompletionTokens,
		},
	}

	// 映射 stop_reason
	switch choice.FinishReason {
	case "stop":
		resp.StopReason = "end_turn"
	case "length":
		resp.StopReason = "max_tokens"
	case "tool_calls", "function_call":
		resp.StopReason = "tool_use"
	default:
		resp.StopReason = "end_turn"
	}

	// 解析 content
	if message.Content != nil {
		if text, ok := message.Content.(string); ok && text != "" {
			resp.Content = append(resp.Content, ContentBlock{
				Type: "text",
				Text: text,
			})
		}
	}

	// 解析 tool_calls
	for _, tc := range message.ToolCalls {
		if tc.Type == "function" {
			var input map[string]interface{}
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			}

			// 转换 tool call ID 格式: fc_ -> call_
			toolCallID := tc.ID
			if strings.HasPrefix(toolCallID, "fc_") {
				toolCallID = "call_" + toolCallID[3:]
			}

			resp.Content = append(resp.Content, ContentBlock{
				Type:  "tool_use",
				ID:    toolCallID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
	}

	// 如果没有内容，添加空文本块
	if len(resp.Content) == 0 {
		resp.Content = append(resp.Content, ContentBlock{
			Type: "text",
			Text: "",
		})
	}

	// 解析缓存 token
	if openAIResp.Usage.PromptTokensDetails != nil {
		resp.Usage.CacheReadTokens = openAIResp.Usage.PromptTokensDetails.CachedTokens
	}

	return resp, nil
}

// BuildResponse 将内部格式构建为 OpenAI 响应
func (o *OpenAIConverter) BuildResponse(resp *InternalResponse) ([]byte, error) {
	message := models.OpenAIMessage{
		Role:    resp.Role,
		Content: "",
	}

	// 构建 content 和 tool_calls
	toolCalls := []models.OpenAIToolCall{}
	var textContent string

	for _, cb := range resp.Content {
		switch cb.Type {
		case "text":
			textContent += cb.Text
		case "tool_use":
			// 转换 tool call ID 格式: call_ -> fc_
			toolCallID := cb.ID
			if strings.HasPrefix(toolCallID, "call_") {
				toolCallID = "fc_" + toolCallID[5:]
			}

			args := ""
			if cb.Input != nil {
				argsBytes, _ := json.Marshal(cb.Input)
				args = string(argsBytes)
			}
			toolCalls = append(toolCalls, models.OpenAIToolCall{
				ID:   toolCallID,
				Type: "function",
				Function: models.OpenAIFunctionCall{
					Name:      cb.Name,
					Arguments: args,
				},
			})
		}
	}

	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
		message.Content = textContent
		if textContent == "" {
			message.Content = nil
		}
	} else {
		message.Content = textContent
	}

	// 映射 stop_reason
	finishReason := "stop"
	switch resp.StopReason {
	case "max_tokens":
		finishReason = "length"
	case "tool_use":
		finishReason = "tool_calls"
	}

	openAIResp := models.OpenAIResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: 0, // 由调用者设置
		Model:   "",
		Choices: []models.OpenAIChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: finishReason,
			},
		},
		Usage: models.OpenAIUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	return json.Marshal(openAIResp)
}

// ParseStreamEvent 解析 OpenAI 流式事件
func (o *OpenAIConverter) ParseStreamEvent(line []byte) (*StreamEvent, error) {
	// OpenAI SSE 格式: data: {...}
	lineStr := string(line)
	if !strings.HasPrefix(lineStr, "data: ") {
		return nil, fmt.Errorf("invalid sse format")
	}

	data := strings.TrimPrefix(lineStr, "data: ")
	if data == "[DONE]" {
		return &StreamEvent{Type: "done"}, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}

	event := &StreamEvent{}

	// 解析 choice
	choices, ok := raw["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return event, nil
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return event, nil
	}

	// 解析 delta
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return event, nil
	}

	// 解析 content
	if content, ok := delta["content"].(string); ok {
		event.Delta = &StreamDelta{
			Type: "text_delta",
			Text: content,
		}
	}

	// 解析 tool_calls - OpenAI tool_calls streaming is handled via forceNonStream mode
	// when format conversion is needed, so we don't need to handle partial_json here
	if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
		tc := toolCalls[0].(map[string]interface{})
		index, _ := tc["index"].(float64)

		if function, ok := tc["function"].(map[string]interface{}); ok {
			args, _ := function["arguments"].(string)
			// Store tool call arguments in event metadata for processing
			if args != "" {
				event.Delta = &StreamDelta{
					Type: "tool_call_delta",
					Text: args, // Use Text field to carry the arguments
				}
			}
			event.Index = int(index)
		}
	}

	// 解析 finish_reason
	if finishReason, ok := choice["finish_reason"].(string); ok {
		event.Delta.StopReason = finishReason
	}

	// 解析 usage
	if usage, ok := raw["usage"].(map[string]interface{}); ok {
		event.Usage = &UsageInfo{
			InputTokens:  int(usage["prompt_tokens"].(float64)),
			OutputTokens: int(usage["completion_tokens"].(float64)),
		}
	}

	return event, nil
}

// BuildStreamEvent implements FormatConverter.
// Converts a StreamEvent to OpenAI SSE format data payload.
func (o *OpenAIConverter) BuildStreamEvent(event *StreamEvent) ([]byte, error) {
	chunk := map[string]interface{}{
		"object": "chat.completion.chunk",
	}

	// Add message info if available
	if event.Message != nil {
		chunk["id"] = event.Message.ID
		chunk["model"] = event.Message.Model
	}

	choice := map[string]interface{}{
		"index": event.Index,
	}

	delta := map[string]interface{}{}

	if event.Delta != nil {
		// Handle text delta
		if event.Delta.Text != "" && event.Delta.Type != "tool_call_delta" {
			delta["content"] = event.Delta.Text
		}
		// Handle tool call delta - use Text field to carry arguments
		if event.Delta.Type == "tool_call_delta" && event.Delta.Text != "" {
			delta["tool_calls"] = []map[string]interface{}{
				{
					"index": event.Index,
					"function": map[string]interface{}{
						"arguments": event.Delta.Text,
					},
				},
			}
		}
		// Handle finish reason
		if event.Delta.StopReason != "" {
			finishReason := event.Delta.StopReason
			switch finishReason {
			case "end_turn":
				finishReason = "stop"
			case "max_tokens":
				finishReason = "length"
			case "tool_use":
				finishReason = "tool_calls"
			}
			choice["finish_reason"] = finishReason
		}
	}

	choice["delta"] = delta
	chunk["choices"] = []interface{}{choice}

	// Add usage if available
	if event.Usage != nil {
		chunk["usage"] = map[string]interface{}{
			"prompt_tokens":     event.Usage.InputTokens,
			"completion_tokens": event.Usage.OutputTokens,
			"total_tokens":      event.Usage.InputTokens + event.Usage.OutputTokens,
		}
	}

	return json.Marshal(chunk)
}
