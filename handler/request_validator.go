package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// RequestValidator 处理请求验证和特殊请求处理
type RequestValidator struct{}

// NewRequestValidator 创建请求验证器
func NewRequestValidator() *RequestValidator {
	return &RequestValidator{}
}

// LogRequestDetails 记录请求详情
func (r *RequestValidator) LogRequestDetails(req *models.ClaudeMessagesRequest) {
	fmt.Printf("\n📥 [Request Details]\n")
	fmt.Printf("   Model: %s\n", req.Model)
	fmt.Printf("   MaxTokens: %d\n", req.MaxTokens)
	fmt.Printf("   Messages: %d\n", len(req.Messages))
	fmt.Printf("   Stream: %v\n", req.Stream)
	fmt.Printf("   Tools: %d\n", len(req.Tools))
	fmt.Printf("   TopK: %v\n", req.TopK)
	fmt.Printf("   ContextManagement: %v\n", req.ContextManagement)
	fmt.Printf("   Metadata: %v\n", req.Metadata)
	fmt.Printf("   Thinking: %v\n", req.Thinking)

	if len(req.Messages) > 0 {
		for i, msg := range req.Messages {
			contentStr := ""
			if str, ok := msg.Content.(string); ok {
				if len(str) > 50 {
					contentStr = str[:50] + "..."
				} else {
					contentStr = str
				}
			} else {
				contentStr = fmt.Sprintf("%T", msg.Content)
			}
			fmt.Printf("   Message[%d]: role=%s, content=%s\n", i, msg.Role, contentStr)
		}
	}
}

// IsConnectivityTest 检查是否为 Claude CLI 连接测试请求
func (r *RequestValidator) IsConnectivityTest(req *models.ClaudeMessagesRequest) bool {
	if req.MaxTokens != 1 || len(req.Messages) != 1 {
		return false
	}

	firstMsg := req.Messages[0]
	if firstMsg.Role != "user" {
		return false
	}

	if contentStr, ok := firstMsg.Content.(string); ok {
		return contentStr == "test" || contentStr == "quota"
	}

	return false
}

// HandleConnectivityTest 处理连接测试请求
func (r *RequestValidator) HandleConnectivityTest(c *gin.Context, req *models.ClaudeMessagesRequest) {
	fmt.Printf("✅ [RequestValidator] Returning connectivity test response\n")

	c.JSON(200, models.ClaudeResponse{
		ID:         "msg_test_" + fmt.Sprintf("%d", time.Now().Unix()),
		Type:       "message",
		Role:       models.RoleAssistant,
		Model:      req.Model,
		Content:    []models.ClaudeContentBlock{{Type: models.ContentText, Text: "OK"}},
		StopReason: models.StopEndTurn,
		Usage: models.ClaudeUsage{
			InputTokens:  1,
			OutputTokens: 1,
		},
	})
}

// ValidateRequest 验证请求参数
func (r *RequestValidator) ValidateRequest(req *models.ClaudeMessagesRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}

	if req.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be greater than 0")
	}

	if len(req.Messages) == 0 {
		return fmt.Errorf("messages cannot be empty")
	}

	return nil
}
