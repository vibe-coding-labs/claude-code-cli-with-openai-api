package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

type Handler struct {
	client *client.OpenAIClient
	config *config.Config
}

func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		client: client.NewOpenAIClient(cfg),
		config: cfg,
	}
}

// ValidateAPIKey middleware
func (h *Handler) ValidateAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip validation if ANTHROPIC_API_KEY is not set
		if h.config.AnthropicAPIKey == "" {
			c.Next()
			return
		}

		// Extract API key from headers
		clientAPIKey := c.GetHeader("x-api-key")
		if clientAPIKey == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				clientAPIKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		// Validate the client API key
		if !h.config.ValidateClientAPIKey(clientAPIKey) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": map[string]interface{}{
					"type":    "authentication_error",
					"message": "Invalid API key. Please provide a valid Anthropic API key.",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// CreateMessage handles /v1/messages endpoint
func (h *Handler) CreateMessage(c *gin.Context) {
	h.handleMessageWithConfig(c, nil)
}

// CreateMessageWithConfig handles /v1/configs/:id/messages endpoint
func (h *Handler) CreateMessageWithConfig(c *gin.Context) {
	configID := c.Param("id")
	cfg, err := config.GetConfigManager().GetConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": fmt.Sprintf("Config not found: %s", configID),
			},
		})
		return
	}

	if !cfg.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request",
				"message": fmt.Sprintf("Config %s is disabled", configID),
			},
		})
		return
	}

	h.handleMessageWithConfig(c, cfg)
}

// handleMessageWithConfig handles message creation with optional config
func (h *Handler) handleMessageWithConfig(c *gin.Context, apiConfig *models.APIConfig) {
	var req models.ClaudeMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	// Use provided config or fall back to default
	var targetConfig *config.Config
	var targetClient *client.OpenAIClient
	
	if apiConfig != nil {
		// Validate API key if config has one set
		if apiConfig.AnthropicAPIKey != "" {
			clientAPIKey := c.GetHeader("x-api-key")
			if clientAPIKey == "" {
				authHeader := c.GetHeader("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					clientAPIKey = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			
			if clientAPIKey != apiConfig.AnthropicAPIKey {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": map[string]interface{}{
						"type":    "authentication_error",
						"message": "Invalid API key for this configuration",
					},
				})
				return
			}
		}
		
		targetConfig = config.ToConfigFromAPIConfig(apiConfig)
		targetClient = client.NewOpenAIClient(targetConfig)
	} else {
		// Use default handler config
		targetConfig = h.config
		targetClient = h.client
		
		// Validate API key if needed
		if h.config.AnthropicAPIKey != "" {
			clientAPIKey := c.GetHeader("x-api-key")
			if clientAPIKey == "" {
				authHeader := c.GetHeader("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					clientAPIKey = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			
			if !h.config.ValidateClientAPIKey(clientAPIKey) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": map[string]interface{}{
						"type":    "authentication_error",
						"message": "Invalid API key. Please provide a valid Anthropic API key.",
					},
				})
				return
			}
		}
	}

	// Convert Claude request to OpenAI format
	openAIReq := converter.ConvertClaudeToOpenAI(&req)

	// Check if client disconnected before processing
	if c.Request.Context().Err() != nil {
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error": map[string]interface{}{
				"type":    "cancelled",
				"message": "Client disconnected",
			},
		})
		return
	}

	if req.Stream {
		// Handle streaming response
		reader, err := targetClient.CreateChatCompletionStream(openAIReq)
		if err != nil {
			// Extract error message and status code
			errorMsg := err.Error()
			statusCode := http.StatusInternalServerError

			// Try to extract status code from error message if available
			// Error format: "OpenAI API error (status 401): ..."
			if strings.Contains(errorMsg, "status") {
				var extractedStatus int
				if n, _ := fmt.Sscanf(errorMsg, "OpenAI API error (status %d):", &extractedStatus); n == 1 {
					if extractedStatus >= 400 && extractedStatus < 600 {
						statusCode = extractedStatus
					}
				}
			}

			// Extract the actual error message (after the status code)
			// Format: "OpenAI API error (status 401): actual error message"
			if idx := strings.Index(errorMsg, "): "); idx > 0 {
				errorMsg = errorMsg[idx+3:]
			}

			// Classify and format error message
			classifiedError := client.ClassifyOpenAIError(errorMsg)

			c.JSON(statusCode, gin.H{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": classifiedError,
				},
			})
			return
		}
		defer reader.Close()

		converter.ConvertOpenAIStreamingToClaude(c, reader, &req, c.Request.Context())
	} else {
		// Handle non-streaming response
		openAIResp, err := targetClient.CreateChatCompletion(openAIReq)
		if err != nil {
			// Extract error message and status code
			errorMsg := err.Error()
			statusCode := http.StatusInternalServerError

			// Try to extract status code from error message if available
			// Error format: "OpenAI API error (status 401): ..."
			if strings.Contains(errorMsg, "status") {
				var extractedStatus int
				if n, _ := fmt.Sscanf(errorMsg, "OpenAI API error (status %d):", &extractedStatus); n == 1 {
					if extractedStatus >= 400 && extractedStatus < 600 {
						statusCode = extractedStatus
					}
				}
			}

			// Extract the actual error message (after the status code)
			// Format: "OpenAI API error (status 401): actual error message"
			if idx := strings.Index(errorMsg, "): "); idx > 0 {
				errorMsg = errorMsg[idx+3:]
			}

			// Classify and format error message
			classifiedError := client.ClassifyOpenAIError(errorMsg)

			c.JSON(statusCode, gin.H{
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": classifiedError,
				},
			})
			return
		}

		claudeResp := converter.ConvertOpenAIToClaudeResponse(openAIResp, &req)
		c.JSON(http.StatusOK, claudeResp)
	}
}

