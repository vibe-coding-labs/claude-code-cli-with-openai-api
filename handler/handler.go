package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// Handler 主处理器
type Handler struct {
	client             *client.OpenAIClient
	config             *config.Config
	authHandler        *AuthHandler
	requestValidator   *RequestValidator
	responseHandler    *ResponseHandler
	lbRegistry         *LoadBalancerManagerRegistry
	securityComponents *SecurityComponents
}

// NewHandler 创建新的处理器
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		client:             client.NewOpenAIClient(cfg),
		config:             cfg,
		authHandler:        NewAuthHandler(),
		requestValidator:   NewRequestValidator(),
		responseHandler:    NewResponseHandler(),
		lbRegistry:         NewLoadBalancerManagerRegistry(DefaultLoadBalancerManagerConfig()),
		securityComponents: nil, // Security components are optional and initialized separately
	}
}

// EnableSecurity initializes and enables security features
func (h *Handler) EnableSecurity(db *sql.DB) error {
	components, err := InitializeSecurityComponents(db)
	if err != nil {
		return fmt.Errorf("failed to initialize security components: %w", err)
	}
	h.securityComponents = components
	return nil
}

// DisableSecurity disables security features
func (h *Handler) DisableSecurity() error {
	if h.securityComponents != nil {
		if err := h.securityComponents.Close(); err != nil {
			return fmt.Errorf("failed to close security components: %w", err)
		}
		h.securityComponents = nil
	}
	return nil
}

// IsSecurityEnabled returns whether security features are enabled
func (h *Handler) IsSecurityEnabled() bool {
	return h.securityComponents != nil
}

// Shutdown gracefully shuts down the handler
func (h *Handler) Shutdown() error {
	// Shutdown load balancer registry
	if h.lbRegistry != nil {
		if err := h.lbRegistry.StopAll(); err != nil {
			return fmt.Errorf("failed to stop load balancer registry: %w", err)
		}
	}

	// Shutdown security components
	if h.securityComponents != nil {
		if err := h.securityComponents.Close(); err != nil {
			return fmt.Errorf("failed to close security components: %w", err)
		}
	}

	return nil
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
// 通过 API key 识别使用哪个配置
func (h *Handler) CreateMessage(c *gin.Context) {
	logger := utils.GetLogger()
	startTime := time.Now()

	// 提取 API key
	apiKey := h.authHandler.extractAPIKey(c)
	logger.Debug("  API Key extracted: %s", maskAPIKey(apiKey))

	// API key 是必需的
	if apiKey == "" {
		logger.Warn("← [CreateMessage] Missing API key")
		utils.GetLogger().LogResponse(http.StatusUnauthorized, time.Since(startTime))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": map[string]interface{}{
				"type":    "authentication_error",
				"message": "Missing API key. Please provide a valid Anthropic API key.",
			},
		})
		return
	}

	// 首先尝试查找负载均衡器
	loadBalancer, lbErr := database.GetLoadBalancerByAPIKey(apiKey)
	if lbErr == nil && loadBalancer != nil {
		logger.Info("  Found load balancer by API key: %s (%s)", loadBalancer.ID, loadBalancer.Name)

		// 检查负载均衡器是否启用
		if !loadBalancer.Enabled {
			logger.Warn("← [CreateMessage] Load balancer disabled: %s", loadBalancer.ID)
			utils.GetLogger().LogResponse(http.StatusForbidden, time.Since(startTime))
			c.JSON(http.StatusForbidden, gin.H{
				"error": map[string]interface{}{
					"type":    "permission_error",
					"message": "This load balancer is currently disabled.",
				},
			})
			return
		}

		// 获取或创建负载均衡器管理器（包含健康检查、熔断器等增强功能）
		lbManager, err := h.lbRegistry.GetManager(loadBalancer.ID)
		if err != nil {
			logger.Error("← [CreateMessage] Failed to get load balancer manager: %v", err)
			utils.GetLogger().LogResponse(http.StatusInternalServerError, time.Since(startTime))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": map[string]interface{}{
					"type":    "internal_error",
					"message": "Failed to initialize load balancer",
				},
			})
			return
		}

		// 获取选择器
		selector := lbManager.GetSelector()

		// 选择一个配置
		selectedConfig, err := selector.SelectConfig(c.Request.Context())
		if err != nil {
			logger.Error("← [CreateMessage] Failed to select config from load balancer: %v", err)
			utils.GetLogger().LogResponse(http.StatusInternalServerError, time.Since(startTime))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": map[string]interface{}{
					"type":    "internal_error",
					"message": "No available configurations in load balancer",
				},
			})
			return
		}

		logger.Info("  Selected config from load balancer: %s (%s)", selectedConfig.ID, selectedConfig.Name)

		// 使用选中的配置处理请求，并通过管理器记录请求
		h.handleMessageWithLoadBalancer(c, selectedConfig, lbManager)

		// 释放连接（用于最少连接策略）
		if loadBalancer.Strategy == "least_connections" {
			selector.ReleaseConnection(selectedConfig.ID)
		}

		return
	}

	// 如果不是负载均衡器，尝试查找普通配置
	dbConfig, err := database.GetConfigByAnthropicAPIKey(apiKey)
	if err != nil || dbConfig == nil {
		logger.Warn("← [CreateMessage] Invalid API key: %s", maskAPIKey(apiKey))
		utils.GetLogger().LogResponse(http.StatusUnauthorized, time.Since(startTime))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": map[string]interface{}{
				"type":    "authentication_error",
				"message": "Invalid API key. Please check your Anthropic API key and try again.",
			},
		})
		return
	}

	logger.Info("  Found config by API key: %s (%s)", dbConfig.ID, dbConfig.Name)

	// 检查配置是否启用
	if !dbConfig.Enabled {
		logger.Warn("← [CreateMessage] Config disabled: %s", dbConfig.ID)
		utils.GetLogger().LogResponse(http.StatusForbidden, time.Since(startTime))
		c.JSON(http.StatusForbidden, gin.H{
			"error": map[string]interface{}{
				"type":    "permission_error",
				"message": "This configuration is currently disabled.",
			},
		})
		return
	}

	h.handleMessageWithConfig(c, dbConfig)
}

