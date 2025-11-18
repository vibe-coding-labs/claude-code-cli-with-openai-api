package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ConfigHandler handles configuration management API endpoints
type ConfigHandler struct {
	manager *config.ConfigManager
}

// NewConfigHandler creates a new ConfigHandler
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{
		manager: config.GetConfigManager(),
	}
}

// ListConfigs returns all configurations
func (h *ConfigHandler) ListConfigs(c *gin.Context) {
	configs := h.manager.GetAllConfigs()
	
	// Remove sensitive information for list view
	safeConfigs := make([]models.APIConfig, 0, len(configs))
	for _, cfg := range configs {
		safeCfg := *cfg
		// Mask API keys
		if len(safeCfg.OpenAIAPIKey) > 8 {
			safeCfg.OpenAIAPIKey = safeCfg.OpenAIAPIKey[:4] + "..." + safeCfg.OpenAIAPIKey[len(safeCfg.OpenAIAPIKey)-4:]
		}
		if len(safeCfg.AnthropicAPIKey) > 8 {
			safeCfg.AnthropicAPIKey = safeCfg.AnthropicAPIKey[:4] + "..." + safeCfg.AnthropicAPIKey[len(safeCfg.AnthropicAPIKey)-4:]
		}
		safeConfigs = append(safeConfigs, safeCfg)
	}

	defaultID, _ := h.manager.GetDefaultConfig()
	defaultIDStr := ""
	if defaultID != nil {
		defaultIDStr = defaultID.ID
	}

	c.JSON(http.StatusOK, gin.H{
		"configs":         safeConfigs,
		"default_config_id": defaultIDStr,
	})
}

// GetConfig returns a specific configuration
func (h *ConfigHandler) GetConfig(c *gin.Context) {
	id := c.Param("id")
	cfg, err := h.manager.GetConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// CreateConfig creates a new configuration
func (h *ConfigHandler) CreateConfig(c *gin.Context) {
	var req models.APIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	cfg, err := h.manager.CreateConfig(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": map[string]interface{}{
				"type":    "internal_error",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, cfg)
}

// UpdateConfig updates an existing configuration
func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	id := c.Param("id")
	var req models.APIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	cfg, err := h.manager.UpdateConfig(id, &req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// DeleteConfig deletes a configuration
func (h *ConfigHandler) DeleteConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.DeleteConfig(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Config deleted successfully",
	})
}

// SetDefaultConfig sets the default configuration
func (h *ConfigHandler) SetDefaultConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.SetDefaultConfig(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Default config set successfully",
	})
}

// TestConfig tests a configuration by making a test API call
func (h *ConfigHandler) TestConfig(c *gin.Context) {
	id := c.Param("id")
	cfg, err := h.manager.GetConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": err.Error(),
			},
		})
		return
	}

	// Create a temporary config for testing
	tempConfig := config.ToConfigFromAPIConfig(cfg)
	testClient := client.NewOpenAIClient(tempConfig)

	// Create test request
	testReq := &models.OpenAIRequest{
		Model: cfg.SmallModel,
		Messages: []models.OpenAIMessage{
			{
				Role:    models.RoleUser,
				Content: "Hello",
			},
		},
		MaxTokens: 5,
	}

	// Test the connection
	resp, err := testClient.CreateChatCompletion(testReq)
	now := time.Now()

	if err != nil {
		errorMsg := err.Error()
		h.manager.UpdateTestStatus(id, "failed", errorMsg)
		c.JSON(http.StatusServiceUnavailable, models.TestConfigResponse{
			Status:    "failed",
			Message:   "Connection test failed",
			Error:     errorMsg,
			ErrorType: "API Error",
			Timestamp: now,
			Suggestions: []string{
				"Check your OpenAI API key is valid",
				"Verify your API key has the necessary permissions",
				"Check if you have reached rate limits",
				"Verify the base URL is correct",
			},
		})
		return
	}

	h.manager.UpdateTestStatus(id, "success", "")
	c.JSON(http.StatusOK, models.TestConfigResponse{
		Status:     "success",
		Message:    "Successfully connected to OpenAI API",
		ModelUsed:  cfg.SmallModel,
		ResponseID: resp.ID,
		Timestamp:  now,
	})
}

