package handler

import (
	"fmt"
	"strings"
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
			contentStr := formatContentForLogging(msg.Content)
			fmt.Printf("   Message[%d]: role=%s, content=%s\n", i, msg.Role, contentStr)
		}
	}
}

// formatContentForLogging formats message content for logging purposes
func formatContentForLogging(content interface{}) string {
	if content == nil {
		return "<nil>"
	}

	// Handle string content
	if str, ok := content.(string); ok {
		if len(str) > 50 {
			return str[:50] + "..."
		}
		return str
	}

	// Handle array of content blocks
	if blocks, ok := content.([]interface{}); ok {
		var parts []string
		for _, block := range blocks {
			if blockMap, ok := block.(map[string]interface{}); ok {
				blockType, _ := blockMap["type"].(string)
				switch blockType {
				case "text":
					if text, ok := blockMap["text"].(string); ok {
						if len(text) > 30 {
							parts = append(parts, text[:30]+"...")
						} else {
							parts = append(parts, text)
						}
					}
				case "tool_use":
					name, _ := blockMap["name"].(string)
					parts = append(parts, fmt.Sprintf("[tool_use: %s]", name))
				case "tool_result":
					toolUseID, _ := blockMap["tool_use_id"].(string)
					parts = append(parts, fmt.Sprintf("[tool_result: %s]", toolUseID))
				case "image":
					parts = append(parts, "[image]")
				default:
					parts = append(parts, fmt.Sprintf("[%s]", blockType))
				}
			}
		}
		return fmt.Sprintf("[%d blocks: %s]", len(blocks), strings.Join(parts, ", "))
	}

	// For other types, just show the type
	return fmt.Sprintf("<%T>", content)
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
