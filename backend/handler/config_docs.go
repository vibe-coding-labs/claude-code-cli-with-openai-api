package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetClaudeConfig returns the Claude-specific configuration endpoint
func (h *ConfigHandler) GetClaudeConfig(c *gin.Context) {
	// Get API key from header
	apiKey := c.GetHeader("x-api-key")
	if apiKey == "" {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			apiKey = authHeader[7:]
		}
	}

	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "API key required",
		})
		return
	}

	// Find config by API key
	cfg, err := h.manager.GetConfigByAnthropicKey(apiKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found for the provided API key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":              cfg.ID,
		"name":            cfg.Name,
		"openai_base_url": cfg.OpenAIBaseURL,
		"models": gin.H{
			"big":    cfg.BigModel,
			"middle": cfg.MiddleModel,
			"small":  cfg.SmallModel,
		},
	})
}

// GetAPIDocs returns the API documentation
func (h *ConfigHandler) GetAPIDocs(c *gin.Context) {
	docs := gin.H{
		"version": "1.0.0",
		"endpoints": []gin.H{
			{
				"path":        "/v1/messages",
				"method":      "POST",
				"description": "Create a message (Claude Messages API compatible)",
			},
			{
				"path":        "/v1/configs",
				"method":      "GET",
				"description": "List all API configurations",
			},
			{
				"path":        "/v1/configs",
				"method":      "POST",
				"description": "Create a new API configuration",
			},
			{
				"path":        "/v1/configs/:id",
				"method":      "GET",
				"description": "Get a specific configuration",
			},
			{
				"path":        "/v1/configs/:id",
				"method":      "PUT",
				"description": "Update a configuration",
			},
			{
				"path":        "/v1/configs/:id",
				"method":      "DELETE",
				"description": "Delete a configuration",
			},
			{
				"path":        "/v1/configs/:id/default",
				"method":      "POST",
				"description": "Set as default configuration",
			},
			{
				"path":        "/v1/configs/:id/test",
				"method":      "POST",
				"description": "Test a configuration",
			},
			{
				"path":        "/health",
				"method":      "GET",
				"description": "Health check endpoint",
			},
		},
	}

	c.JSON(http.StatusOK, docs)
}
