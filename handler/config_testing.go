package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestConfig tests a configuration by making a test request
func (h *ConfigHandler) TestConfig(c *gin.Context) {
	id := c.Param("id")

	cfg, err := h.manager.GetConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found: " + err.Error(),
		})
		return
	}

	// Create a test config
	testConfig := &config.Config{
		OpenAIBaseURL:   cfg.OpenAIBaseURL,
		OpenAIAPIKey:    cfg.OpenAIAPIKey,
		BigModel:        cfg.BigModel,
		MiddleModel:     cfg.MiddleModel,
		SmallModel:      cfg.SmallModel,
		MaxTokensLimit:  cfg.MaxTokensLimit,
		RequestTimeout:  cfg.RequestTimeout,
		AnthropicAPIKey: cfg.AnthropicAPIKey,
	}

	// Create a test client
	testClient := client.NewOpenAIClient(testConfig)

	// Make a simple test request
	testReq := &models.OpenAIRequest{
		Model: testConfig.SmallModel,
		Messages: []models.OpenAIMessage{
			{
				Role:    models.RoleUser,
				Content: "Hello",
			},
		},
		MaxTokens:   10,
		Temperature: 0.1,
	}

	startTime := time.Now()
	resp, err := testClient.CreateChatCompletion(testReq)
	duration := time.Since(startTime)

	if err != nil {
		// Test failed
		h.manager.UpdateTestStatus(id, "failed", err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":     false,
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
		})
		return
	}

	// Test successful
	h.manager.UpdateTestStatus(id, "success", "")
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"duration_ms": duration.Milliseconds(),
		"response": gin.H{
			"model":  resp.Model,
			"tokens": resp.Usage.TotalTokens,
		},
	})
}