// CountTokens handles /v1/messages/count_tokens endpoint
func (h *Handler) CountTokens(c *gin.Context) {
	var req models.ClaudeTokenCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	totalChars := 0

	// Count system message characters
	if req.System != nil {
		if systemStr, ok := req.System.(string); ok {
			totalChars += len(systemStr)
		} else if systemArr, ok := req.System.([]interface{}); ok {
			for _, block := range systemArr {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if text, ok := blockMap["text"].(string); ok {
						totalChars += len(text)
					}
				}
			}
		}
	}

	// Count message characters
	for _, msg := range req.Messages {
		if msg.Content == nil {
			continue
		}
		if contentStr, ok := msg.Content.(string); ok {
			totalChars += len(contentStr)
		} else if contentArr, ok := msg.Content.([]interface{}); ok {
			for _, block := range contentArr {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if text, ok := blockMap["text"].(string); ok {
						totalChars += len(text)
					}
				}
			}
		}
	}

	// Rough estimation: 4 characters per token
	estimatedTokens := totalChars / 4
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": estimatedTokens,
	})
}

// HealthCheck handles /health endpoint
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":                    "healthy",
		"timestamp":                 time.Now().Format(time.RFC3339),
		"openai_api_configured":     h.config.OpenAIAPIKey != "",
		"api_key_valid":             h.config.ValidateAPIKey(),
		"client_api_key_validation": h.config.AnthropicAPIKey != "",
	})
}

// TestConnection handles /test-connection endpoint
func (h *Handler) TestConnection(c *gin.Context) {
	testReq := &models.OpenAIRequest{
		Model: h.config.SmallModel,
		Messages: []models.OpenAIMessage{
			{
				Role:    models.RoleUser,
				Content: "Hello",
			},
		},
		MaxTokens: 5,
	}

	resp, err := h.client.CreateChatCompletion(testReq)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":     "failed",
			"error_type": "API Error",
			"message":    err.Error(),
			"timestamp":  time.Now().Format(time.RFC3339),
			"suggestions": []string{
				"Check your OPENAI_API_KEY is valid",
				"Verify your API key has the necessary permissions",
				"Check if you have reached rate limits",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"message":     "Successfully connected to OpenAI API",
		"model_used":  h.config.SmallModel,
		"timestamp":   time.Now().Format(time.RFC3339),
		"response_id": resp.ID,
	})
}

// Root handles / endpoint
func (h *Handler) Root(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Claude-to-OpenAI API Proxy (Golang) v1.0.0",
		"status":  "running",
		"config": gin.H{
			"openai_base_url":           h.config.OpenAIBaseURL,
			"max_tokens_limit":          h.config.MaxTokensLimit,
			"api_key_configured":        h.config.OpenAIAPIKey != "",
			"client_api_key_validation": h.config.AnthropicAPIKey != "",
			"big_model":                 h.config.BigModel,
			"middle_model":              h.config.MiddleModel,
			"small_model":               h.config.SmallModel,
		},
		"endpoints": gin.H{
			"messages":        "/v1/messages",
			"count_tokens":    "/v1/messages/count_tokens",
			"health":          "/health",
			"test_connection": "/test-connection",
		},
	})
}
