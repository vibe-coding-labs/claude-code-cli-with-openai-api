package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	oldModels "github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// MessagesHandler Messages API处理器
type MessagesHandler struct {
	client *client.OpenAIClient
	config *config.Config
}

// NewMessagesHandler 创建新的Messages处理器
func NewMessagesHandler(cfg *config.Config) *MessagesHandler {
	return &MessagesHandler{
		client: client.NewOpenAIClient(cfg),
		config: cfg,
	}
}

// CreateMessage 创建消息
func (h *MessagesHandler) CreateMessage(c *gin.Context) {
	var req models.MessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "invalid_request_error",
				Message: fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	// 记录请求
	fmt.Printf("📨 [Messages API] Processing request\n")
	fmt.Printf("   Model: %s\n", req.Model)
	fmt.Printf("   Messages: %d\n", len(req.Messages))
	fmt.Printf("   Max tokens: %d\n", req.MaxTokens)
	fmt.Printf("   Stream: %v\n", req.Stream)

	// 转换为旧模型格式以复用现有转换逻辑
	oldReq := convertToOldFormat(&req)

	// 转换为OpenAI格式
	openAIReq := converter.ConvertClaudeToOpenAIWithConfig(oldReq, h.config)

	// 处理响应
	if req.Stream {
		h.handleStreamingResponse(c, openAIReq, &req)
	} else {
		h.handleNonStreamingResponse(c, openAIReq, &req)
	}
}

// CountTokens 计算tokens数量
func (h *MessagesHandler) CountTokens(c *gin.Context) {
	var req models.CountTokensRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "invalid_request_error",
				Message: fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	fmt.Printf("🔢 [Count Tokens] Processing request\n")
	fmt.Printf("   Model: %s\n", req.Model)
	fmt.Printf("   Messages: %d\n", len(req.Messages))

	// 简单的token估算（实际应该使用tiktoken或类似库）
	inputTokens := 0
	for _, msg := range req.Messages {
		switch content := msg.Content.(type) {
		case string:
			inputTokens += len(content) / 4 // 粗略估算：4个字符约等于1个token
		case []interface{}:
			for _, block := range content {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							inputTokens += len(textStr) / 4
						}
					}
				}
			}
		}
	}

	// 系统提示词的tokens
	if req.System != nil {
		switch sys := req.System.(type) {
		case string:
			inputTokens += len(sys) / 4
		}
	}

	// 工具定义的tokens
	for _, tool := range req.Tools {
		inputTokens += len(tool.Name) / 4
		inputTokens += len(tool.Description) / 4
		inputTokens += 50 // 估算schema的tokens
	}

	c.JSON(http.StatusOK, models.CountTokensResponse{
		InputTokens: inputTokens,
	})

	fmt.Printf("✅ [Count Tokens] Response: %d tokens\n", inputTokens)
}

// 辅助函数：转换新格式到旧格式
func convertToOldFormat(req *models.MessagesRequest) *oldModels.ClaudeMessagesRequest {
	oldMessages := make([]oldModels.ClaudeMessage, len(req.Messages))
	for i, msg := range req.Messages {
		oldMessages[i] = oldModels.ClaudeMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	oldTools := make([]oldModels.ClaudeTool, len(req.Tools))
	for i, tool := range req.Tools {
		oldTools[i] = oldModels.ClaudeTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
	}

	var oldThinking *oldModels.ClaudeThinking
	if req.Thinking != nil {
		oldThinking = &oldModels.ClaudeThinking{
			Type:         req.Thinking.Type,
			BudgetTokens: req.Thinking.BudgetTokens,
		}
	}

	var oldMetadata *oldModels.ClaudeMetadata
	if req.Metadata != nil {
		oldMetadata = &oldModels.ClaudeMetadata{
			UserID: req.Metadata.UserID,
		}
	}

	// 处理ToolChoice类型
	var toolChoice map[string]interface{}
	if req.ToolChoice != nil {
		if tc, ok := req.ToolChoice.(map[string]interface{}); ok {
			toolChoice = tc
		}
	}

	return &oldModels.ClaudeMessagesRequest{
		Model:                  req.Model,
		Messages:               oldMessages,
		System:                 req.System,
		MaxTokens:              req.MaxTokens,
		Temperature:            req.Temperature,
		TopP:                   req.TopP,
		TopK:                   req.TopK,
		Stream:                 req.Stream,
		StopSequences:          req.StopSequences,
		Metadata:               oldMetadata,
		Tools:                  oldTools,
		ToolChoice:             toolChoice,
		DisableParallelToolUse: req.DisableParallelToolUse,
		ContextManagement:      req.ContextManagement,
		Thinking:               oldThinking,
	}
}

// handleNonStreamingResponse 处理非流式响应
func (h *MessagesHandler) handleNonStreamingResponse(c *gin.Context, openAIReq *oldModels.OpenAIRequest, claudeReq *models.MessagesRequest) {
	// 调用OpenAI
	openAIResp, err := h.client.CreateChatCompletion(openAIReq)
	if err != nil {
		fmt.Printf("❌ OpenAI API error: %v\n", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "api_error",
				Message: fmt.Sprintf("Failed to process request: %v", err),
			},
		})
		return
	}

	// 转换响应
	oldReq := convertToOldFormat(claudeReq)
	claudeResp := converter.ConvertOpenAIToClaudeResponse(openAIResp, oldReq)

	// 转换为新格式
	newResp := convertResponseToNewFormat(claudeResp)
	c.JSON(http.StatusOK, newResp)
}

// handleStreamingResponse 处理流式响应
func (h *MessagesHandler) handleStreamingResponse(c *gin.Context, openAIReq *oldModels.OpenAIRequest, claudeReq *models.MessagesRequest) {
	// 调用OpenAI流式API
	stream, err := h.client.CreateChatCompletionStream(openAIReq)
	if err != nil {
		fmt.Printf("❌ OpenAI Stream API error: %v\n", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "api_error",
				Message: fmt.Sprintf("Failed to create stream: %v", err),
			},
		})
		return
	}
	defer stream.Close()

	// 使用现有的流式转换器
	oldReq := convertToOldFormat(claudeReq)
	converter.ConvertOpenAIStreamingToClaude(c, stream, oldReq, c.Request.Context())
}

// convertResponseToNewFormat 转换响应到新格式
func convertResponseToNewFormat(oldResp *oldModels.ClaudeResponse) *models.MessagesResponse {
	contentBlocks := make([]models.ContentBlock, len(oldResp.Content))
	for i, block := range oldResp.Content {
		contentBlocks[i] = models.ContentBlock{
			Type:      block.Type,
			Text:      block.Text,
			Source:    block.Source,
			ID:        block.ID,
			Name:      block.Name,
			Input:     block.Input,
			ToolUseID: block.ToolUseID,
			Content:   block.Content,
			IsError:   block.IsError,
		}
	}

	return &models.MessagesResponse{
		ID:           oldResp.ID,
		Type:         oldResp.Type,
		Role:         oldResp.Role,
		Model:        oldResp.Model,
		Content:      contentBlocks,
		StopReason:   oldResp.StopReason,
		StopSequence: oldResp.StopSequence,
		Usage: models.Usage{
			InputTokens:              oldResp.Usage.InputTokens,
			OutputTokens:             oldResp.Usage.OutputTokens,
			CacheCreationInputTokens: oldResp.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     oldResp.Usage.CacheReadInputTokens,
		},
	}
}
