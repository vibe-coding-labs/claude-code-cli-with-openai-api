package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ResponseHandler 处理响应生成
type ResponseHandler struct{}

// NewResponseHandler 创建响应处理器
func NewResponseHandler() *ResponseHandler {
	return &ResponseHandler{}
}

// HandleStreamingResponse 处理流式响应
func (r *ResponseHandler) HandleStreamingResponse(
	c *gin.Context,
	targetClient *client.OpenAIClient,
	openAIReq *models.OpenAIRequest,
	claudeReq *models.ClaudeMessagesRequest,
) {
	fmt.Printf("\n🌊 [Streaming Mode]\n")
	fmt.Printf("   Initiating streaming request to upstream API\n")

	reader, err := targetClient.CreateChatCompletionStream(openAIReq)
	if err != nil {
		fmt.Printf("❌ [Streaming] Failed to create stream: %v\n", err)
		r.sendErrorResponse(c, err)
		return
	}
	defer reader.Close()

	fmt.Printf("✅ [Streaming] Stream created, starting conversion to Claude format\n")
	converter.ConvertOpenAIStreamingToClaude(c, reader, claudeReq, c.Request.Context())
	fmt.Printf("✅ [Streaming] Stream completed\n")
}

// HandleNonStreamingResponse 处理非流式响应
func (r *ResponseHandler) HandleNonStreamingResponse(
	c *gin.Context,
	targetClient *client.OpenAIClient,
	openAIReq *models.OpenAIRequest,
	claudeReq *models.ClaudeMessagesRequest,
) {
	fmt.Printf("\n📝 [Non-Streaming Mode]\n")
	fmt.Printf("   Sending non-streaming request to upstream API\n")

	openAIResp, err := targetClient.CreateChatCompletion(openAIReq)
	if err != nil {
		fmt.Printf("❌ [Non-Streaming] Request failed: %v\n", err)
		r.sendErrorResponse(c, err)
		return
	}

	fmt.Printf("✅ [Non-Streaming] Response received from upstream API\n")
	fmt.Printf("   Choices: %d\n", len(openAIResp.Choices))
	if len(openAIResp.Choices) > 0 {
		fmt.Printf("   First choice finish_reason: %s\n", openAIResp.Choices[0].FinishReason)
	}

	claudeResp := converter.ConvertOpenAIToClaudeResponse(openAIResp, claudeReq)
	fmt.Printf("✅ [Non-Streaming] Converted to Claude format, returning response\n")
	c.JSON(http.StatusOK, claudeResp)
	fmt.Printf("✅ [Non-Streaming] Response sent successfully\n")
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