// maskAPIKey 掩码 API key 用于日志
func maskAPIKey(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:8] + "****" + key[len(key)-4:]
}

// CreateMessageWithConfig 处理 /v1/configs/:id/messages 端点
func (h *Handler) CreateMessageWithConfig(c *gin.Context) {
	logger := utils.GetLogger()
	startTime := time.Now()

	configID := c.Param("id")
	logger.Info("→ [CreateMessageWithConfig] Request received")
	logger.Debug("  Config ID: %s", configID)
	logger.Debug("  URL: %s", c.Request.URL.String())
	logger.Debug("  Method: %s", c.Request.Method)
	logger.Debug("  Remote Address: %s", c.ClientIP())

	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 20 {
		logger.Debug("  Headers: Content-Type=%s, Authorization=%s...",
			c.GetHeader("Content-Type"), authHeader[:20])
	} else {
		logger.Debug("  Headers: Content-Type=%s, Authorization=%s",
			c.GetHeader("Content-Type"), authHeader)
	}

	// 从数据库获取配置
	logger.Debug("  Fetching config from database...")
	dbConfig, err := database.GetAPIConfig(configID)
	if err != nil {
		logger.Error("← [CreateMessageWithConfig] Config not found: %s, error: %v", configID, err)
		utils.GetLogger().LogResponse(http.StatusNotFound, time.Since(startTime))
		c.JSON(http.StatusNotFound, gin.H{
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": fmt.Sprintf("Config not found: %s", configID),
			},
		})
		return
	}
	logger.Info("  Config loaded: %s (Enabled: %v)", dbConfig.Name, dbConfig.Enabled)

	if !dbConfig.Enabled {
		logger.Warn("← [CreateMessageWithConfig] Config disabled: %s", configID)
		utils.GetLogger().LogResponse(http.StatusBadRequest, time.Since(startTime))
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
	h.handleMessageWithConfigAndManager(c, dbConfig, nil)
}

// handleMessageWithLoadBalancer 处理通过负载均衡器的消息请求
func (h *Handler) handleMessageWithLoadBalancer(c *gin.Context, dbConfig *database.APIConfig, lbManager *LoadBalancerManager) {
	h.handleMessageWithConfigAndManager(c, dbConfig, lbManager)
}

