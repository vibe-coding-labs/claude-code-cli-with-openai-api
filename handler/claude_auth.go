package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GetMe 返回组织信息 - Claude Code 客户端需要此端点来验证身份
// 根据官方文档: https://docs.claude.com/en/api/admin-api/organization/get-me
func (h *Handler) GetMe(c *gin.Context) {
	fmt.Printf("🔵 [GetMe] Claude CLI 请求组织信息\n")

	// 从context获取config ID（如果有）
	configID := c.Param("id")
	if configID == "" {
		configID = "default"
	}

	// 返回 Organization 信息（符合 Anthropic Admin API 规范）
	c.JSON(http.StatusOK, gin.H{
		"id":   fmt.Sprintf("org_%s", configID),
		"name": "Proxy Organization",
		"type": "organization",
	})

	fmt.Printf("✅ [GetMe] 返回组织信息成功\n")
}

// GetOrganizationUsage 返回组织使用情况 - Claude Code 客户端可能需要
func (h *Handler) GetOrganizationUsage(c *gin.Context) {
	fmt.Printf("🔵 [GetOrganizationUsage] Claude CLI 请求使用情况\n")

	orgID := c.Param("org_id")
	fmt.Printf("   Organization ID: %s\n", orgID)

	// 返回基础使用信息
	c.JSON(http.StatusOK, gin.H{
		"object": "usage",
		"data": []gin.H{
			{
				"type":           "credit_balance",
				"credit_balance": 1000000, // 返回一个大的余额
			},
		},
	})

	fmt.Printf("✅ [GetOrganizationUsage] 返回使用情况成功\n")
}

// GetModels 返回可用模型列表 - Claude Code 客户端可能需要
func (h *Handler) GetModels(c *gin.Context) {
	fmt.Printf("🔵 [GetModels] Claude CLI 请求模型列表\n")

	// 返回 Claude 模型列表
	models := []gin.H{
		{
			"id":       "claude-3-5-sonnet-20241022",
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-3-opus-20240229",
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-3-sonnet-20240229",
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-3-haiku-20240307",
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "anthropic",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})

	fmt.Printf("✅ [GetModels] 返回模型列表成功，共 %d 个模型\n", len(models))
}