// GetClaudeConfig returns the configuration in Claude Code CLI format
func (h *ConfigHandler) GetClaudeConfig(c *gin.Context) {
	id := c.Param("id")
	cfg, err := h.manager.GetConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": err.Error(),
			},
		})
		return
	}

	// Get server address from query or use default
	serverAddr := c.Query("server")
	if serverAddr == "" {
		// Try to get from request host
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		host := c.Request.Host
		if host == "" {
			host = "localhost:10086"
		}
		serverAddr = fmt.Sprintf("%s://%s", scheme, host)
	}

	claudeConfig := models.ClaudeConfigFormat{
		ANTHROPIC_BASE_URL: fmt.Sprintf("%s/v1/configs/%s", serverAddr, id),
		ANTHROPIC_API_KEY:  cfg.AnthropicAPIKey,
		ConfigID:           cfg.ID,
		ConfigName:         cfg.Name,
	}

	c.JSON(http.StatusOK, claudeConfig)
}

// GetAPIDocs returns API documentation
func (h *ConfigHandler) GetAPIDocs(c *gin.Context) {
	docs := gin.H{
		"title":       "Claude-to-OpenAI API Proxy - API Documentation",
		"version":     "1.0.0",
		"description": "API documentation for the Claude-to-OpenAI proxy service",
		"endpoints": gin.H{
			"claude_api": gin.H{
				"description": "Claude API compatible endpoints",
				"endpoints": []gin.H{
					{
						"method":      "POST",
						"path":        "/v1/messages",
						"description": "Create a chat completion (Claude format)",
						"auth":        "Required: x-api-key or Authorization header",
					},
					{
						"method":      "POST",
						"path":        "/v1/messages/count_tokens",
						"description": "Count tokens in a message",
						"auth":        "Required: x-api-key or Authorization header",
					},
					{
						"method":      "POST",
						"path":        "/v1/configs/:id/messages",
						"description": "Create a chat completion using a specific config (Claude format)",
						"auth":        "Required: x-api-key or Authorization header",
					},
				},
			},
			"config_management": gin.H{
				"description": "Configuration management endpoints",
				"endpoints": []gin.H{
					{
						"method":      "GET",
						"path":        "/api/configs",
						"description": "List all configurations",
					},
					{
						"method":      "GET",
						"path":        "/api/configs/:id",
						"description": "Get a specific configuration",
					},
					{
						"method":      "POST",
						"path":        "/api/configs",
						"description": "Create a new configuration",
					},
					{
						"method":      "PUT",
						"path":        "/api/configs/:id",
						"description": "Update a configuration",
					},
					{
						"method":      "DELETE",
						"path":        "/api/configs/:id",
						"description": "Delete a configuration",
					},
					{
						"method":      "POST",
						"path":        "/api/configs/:id/test",
						"description": "Test a configuration",
					},
					{
						"method":      "POST",
						"path":        "/api/configs/:id/set-default",
						"description": "Set a configuration as default",
					},
					{
						"method":      "GET",
						"path":        "/api/configs/:id/claude-config",
						"description": "Get Claude Code CLI format configuration",
					},
				},
			},
			"utility": gin.H{
				"description": "Utility endpoints",
				"endpoints": []gin.H{
					{
						"method":      "GET",
						"path":        "/health",
						"description": "Health check endpoint",
					},
					{
						"method":      "GET",
						"path":        "/test-connection",
						"description": "Test connection to OpenAI API",
					},
					{
						"method":      "GET",
						"path":        "/api/docs",
						"description": "Get API documentation",
					},
				},
			},
		},
		"usage": gin.H{
			"claude_code_cli": "ANTHROPIC_BASE_URL=http://localhost:10086 ANTHROPIC_API_KEY='your-key' claude",
			"multiple_configs": "ANTHROPIC_BASE_URL=http://localhost:10086/v1/configs/{config-id} ANTHROPIC_API_KEY='your-key' claude",
		},
	}

	c.JSON(http.StatusOK, docs)
}

