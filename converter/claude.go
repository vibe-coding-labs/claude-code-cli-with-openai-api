package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ClaudeConverter 处理 Claude API 格式与内部格式之间的转换
type ClaudeConverter struct{}

// NewClaudeConverter 创建新的 Claude 转换器
func NewClaudeConverter() *ClaudeConverter {
	return &ClaudeConverter{}
}

// ParseRequest 将 Claude 请求解析为内部格式
func (c *ClaudeConverter) ParseRequest(body []byte) (*InternalRequest, error) {
	var claudeReq models.ClaudeMessagesRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		return nil, fmt.Errorf("failed to parse claude request: %w", err)
	}

	req := &InternalRequest{
		Model:       claudeReq.Model,
		MaxTokens:   claudeReq.MaxTokens,
		Temperature: &claudeReq.Temperature,
		Stream:      claudeReq.Stream,
		StopSeqs:    claudeReq.StopSequences,
		Metadata:    make(map[string]interface{}),
	}

	// 提取 reasoning_effort 从 metadata
	if claudeReq.Metadata != nil && claudeReq.Metadata.SessionID != "" {
		req.Metadata["session_id"] = claudeReq.Metadata.SessionID
	}

	// 解析 system
	if claudeReq.System != nil {
		switch v := claudeReq.System.(type) {
		case string:
			req.System = v
		case []interface{}:
			var parts []string
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
			req.System = strings.Join(parts, "\n")
		}
	}

	// 解析 tools
	for _, tool := range claudeReq.Tools {
		req.Tools = append(req.Tools, ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}

	// 解析 tool_choice
	if claudeReq.ToolChoice != nil {
		req.ToolChoice = claudeReq.ToolChoice
	}

	// 解析 messages
	for _, msg := range claudeReq.Messages {
		internalMsg := InternalMessage{Role: msg.Role}

		switch content := msg.Content.(type) {
		case string:
			// 纯文本消息
			internalMsg.Content = []ContentBlock{
				{Type: "text", Text: content},
			}
		case []interface{}:
			// 复杂内容块数组
			for _, block := range content {
				blockBytes, _ := json.Marshal(block)
				var blockMap map[string]interface{}
				if err := json.Unmarshal(blockBytes, &blockMap); err != nil {
					continue
				}

				blockType, _ := blockMap["type"].(string)
				cb := ContentBlock{Type: blockType}

				switch blockType {
				case "text":
					cb.Text, _ = blockMap["text"].(string)
				case "image":
					// 处理图片 - 提取source
					cb.Type = "image"
					if source, ok := blockMap["source"].(map[string]interface{}); ok {
						sourceType, _ := source["type"].(string)
						mediaType, _ := source["media_type"].(string)
						data, _ := source["data"].(string)
						cb.Source = &ImageSource{
							Type:      sourceType,
							MediaType: mediaType,
							Data:      data,
						}
					}
				case "tool_use":
					cb.ID, _ = blockMap["id"].(string)
					cb.Name, _ = blockMap["name"].(string)
					if input, ok := blockMap["input"].(map[string]interface{}); ok {
						cb.Input = input
					}
				case "tool_result":
					cb.ToolUseID, _ = blockMap["tool_use_id"].(string)
					if content, ok := blockMap["content"].(string); ok {
						cb.Content = content
					}
				}

				internalMsg.Content = append(internalMsg.Content, cb)
			}
		}

		req.Messages = append(req.Messages, internalMsg)
	}

	return req, nil
}

// BuildRequest 将内部格式构建为 Claude 请求
func (c *ClaudeConverter) BuildRequest(req *InternalRequest) ([]byte, error) {
	claudeReq := models.ClaudeMessagesRequest{
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Stream:        req.Stream,
		StopSequences: req.StopSeqs,
	}

	if req.Temperature != nil {
		claudeReq.Temperature = *req.Temperature
	}

	if req.System != "" {
		claudeReq.System = req.System
	}

	// 构建 tools
	for _, tool := range req.Tools {
		claudeReq.Tools = append(claudeReq.Tools, models.ClaudeTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}

	if req.ToolChoice != nil {
		claudeReq.ToolChoice = req.ToolChoice.(map[string]interface{})
	}

	// 构建 messages
	for _, msg := range req.Messages {
		claudeMsg := models.ClaudeMessage{Role: msg.Role}

		if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			// 简单文本消息
			claudeMsg.Content = msg.Content[0].Text
		} else {
			// 复杂内容块
			var blocks []models.ClaudeContentBlock
			for _, cb := range msg.Content {
				block := models.ClaudeContentBlock{
					Type:      cb.Type,
					Text:      cb.Text,
					ID:        cb.ID,
					Name:      cb.Name,
					ToolUseID: cb.ToolUseID,
				}

				if cb.Input != nil {
					block.Input = cb.Input
				}
				if cb.Content != "" {
					block.Content = cb.Content
				}
				// 处理图片source
				if cb.Type == "image" && cb.Source != nil {
					block.Source = map[string]interface{}{
						"type":       cb.Source.Type,
						"media_type": cb.Source.MediaType,
						"data":       cb.Source.Data,
					}
				}

				blocks = append(blocks, block)
			}
			claudeMsg.Content = blocks
		}

		claudeReq.Messages = append(claudeReq.Messages, claudeMsg)
	}

	return json.Marshal(claudeReq)
}

