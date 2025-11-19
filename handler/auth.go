package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// AuthHandler 处理认证相关逻辑
type AuthHandler struct{}

// NewAuthHandler 创建认证处理器
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// ValidateAPIKey 验证 API Key
// 返回: (通过验证, 错误响应)
func (a *AuthHandler) ValidateAPIKey(c *gin.Context, dbConfig *database.APIConfig) (bool, *gin.H) {
	// 如果配置没有设置 API Key，则不需要验证
	if dbConfig.AnthropicAPIKey == "" {
		fmt.Printf("   ℹ️ No API key validation required for this config\n")
		return true, nil
	}

	// 从请求中获取 API Key
	clientAPIKey := a.extractAPIKey(c)

	if clientAPIKey == "" {
		fmt.Printf("❌ [Auth] No API key provided in request\n")
		return false, &gin.H{
			"error": map[string]interface{}{
				"type":    "authentication_error",
				"message": "Missing API key. Please provide an Anthropic API key in the x-api-key header or Authorization header.",
			},
		}
	}

	fmt.Printf("   🔑 Client API Key: %s...\n", clientAPIKey[:min(len(clientAPIKey), 20)])
	fmt.Printf("   🔐 Expected API Key: %s...\n", dbConfig.AnthropicAPIKey[:min(len(dbConfig.AnthropicAPIKey), 20)])

	// 验证 API Key
	if clientAPIKey != dbConfig.AnthropicAPIKey {
		fmt.Printf("❌ [Auth] API key mismatch\n")
		return false, &gin.H{
			"error": map[string]interface{}{
				"type":    "authentication_error",
				"message": "Invalid API key. Please provide a valid Anthropic API key.",
			},
		}
	}

	fmt.Printf("✅ [Auth] API key validated successfully\n")
	return true, nil
}

// extractAPIKey 从请求中提取 API Key
func (a *AuthHandler) extractAPIKey(c *gin.Context) string {
	// 优先从 x-api-key header 获取
	apiKey := c.GetHeader("x-api-key")
	if apiKey != "" {
		return apiKey
	}

	// 从 Authorization header 获取
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SendAuthError 发送认证错误响应
func (a *AuthHandler) SendAuthError(c *gin.Context, statusCode int, errorResp *gin.H) {
	c.JSON(statusCode, errorResp)
}
