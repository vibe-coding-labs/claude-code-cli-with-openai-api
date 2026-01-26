package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// ResponseHandler 处理响应生成
type ResponseHandler struct{}

// NewResponseHandler 创建响应处理器
func NewResponseHandler() *ResponseHandler {
	return &ResponseHandler{}
}

func getUserIDFromContext(c *gin.Context) int64 {
	value, ok := c.Get("user_id")
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// HandleStreamingResponse 处理流式响应
func (r *ResponseHandler) HandleStreamingResponse(
	c *gin.Context,
	targetClient *client.OpenAIClient,
	openAIReq *models.OpenAIRequest,
	claudeReq *models.ClaudeMessagesRequest,
	configID string,
	startTime time.Time,
) {
	fmt.Printf("\n🌊 [Streaming Mode]\n")
	fmt.Printf("   Initiating streaming request to upstream API\n")

	reader, err := targetClient.CreateChatCompletionStream(openAIReq)
	if err != nil {
		fmt.Printf("❌ [Streaming] Failed to create stream: %v\n", err)
		r.sendErrorResponse(c, err)
		r.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
		return
	}
	defer reader.Close()

	fmt.Printf("✅ [Streaming] Stream created, starting conversion to Claude format\n")
	streamResult := converter.ConvertOpenAIStreamingToClaude(c, reader, claudeReq, c.Request.Context())
	fmt.Printf("✅ [Streaming] Stream completed\n")

	// 记录请求日志（使用收集的流式响应数据）
	if streamResult != nil {
		r.logRequestWithStreamingDetails(c, configID, openAIReq.Model, streamResult, startTime, "success", "", claudeReq)
	} else {
		// 如果streamResult为nil，说明发生了错误，记录基本信息
		r.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", "Streaming failed", claudeReq, nil)
	}
}

// HandleNonStreamingResponse 处理非流式响应
func (r *ResponseHandler) HandleNonStreamingResponse(
	c *gin.Context,
	targetClient *client.OpenAIClient,
	openAIReq *models.OpenAIRequest,
	claudeReq *models.ClaudeMessagesRequest,
	configID string,
	startTime time.Time,
) {
	fmt.Printf("\n📝 [Non-Streaming Mode]\n")
	fmt.Printf("   Sending non-streaming request to upstream API\n")

	openAIResp, err := targetClient.CreateChatCompletion(openAIReq)
	if err != nil {
		fmt.Printf("❌ [Non-Streaming] Request failed: %v\n", err)
		r.sendErrorResponse(c, err)
		r.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
		return
	}

	fmt.Printf("✅ [Non-Streaming] Response received from upstream API\n")
	fmt.Printf("   Choices: %d\n", len(openAIResp.Choices))
	if len(openAIResp.Choices) > 0 {
		fmt.Printf("   First choice finish_reason: %s\n", openAIResp.Choices[0].FinishReason)
	}

	claudeResp := converter.ConvertOpenAIToClaudeResponse(openAIResp, claudeReq)
	if claudeResp == nil {
		fmt.Printf("❌ [Non-Streaming] Failed to convert response - response is nil\n")
		err := fmt.Errorf("failed to convert OpenAI response to Claude format: response or choices is empty")
		r.sendErrorResponse(c, err)
		r.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
		return
	}

	fmt.Printf("✅ [Non-Streaming] Converted to Claude format, returning response\n")

	// 记录请求日志（含详细请求和响应）
	r.logRequestWithDetails(c, configID, openAIReq.Model,
		claudeResp.Usage.InputTokens,
		claudeResp.Usage.OutputTokens,
		startTime, "success", "", claudeReq, claudeResp)

	c.JSON(http.StatusOK, claudeResp)
	fmt.Printf("✅ [Non-Streaming] Response sent successfully\n")
}

// logRequestWithStreamingDetails 记录流式请求日志到数据库
func (r *ResponseHandler) logRequestWithStreamingDetails(
	c *gin.Context,
	configID string,
	model string,
	streamResult *converter.StreamingResult,
	startTime time.Time,
	status string,
	errorMsg string,
	claudeReq *models.ClaudeMessagesRequest,
) {
	// 如果 configID 为空，使用 "default" 作为标识
	if configID == "" {
		configID = "default"
	}

	duration := time.Since(startTime)

	// 序列化请求体
	requestBody := ""
	if claudeReq != nil {
		if reqJSON, err := json.Marshal(claudeReq); err == nil {
			requestBody = string(reqJSON)
		}
	}

	// 生成响应预览（使用收集的内容）
	responsePreview := streamResult.Content
	if len(responsePreview) > 500 {
		responsePreview = responsePreview[:500] + "..."
	}

	// 生成请求摘要
	requestSummary := ""
	if claudeReq != nil {
		if len(claudeReq.Messages) > 0 {
			lastMsg := claudeReq.Messages[len(claudeReq.Messages)-1]
			if lastMsg.Role == "user" {
				// 尝试提取文本内容
				if contentStr, ok := lastMsg.Content.(string); ok {
					if len(contentStr) > 200 {
						requestSummary = contentStr[:200] + "..."
					} else {
						requestSummary = contentStr
					}
				} else if contentArr, ok := lastMsg.Content.([]interface{}); ok && len(contentArr) > 0 {
					// 处理数组形式的content
					if contentMap, ok := contentArr[0].(map[string]interface{}); ok {
						if text, ok := contentMap["text"].(string); ok {
							if len(text) > 200 {
								requestSummary = text[:200] + "..."
							} else {
								requestSummary = text
							}
						}
					}
				}
			}
		}
	}

	// 获取客户端信息
	clientIP, userAgent := GetClientInfo(c)
	userID := getUserIDFromContext(c)

	log := &database.RequestLog{
		ConfigID:        configID,
		UserID:          userID,
		Model:           model,
		InputTokens:     streamResult.InputTokens,
		OutputTokens:    streamResult.OutputTokens,
		TotalTokens:     streamResult.InputTokens + streamResult.OutputTokens,
		DurationMs:      int(duration.Milliseconds()),
		Status:          status,
		ErrorMessage:    errorMsg,
		RequestBody:     requestBody,
		ResponseBody:    streamResult.Content, // 存储完整的响应内容
		RequestSummary:  requestSummary,
		ResponsePreview: responsePreview,
		ClientIP:        clientIP,
		UserAgent:       userAgent,
	}

	if err := database.LogRequest(log); err != nil {
		logger := utils.GetLogger()
		logger.Error("Failed to log streaming request: %v", err)
	}
}

// logRequestWithDetails 记录请求日志到数据库（含详细请求和响应）
func (r *ResponseHandler) logRequestWithDetails(
	c *gin.Context,
	configID string,
	model string,
	inputTokens int,
	outputTokens int,
	startTime time.Time,
	status string,
	errorMsg string,
	claudeReq *models.ClaudeMessagesRequest,
	claudeResp *models.ClaudeResponse,
) {
	// 如果 configID 为空，使用 "default" 作为标识
	if configID == "" {
		configID = "default"
	}

	duration := time.Since(startTime)

	// 序列化请求体
	requestBody := ""
	if claudeReq != nil {
		if reqJSON, err := json.Marshal(claudeReq); err == nil {
			requestBody = string(reqJSON)
		}
	}

	// 序列化响应体
	responseBody := ""
	responsePreview := ""
	if claudeResp != nil {
		if respJSON, err := json.Marshal(claudeResp); err == nil {
			responseBody = string(respJSON)
			// 生成响应预览（前500字符）
			if len(responseBody) > 500 {
				responsePreview = responseBody[:500] + "..."
			} else {
				responsePreview = responseBody
			}
		}
	}

	// 生成请求摘要
	requestSummary := ""
	if claudeReq != nil {
		if len(claudeReq.Messages) > 0 {
			lastMsg := claudeReq.Messages[len(claudeReq.Messages)-1]
			if lastMsg.Role == "user" {
				// 尝试提取文本内容
				if contentStr, ok := lastMsg.Content.(string); ok {
					if len(contentStr) > 200 {
						requestSummary = contentStr[:200] + "..."
					} else {
						requestSummary = contentStr
					}
				} else if contentArr, ok := lastMsg.Content.([]interface{}); ok && len(contentArr) > 0 {
					// 处理数组形式的content
					if contentMap, ok := contentArr[0].(map[string]interface{}); ok {
						if text, ok := contentMap["text"].(string); ok {
							if len(text) > 200 {
								requestSummary = text[:200] + "..."
							} else {
								requestSummary = text
							}
						}
					}
				}
			}
		}
	}

	// 获取客户端信息
	clientIP, userAgent := GetClientInfo(c)
	userID := getUserIDFromContext(c)

	log := &database.RequestLog{
		ConfigID:        configID,
		UserID:          userID,
		Model:           model,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     inputTokens + outputTokens,
		DurationMs:      int(duration.Milliseconds()),
		Status:          status,
		ErrorMessage:    errorMsg,
		ClientIP:        clientIP,
		UserAgent:       userAgent,
		RequestBody:     requestBody,
		ResponseBody:    responseBody,
		RequestSummary:  requestSummary,
		ResponsePreview: responsePreview,
	}

	if err := database.LogRequest(log); err != nil {
		logger := utils.GetLogger()
		logger.Error("Failed to log request: %v", err)
	}
}

// sendErrorResponse 发送错误响应
func (r *ResponseHandler) sendErrorResponse(c *gin.Context, err error) {
	errorMsg := err.Error()
	statusCode := http.StatusInternalServerError

	// 尝试从错误消息中提取状态码
	// 错误格式: "OpenAI API error (status 401): ..."
	if strings.Contains(errorMsg, "status") {
		var extractedStatus int
		if n, _ := fmt.Sscanf(errorMsg, "OpenAI API error (status %d):", &extractedStatus); n == 1 {
			if extractedStatus >= 400 && extractedStatus < 600 {
				statusCode = extractedStatus
			}
		}
	}

	// 提取实际的错误消息（状态码之后的部分）
	// 格式: "OpenAI API error (status 401): actual error message"
	if idx := strings.Index(errorMsg, "): "); idx > 0 {
		errorMsg = errorMsg[idx+3:]
	}

	// 分类并格式化错误消息
	classifiedError := client.ClassifyOpenAIError(errorMsg)

	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": classifiedError,
		},
	})
}
