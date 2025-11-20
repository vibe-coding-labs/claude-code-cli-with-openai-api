package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// GetMe 返回组织信息 - Claude Code 客户端需要此端点来验证身份
// 根据官方文档: https://docs.claude.com/en/api/admin-api/organization/get-me
func (h *Handler) GetMe(c *gin.Context) {
	logger := utils.GetLogger()
	logger.Info("→ [GetMe] Claude CLI requesting organization info")
	logger.Debug("  Path: %s", c.Request.URL.Path)
	logger.Debug("  Method: %s", c.Request.Method)
	logger.Debug("  Headers: %v", c.Request.Header)

	// 从context获取config ID（如果有）
	configID := c.Param("id")
	if configID == "" {
		configID = "default"
	}
	logger.Debug("  Config ID: %s", configID)

	// 返回 Organization 信息（符合 Anthropic Admin API 规范）
	response := gin.H{
		"id":   fmt.Sprintf("org_%s", configID),
		"name": "Proxy Organization",
		"type": "organization",
	}
	logger.Debug("  Response: %+v", response)
	c.JSON(http.StatusOK, response)

	logger.Info("← [GetMe] Successfully returned organization info")
}

// GetOrganizationUsage 返回组织使用情况 - Claude Code 客户端可能需要
func (h *Handler) GetOrganizationUsage(c *gin.Context) {
	logger := utils.GetLogger()
	logger.Info("→ [GetOrganizationUsage] Claude CLI requesting usage info")

	orgID := c.Param("org_id")
	logger.Debug("  Organization ID: %s", orgID)
	logger.Debug("  Path: %s", c.Request.URL.Path)

	// 返回基础使用信息
	response := gin.H{
		"object": "usage",
		"data": []gin.H{
			{
				"type":           "credit_balance",
				"credit_balance": 1000000, // 返回一个大的余额
			},
		},
	}
	logger.Debug("  Response: %+v", response)
	c.JSON(http.StatusOK, response)

	logger.Info("← [GetOrganizationUsage] Successfully returned usage info")
}

// GetModels 返回可用模型列表 - Claude Code 客户端可能需要
func (h *Handler) GetModels(c *gin.Context) {
	logger := utils.GetLogger()
	logger.Info("→ [GetModels] Claude CLI requesting model list")
	logger.Debug("  Path: %s", c.Request.URL.Path)

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

	response := gin.H{
		"object": "list",
		"data":   models,
	}
	logger.Debug("  Models count: %d", len(models))
	logger.Debug("  Response: %+v", response)
	c.JSON(http.StatusOK, response)

	logger.Info("← [GetModels] Successfully returned %d models", len(models))
}