// handleMessageWithConfigAndManager 处理消息请求的核心逻辑（支持负载均衡器管理器）
func (h *Handler) handleMessageWithConfigAndManager(c *gin.Context, dbConfig *database.APIConfig, lbManager *LoadBalancerManager) {
	logger := utils.GetLogger()
	startTime := time.Now()
	logger.Info("→ [handleMessageWithConfig] Processing message request")

	// 解析请求
	logger.Debug("  Parsing request body...")
	var req models.ClaudeMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("← [handleMessageWithConfig] Failed to parse request: %v", err)
		utils.GetLogger().LogResponse(http.StatusBadRequest, time.Since(startTime))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}
	logger.Debug("  Request parsed successfully: Model=%s, Stream=%v, Messages=%d", req.Model, req.Stream, len(req.Messages))

	// 记录请求详情
	h.requestValidator.LogRequestDetails(&req)

	// 处理连接测试请求
	if h.requestValidator.IsConnectivityTest(&req) {
		h.requestValidator.HandleConnectivityTest(c, &req)
		return
	}

	// 验证请求参数
	logger.Debug("  Validating request...")
	if err := h.requestValidator.ValidateRequest(&req); err != nil {
		logger.Error("← [handleMessageWithConfig] Request validation failed: %v", err)
		utils.GetLogger().LogResponse(http.StatusBadRequest, time.Since(startTime))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": err.Error(),
			},
		})
		return
	}
	logger.Debug("  Request validation passed")

	// 配置设置
	logger.Debug("  Setting up configuration...")
	var targetConfig *config.Config
	var targetClient *client.OpenAIClient

	if dbConfig != nil {
		logger.Debug("  Using database config: %s", dbConfig.Name)
		c.Set("user_id", dbConfig.UserID)

		// 验证 API Key（仅当不是通过负载均衡器时）
		if lbManager == nil {
			if valid, errorResp := h.authHandler.ValidateAPIKey(c, dbConfig); !valid {
				h.authHandler.SendAuthError(c, http.StatusUnauthorized, errorResp)
				return
			}
		} else {
			logger.Debug("  Skipping API key validation (using load balancer)")
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
		logger.Debug("  Using default config")
		targetConfig = h.config
		targetClient = h.client
	}

	// 转换请求
	logger.Debug("  Converting Claude request to OpenAI format...")
	logger.Debug("  Target config: BigModel=%s, MiddleModel=%s, SmallModel=%s",
		targetConfig.BigModel, targetConfig.MiddleModel, targetConfig.SmallModel)

	openAIReq := converter.ConvertClaudeToOpenAIWithConfig(&req, targetConfig)
	logger.Info("  Converted to OpenAI model: %s", openAIReq.Model)
	logger.Debug("  OpenAI request: messages=%d, max_tokens=%d, stream=%v",
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
	logger.Debug("  Handling response (stream=%v)...", req.Stream)

	// 获取 config ID（如果有）
	configID := ""
	if dbConfig != nil {
		configID = dbConfig.ID
	}

	if req.Stream {
		h.responseHandler.HandleStreamingResponse(c, targetClient, openAIReq, &req, configID, startTime)
	} else {
		h.responseHandler.HandleNonStreamingResponse(c, targetClient, openAIReq, &req, configID, startTime)
	}
	logger.Info("← [handleMessageWithConfig] Request completed in %v", time.Since(startTime))
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
// 如果是浏览器访问，重定向到 /ui/
// 如果是API调用，返回JSON
func (h *Handler) Root(c *gin.Context) {
	userAgent := c.GetHeader("User-Agent")

	// 检测是否是浏览器
	if isBrowserUserAgent(userAgent) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
		return
	}

	// 非浏览器访问，返回JSON
	h.Index(c)
}

// isBrowserUserAgent 检测User-Agent是否是浏览器
func isBrowserUserAgent(ua string) bool {
	if ua == "" {
		return false
	}

	// 常见浏览器User-Agent关键字
	browserKeywords := []string{
		"Mozilla",
		"Chrome",
		"Safari",
		"Firefox",
		"Edge",
		"Opera",
		"MSIE",
		"Trident",
	}

	// 排除一些API客户端
	apiClientKeywords := []string{
		"curl",
		"wget",
		"python",
		"go-http-client",
		"axios",
		"okhttp",
		"java",
		"apache-httpclient",
		"postman",
		"insomnia",
	}

	// 先检查是否是API客户端
	for _, keyword := range apiClientKeywords {
		if containsIgnoreCase(ua, keyword) {
			return false
		}
	}

	// 再检查是否包含浏览器关键字
	for _, keyword := range browserKeywords {
		if containsIgnoreCase(ua, keyword) {
			return true
		}
	}

	return false
}

// containsIgnoreCase 不区分大小写的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// HealthCheck 处理健康检查（别名）
func (h *Handler) HealthCheck(c *gin.Context) {
	h.Health(c)
}

// GetSecurityMiddleware returns the security middleware chain
// Returns nil if security is not enabled
func (h *Handler) GetSecurityMiddleware() []gin.HandlerFunc {
	if !h.IsSecurityEnabled() {
		return nil
	}

	middleware := h.securityComponents.Middleware
	usageTracker := h.securityComponents.UsageTracker

	return []gin.HandlerFunc{
		middleware.IPFilterMiddleware(),
		middleware.AuthenticationMiddleware(),
		middleware.RateLimitMiddleware(),
		middleware.HMACVerificationMiddleware(),
		middleware.QuotaCheckMiddleware(),
		middleware.UsageTrackingMiddleware(usageTracker),
	}
}
