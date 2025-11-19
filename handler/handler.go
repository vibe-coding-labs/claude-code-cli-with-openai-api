package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// Handler 主处理器
type Handler struct {
	client           *client.OpenAIClient
	config           *config.Config
	authHandler      *AuthHandler
	requestValidator *RequestValidator
	responseHandler  *ResponseHandler
}

// NewHandler 创建新的处理器
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		client:           client.NewOpenAIClient(cfg),
		config:           cfg,
		authHandler:      NewAuthHandler(),
		requestValidator: NewRequestValidator(),
		responseHandler:  NewResponseHandler(),
	}
}

// ValidateAPIKey middleware 验证 API Key
func (h *Handler) ValidateAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip validation if ANTHROPIC_API_KEY is not set
		if h.config.AnthropicAPIKey == "" {
			c.Next()
			return
		}

		// Use auth handler to validate
		clientAPIKey := h.authHandler.extractAPIKey(c)
		if clientAPIKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": map[string]interface{}{
					"type":    "authentication_error",
					"message": "Missing API key",
				},
			})
			c.Abort()
			return
		}

		if clientAPIKey != h.config.AnthropicAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": map[string]interface{}{
					"type":    "authentication_error",
					"message": "Invalid API key",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// CreateMessage 处理 /v1/messages 端点
func (h *Handler) CreateMessage(c *gin.Context) {
	h.handleMessageWithConfig(c, nil)
}

// CreateMessageWithConfig 处理 /v1/configs/:id/messages 端点
func (h *Handler) CreateMessageWithConfig(c *gin.Context) {
	configID := c.Param("id")
	fmt.Printf("\n🔵 [CreateMessageWithConfig] Config ID: %s\n", configID)
	fmt.Printf("   URL: %s\n", c.Request.URL.String())
	fmt.Printf("   Method: %s\n", c.Request.Method)

	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 20 {
		fmt.Printf("   Headers: Content-Type=%s, Authorization=%s...\n",
			c.GetHeader("Content-Type"), authHeader[:20])
	} else {
		fmt.Printf("   Headers: Content-Type=%s, Authorization=%s\n",
			c.GetHeader("Content-Type"), authHeader)
	}

	// 从数据库获取配置
	dbConfig, err := database.GetAPIConfig(configID)
	if err != nil {
		fmt.Printf("❌ [CreateMessageWithConfig] Config not found: %s, error: %v\n", configID, err)
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": fmt.Sprintf("Config not found: %s", configID),
			},
		})
		return
	}
	fmt.Printf("✅ [CreateMessageWithConfig] Config loaded: %s\n", dbConfig.Name)

	if !dbConfig.Enabled {
		fmt.Printf("❌ [CreateMessageWithConfig] Config disabled: %s\n", configID)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request",
				"message": fmt.Sprintf("Config %s is disabled", configID),
			},
		})
		return
	}

	h.handleMessageWithConfig(c, dbConfig)
}

// handleMessageWithConfig 处理消息请求的核心逻辑
func (h *Handler) handleMessageWithConfig(c *gin.Context, dbConfig *database.APIConfig) {
	fmt.Printf("\n🟢 [handleMessageWithConfig] Starting...\n")

	// 解析请求
	var req models.ClaudeMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Printf("❌ [handleMessageWithConfig] Failed to parse request: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	// 记录请求详情
	h.requestValidator.LogRequestDetails(&req)

	// 处理连接测试请求
	if h.requestValidator.IsConnectivityTest(&req) {
		h.requestValidator.HandleConnectivityTest(c, &req)
		return
	}

	// 验证请求参数
	if err := h.requestValidator.ValidateRequest(&req); err != nil {
		fmt.Printf("❌ [handleMessageWithConfig] Request validation failed: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": err.Error(),
			},
		})
		return
	}

	// 配置设置
	fmt.Printf("\n🔧 [Config Setup]\n")
	var targetConfig *config.Config
	var targetClient *client.OpenAIClient

	if dbConfig != nil {
		fmt.Printf("   Using database config: %s\n", dbConfig.Name)

		// 验证 API Key
		if valid, errorResp := h.authHandler.ValidateAPIKey(c, dbConfig); !valid {
			h.authHandler.SendAuthError(c, http.StatusUnauthorized, errorResp)
			return
		}

		// 使用数据库配置创建临时配置和客户端
		targetConfig = &config.Config{
			OpenAIBaseURL:   dbConfig.OpenAIBaseURL,
			OpenAIAPIKey:    dbConfig.OpenAIAPIKey,
			BigModel:        dbConfig.BigModel,
			MiddleModel:     dbConfig.MiddleModel,
			SmallModel:      dbConfig.SmallModel,
			MaxTokensLimit:  dbConfig.MaxTokensLimit,
			RequestTimeout:  h.config.RequestTimeout,
			AnthropicAPIKey: dbConfig.AnthropicAPIKey,
		}
		targetClient = client.NewOpenAIClient(targetConfig)
	} else {
		fmt.Printf("   Using default config\n")
		targetConfig = h.config
		targetClient = h.client
	}

	// 转换请求
	fmt.Printf("\n🔄 [Request Conversion]\n")
	fmt.Printf("   Converting Claude request to OpenAI format\n")
	fmt.Printf("   Target config: BigModel=%s, MiddleModel=%s, SmallModel=%s\n",
		targetConfig.BigModel, targetConfig.MiddleModel, targetConfig.SmallModel)

	openAIReq := converter.ConvertClaudeToOpenAIWithConfig(&req, targetConfig)
	fmt.Printf("   ✅ Converted to OpenAI model: %s\n", openAIReq.Model)
	fmt.Printf("   OpenAI request: messages=%d, max_tokens=%d, stream=%v\n",
		len(openAIReq.Messages), openAIReq.MaxTokens, openAIReq.Stream)

	// 检查客户端是否断开
	if c.Request.Context().Err() != nil {
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error": map[string]interface{}{
				"type":    "cancelled",
				"message": "Client disconnected",
			},
		})
		return
	}

	// 处理响应
	if req.Stream {
		h.responseHandler.HandleStreamingResponse(c, targetClient, openAIReq, &req)
	} else {
		h.responseHandler.HandleNonStreamingResponse(c, targetClient, openAIReq, &req)
	}
}

// CountTokens 处理 /v1/messages/count_tokens 端点
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

	// 简单估算（实际应该更精确）
	totalTokens := len(req.Messages) * 100 // 粗略估算

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": totalTokens,
	})
}

// Health 处理健康检查
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": fmt.Sprintf("%d", 1234567890),
	})
}

// TestConnection 测试连接
func (h *Handler) TestConnection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Connection successful",
	})
}

// Index 处理根路径
func (h *Handler) Index(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "claude-to-openai-proxy",
		"version": "1.0.0",
	})
}

// Root 处理根路径（别名）
func (h *Handler) Root(c *gin.Context) {
	h.Index(c)
}

// HealthCheck 处理健康检查（别名）
func (h *Handler) HealthCheck(c *gin.Context) {
	h.Health(c)
}
