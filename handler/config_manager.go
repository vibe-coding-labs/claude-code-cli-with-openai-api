package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// GetAllConfigs returns all API configurations
func (h *Handler) GetAllConfigs(c *gin.Context) {
	userID, role := getUserContext(c)
	var (
		configs []*database.APIConfig
		err     error
	)
	if isAdminRole(role) {
		configs, err = database.GetAllAPIConfigs()
	} else {
		configs, err = database.GetAPIConfigsByUser(userID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get configs: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configs": configs,
	})
}

// GetConfig returns a specific API configuration
func (h *Handler) GetConfig(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Config ID is required",
		})
		return
	}

	config, err := database.GetAPIConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Config not found: %v", err),
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"config": config,
	})
}

// CreateConfig creates a new API configuration
func (h *Handler) CreateConfig(c *gin.Context) {
	var config database.APIConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	userID, _ := getUserContext(c)

	// Validate required fields
	if config.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Name is required",
		})
		return
	}

	if config.OpenAIAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "OpenAI API Key is required",
		})
		return
	}

	if config.OpenAIBaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "OpenAI Base URL is required",
		})
		return
	}

	// Set defaults
	if config.MaxTokensLimit == 0 {
		config.MaxTokensLimit = 4096
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 90
	}
	if config.BigModel == "" {
		config.BigModel = "gpt-4o"
	}
	if config.MiddleModel == "" {
		config.MiddleModel = "gpt-4o"
	}
	if config.SmallModel == "" {
		config.SmallModel = "gpt-4o-mini"
	}
	// Enable by default
	config.Enabled = true
	config.UserID = userID

	// Create config
	if err := database.CreateAPIConfig(&config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create config: %v", err),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Config created successfully",
		"id":      config.ID,
	})
}

// UpdateConfig updates an existing API configuration
func (h *Handler) UpdateConfig(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Config ID is required",
		})
		return
	}

	var config database.APIConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	userID, role := getUserContext(c)
	existing, err := database.GetAPIConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Config not found: %v", err),
		})
		return
	}
	if !isAdminRole(role) && existing.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	config.ID = id
	config.UserID = existing.UserID

	// Update config
	if err := database.UpdateAPIConfig(&config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to update config: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Config updated successfully",
	})
}

// DeleteConfig deletes an API configuration
func (h *Handler) DeleteConfig(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Config ID is required",
		})
		return
	}

	userID, role := getUserContext(c)
	config, err := database.GetAPIConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Config not found: %v", err),
		})
		return
	}
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	if err := database.DeleteAPIConfig(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to delete config: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Config deleted successfully",
	})
}

// GetConfigStats returns statistics for a config
func (h *Handler) GetConfigStats(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Config ID is required",
		})
		return
	}

	// Get days parameter (default 30)
	days := 30
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	stats, err := database.GetConfigStats(id, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get stats: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
	})
}

// GetClientStats returns client activity statistics for a config
func (h *Handler) GetClientStats(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Config ID is required",
		})
		return
	}

	clientStats, err := database.GetClientStats(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get client stats: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, clientStats)
}

// Note: GetConfigLogs has been moved to config_api.go with enhanced functionality

// TestConfig tests an API configuration by making a simple request
func (h *Handler) TestConfig(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Config ID is required",
		})
		return
	}

	// Get config
	dbConfig, err := database.GetAPIConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Config not found: %v", err),
		})
		return
	}

	// Convert to config.Config
	cfg := &config.Config{
		OpenAIAPIKey:    dbConfig.OpenAIAPIKey,
		OpenAIBaseURL:   dbConfig.OpenAIBaseURL,
		BigModel:        dbConfig.BigModel,
		MiddleModel:     dbConfig.MiddleModel,
		SmallModel:      dbConfig.SmallModel,
		MaxTokensLimit:  dbConfig.MaxTokensLimit,
		RequestTimeout:  dbConfig.RequestTimeout,
		AnthropicAPIKey: dbConfig.AnthropicAPIKey,
	}

	// Create client
	testClient := client.NewOpenAIClient(cfg)

	// Parse request body if provided
	var testMessage string
	var reqBody struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&reqBody); err == nil && reqBody.Message != "" {
		testMessage = reqBody.Message
	} else {
		testMessage = "Hello, this is a test message. Please respond with 'OK'."
	}

	// Make test request
	startTime := time.Now()
	testReq := &models.OpenAIRequest{
		Model: cfg.SmallModel,
		Messages: []models.OpenAIMessage{
			{
				Role:    models.RoleUser,
				Content: testMessage,
			},
		},
		MaxTokens: 500,
	}

	// Serialize request for logging
	requestBodyJSON, _ := json.Marshal(testReq)
	requestBody := string(requestBodyJSON)

	resp, err := testClient.CreateChatCompletion(testReq)
	duration := time.Since(startTime).Milliseconds()

	// Prepare request and response details for logging
	var responseContent string
	var requestSummary = testMessage
	var responsePreview string
	var responseBody string

	// Log the request
	logEntry := &database.RequestLog{
		ConfigID:       id,
		Model:          cfg.SmallModel,
		DurationMs:     int(duration),
		Status:         "success",
		RequestSummary: requestSummary,
		RequestBody:    requestBody,
	}

	if err != nil {
		logEntry.Status = "error"
		logEntry.ErrorMessage = err.Error()
		logEntry.ResponsePreview = "测试失败: " + err.Error()
		_ = database.LogRequest(logEntry)

		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":       "failed",
			"error":        err.Error(),
			"duration_ms":  duration,
			"request_body": requestBody,
		})
		return
	}

	// Serialize response for logging
	responseBodyJSON, _ := json.Marshal(resp)
	responseBody = string(responseBodyJSON)

	// Extract response content and prepare preview
	if resp != nil && len(resp.Choices) > 0 {
		if content, ok := resp.Choices[0].Message.Content.(string); ok {
			responseContent = content
			if len(content) > 200 {
				responsePreview = content[:200] + "..."
			} else {
				responsePreview = content
			}
		}

		// Record token usage
		logEntry.InputTokens = resp.Usage.PromptTokens
		logEntry.OutputTokens = resp.Usage.CompletionTokens
		logEntry.TotalTokens = resp.Usage.TotalTokens
	}

	logEntry.ResponsePreview = responsePreview
	logEntry.ResponseBody = responseBody
	_ = database.LogRequest(logEntry)

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"message":       "Test completed successfully",
		"duration_ms":   duration,
		"model":         cfg.SmallModel,
		"response":      responseContent,
		"request_body":  requestBody,
		"response_body": responseBody,
		"usage": gin.H{
			"input_tokens":  logEntry.InputTokens,
			"output_tokens": logEntry.OutputTokens,
			"total_tokens":  logEntry.TotalTokens,
		},
	})
}