// ParseResponse 将 Claude 响应解析为内部格式
func (c *ClaudeConverter) ParseResponse(body []byte) (*InternalResponse, error) {
	var claudeResp models.ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w", err)
	}

	resp := &InternalResponse{
		ID:         claudeResp.ID,
		Model:      claudeResp.Model,
		Role:       claudeResp.Role,
		StopReason: claudeResp.StopReason,
	}

	// 转换 content blocks
	for _, cb := range claudeResp.Content {
		resp.Content = append(resp.Content, ContentBlock{
			Type: cb.Type,
			Text: cb.Text,
			ID:   cb.ID,
			Name: cb.Name,
			Input: func() map[string]interface{} {
				if cb.Input != nil {
					return cb.Input
				}
				return nil
			}(),
		})
	}

	// 转换 usage
	resp.Usage = &UsageInfo{
		InputTokens:      claudeResp.Usage.InputTokens,
		OutputTokens:     claudeResp.Usage.OutputTokens,
		CacheReadTokens:  claudeResp.Usage.CacheReadInputTokens,
		CacheWriteTokens: claudeResp.Usage.CacheCreationInputTokens,
	}

	return resp, nil
}

// BuildResponse 将内部格式构建为 Claude 响应
func (c *ClaudeConverter) BuildResponse(resp *InternalResponse) ([]byte, error) {
	claudeResp := models.ClaudeResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       resp.Role,
		Model:      resp.Model,
		StopReason: resp.StopReason,
	}

	// 转换 content blocks
	for _, cb := range resp.Content {
		claudeResp.Content = append(claudeResp.Content, models.ClaudeContentBlock{
			Type:  cb.Type,
			Text:  cb.Text,
			ID:    cb.ID,
			Name:  cb.Name,
			Input: cb.Input,
		})
	}

	// 转换 usage
	if resp.Usage != nil {
		claudeResp.Usage = models.ClaudeUsage{
			InputTokens:              resp.Usage.InputTokens,
			OutputTokens:             resp.Usage.OutputTokens,
			CacheReadInputTokens:     resp.Usage.CacheReadTokens,
			CacheCreationInputTokens: resp.Usage.CacheWriteTokens,
		}
	}

	return json.Marshal(claudeResp)
}

// ParseStreamEvent 解析 Claude 流式事件
func (c *ClaudeConverter) ParseStreamEvent(line []byte) (*StreamEvent, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}

	eventType, _ := raw["type"].(string)
	event := &StreamEvent{Type: eventType}

	switch eventType {
	case "content_block_delta":
		if idx, ok := raw["index"].(float64); ok {
			event.Index = int(idx)
		}
		if delta, ok := raw["delta"].(map[string]interface{}); ok {
			event.Delta = &StreamDelta{}
			if t, ok := delta["type"].(string); ok {
				event.Delta.Type = t
			}
			if text, ok := delta["text"].(string); ok {
				event.Delta.Text = text
			}
			if partialJSON, ok := delta["partial_json"].(string); ok {
				event.Delta.Text = partialJSON
			}
		}
	case "message_delta":
		if delta, ok := raw["delta"].(map[string]interface{}); ok {
			event.Delta = &StreamDelta{}
			if stopReason, ok := delta["stop_reason"].(string); ok {
				event.Delta.StopReason = stopReason
			}
		}
		if usage, ok := raw["usage"].(map[string]interface{}); ok {
			event.Usage = &UsageInfo{}
			if outputTokens, ok := usage["output_tokens"].(float64); ok {
				event.Usage.OutputTokens = int(outputTokens)
			}
		}
	}

	return event, nil
}

// BuildStreamEvent implements FormatConverter.
// For Claude, we just marshal the StreamEvent directly since we use native format.
func (c *ClaudeConverter) BuildStreamEvent(event *StreamEvent) ([]byte, error) {
	return json.Marshal(event)
}
