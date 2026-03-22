package handler

import (
	"database/sql"
	"encoding/json"
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
	sessionHandler     *SessionHandler
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
		securityComponents: nil,
		sessionHandler:     NewSessionHandler(),
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

	// 提取 beta headers
	betaHeaders := extractBetaHeaders(c)
	// 检测是否为 Claude Code 客户端
	if isClaudeCode(c) {
		logger.Debug("  Detected Claude Code client")
		// 为 Claude Code 客户端自动添加默认 beta headers
		betaHeaders = appendDefaultBetaHeaders(betaHeaders)
	}
	logger.Debug("  Beta headers: %v", betaHeaders)

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

		// 使用负载均衡器的 ExecuteRequest 方法，支持自动重试和故障转移
		var lastErr error
		requestExecuted := false

		err = lbManager.ExecuteRequest(func(config *database.APIConfig) error {
			// 如果已经执行过（重试情况），记录日志
			if requestExecuted {
				logger.Info("  Retrying with different config, previous error: %v", lastErr)
			}
			requestExecuted = true

			// 释放连接（用于最少连接策略）
			if loadBalancer.Strategy == "least_connections" {
				defer lbManager.GetSelector().ReleaseConnection(config.ID)
			}

			// 执行请求
			err := h.executeMessageRequestWithConfig(c, config, lbManager, betaHeaders)
			if err != nil {
				lastErr = err
				return err
			}
			return nil
		})

		if err != nil {
			logger.Error("← [CreateMessage] Request failed after all retries: %v", err)
			// 错误已经在 executeMessageRequestWithConfig 中发送到客户端
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

	h.handleMessageWithConfig(c, dbConfig, betaHeaders)
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

	// 提取 beta headers
	betaHeaders := extractBetaHeaders(c)
	if isClaudeCode(c) {
		logger.Debug("  Detected Claude Code client")
		betaHeaders = appendDefaultBetaHeaders(betaHeaders)
	}
	logger.Debug("  Beta headers: %v", betaHeaders)

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

	h.handleMessageWithConfig(c, dbConfig, betaHeaders)
}

// handleMessageWithConfig 处理消息请求的核心逻辑（支持重试）
func (h *Handler) handleMessageWithConfig(c *gin.Context, dbConfig *database.APIConfig, betaHeaders []string) {
	logger := utils.GetLogger()
	startTime := time.Now()

	// 普通配置也使用指数退避重试
	maxRetries := 20
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 计算指数退避延迟: baseDelay * 2^(attempt-1)，上限 maxDelay
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxDelay {
				delay = maxDelay
			}
			logger.Info("  ⏱️  Retry attempt %d/%d after %v backoff", attempt, maxRetries, delay)
			time.Sleep(delay)
		}

		// 尝试执行请求
		err := h.executeMessageRequestWithConfig(c, dbConfig, nil, betaHeaders)
		if err == nil {
			// 成功
			if attempt > 0 {
				logger.Info("  Request succeeded after %d retries", attempt)
			}
			return
		}

		lastErr = err
		logger.Warn("  Request failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)

		// 检查是否是可重试错误
		if !client.IsRetryableError(err) {
			logger.Error("← [handleMessageWithConfig] Non-retryable error: %v", err)
			return
		}

		// 检查是否是最后一次尝试
		if attempt >= maxRetries {
			logger.Error("← [handleMessageWithConfig] Max retries exceeded: %v", err)
			return
		}
	}

	logger.Error("← [handleMessageWithConfig] All retries failed after %v: %v", time.Since(startTime), lastErr)
}

// handleMessageWithLoadBalancer 处理通过负载均衡器的消息请求
func (h *Handler) handleMessageWithLoadBalancer(c *gin.Context, dbConfig *database.APIConfig, lbManager *LoadBalancerManager, betaHeaders []string) {
	h.handleMessageWithConfigAndManager(c, dbConfig, lbManager, betaHeaders)
}

// executeMessageRequestWithConfig 执行消息请求，返回 error 以便支持重试机制
func (h *Handler) executeMessageRequestWithConfig(c *gin.Context, dbConfig *database.APIConfig, lbManager *LoadBalancerManager, betaHeaders []string) error {
	logger := utils.GetLogger()
	startTime := time.Now()
	logger.Info("→ [executeMessageRequestWithConfig] Processing message request with config: %s", dbConfig.ID)

	// 解析请求
	logger.Debug("  Parsing request body...")
	var req models.ClaudeMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("← [executeMessageRequestWithConfig] Failed to parse request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		// 请求解析错误不需要重试（所有节点都会失败）
		return fmt.Errorf("request parse error: %w", err)
	}
	logger.Debug("  Request parsed successfully: Model=%s, Stream=%v, Messages=%d", req.Model, req.Stream, len(req.Messages))

	// 记录请求详情
	h.requestValidator.LogRequestDetails(&req)

	// 处理连接测试请求
	if h.requestValidator.IsConnectivityTest(&req) {
		h.requestValidator.HandleConnectivityTest(c, &req)
		return nil
	}

	// 验证请求参数
	logger.Debug("  Validating request...")
	if err := h.requestValidator.ValidateRequest(&req); err != nil {
		logger.Error("← [executeMessageRequestWithConfig] Request validation failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": err.Error(),
			},
		})
		// 请求验证错误不需要重试（所有节点都会失败）
		return fmt.Errorf("request validation error: %w", err)
	}
	logger.Debug("  Request validation passed")

	// 处理会话和对话历史恢复
	var session *database.Session
	if h.sessionHandler != nil && dbConfig != nil {
		sessionID := h.sessionHandler.ExtractSessionID(&req)
		userID := fmt.Sprintf("%d", dbConfig.UserID)
		configID := dbConfig.ID

		// 获取或创建会话
		var err error
		session, err = h.sessionHandler.GetOrCreateSession(sessionID, userID, configID, &req)
		if err != nil {
			logger.Warn("  Failed to get or create session: %v", err)
		} else {
			logger.Info("  Session: %s", session.ID)

			// 如果有 session ID，加载对话历史
			if sessionID != "" && len(req.Messages) > 0 {
				// Claude Code 客户端会自己管理对话历史，每次请求都包含完整消息
				// 服务器不需要追加数据库历史，否则会导致消息重复
				// 只保存新消息到数据库用于记录和审计
				logger.Info("  Using client-provided conversation history (%d messages)", len(req.Messages))
			}

			// 将 session ID 添加到响应 header
			c.Header("X-Session-ID", session.ID)
		}
	}

	// 配置设置
	logger.Debug("  Setting up configuration...")
	var targetConfig *config.Config
	var targetClient *client.OpenAIClient

	if dbConfig != nil {
		logger.Debug("  Using database config: %s", dbConfig.Name)
		c.Set("user_id", dbConfig.UserID)

		// 确定实际使用的模型
		actualModel := utils.MapClaudeModelToOpenAIWithConfig(req.Model, &config.Config{
			BigModel:    dbConfig.BigModel,
			MiddleModel: dbConfig.MiddleModel,
			SmallModel:  dbConfig.SmallModel,
		})

		// 获取对应模型的思考级别
		reasoningEffort := utils.GetReasoningEffortForModel(dbConfig, actualModel)
		logger.Debug("  Model mapping: %s -> %s, reasoning_effort=%s", req.Model, actualModel, reasoningEffort)

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
			ReasoningEffort: reasoningEffort, // 使用根据模型选择的思考级别
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

	openAIReq := converter.ConvertClaudeToOpenAIWithConfig(&req, targetConfig, betaHeaders)

	// 检查是否包含工具调用
	hasTools := len(req.Tools) > 0
	forceNonStream := hasTools && req.Stream
	if forceNonStream {
		logger.Info("  Request has tools with streaming, forcing non-stream upstream request")
		openAIReq.Stream = false
	}

	logger.Info("  Converted to OpenAI model: %s", openAIReq.Model)
	logger.Info("  OpenAI request: messages=%d, max_tokens=%d, stream=%v (original stream=%v), reasoning_effort=%s, tools=%v",
		len(openAIReq.Messages), openAIReq.MaxTokens, openAIReq.Stream, req.Stream, openAIReq.ReasoningEffort, hasTools)

	// 检查客户端是否断开
	if c.Request.Context().Err() != nil {
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error": map[string]interface{}{
				"type":    "cancelled",
				"message": "Client disconnected",
			},
		})
		// 客户端断开不需要重试
		return fmt.Errorf("client disconnected")
	}

	// 处理响应
	logger.Debug("  Handling response (stream=%v)...", req.Stream)

	// 获取 config ID（如果有）
	configID := ""
	if dbConfig != nil {
		configID = dbConfig.ID
	}

	// 准备会话参数
	var sessionID string
	if session != nil {
		sessionID = session.ID
	}

	if req.Stream {
		// 流式响应
		if forceNonStream {
			// 工具+流式：使用非流式请求，但包装成SSE流返回
			logger.Info("  Using non-stream request with SSE wrapper for tool calls")
			h.handleNonStreamAsStream(c, targetClient, openAIReq, &req, configID, startTime, sessionID, betaHeaders)
		} else {
			// 普通流式响应
			// Set beta headers for upstream request
			targetClient.BetaHeaders = betaHeaders
			reader, err := targetClient.CreateChatCompletionStream(openAIReq)
			if err != nil {
				logger.Error("← [executeMessageRequestWithConfig] Stream creation failed: %v", err)
				h.responseHandler.SendErrorResponse(c, err)
				h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), &req, nil)
				return fmt.Errorf("stream creation failed: %w", err)
			}
			defer reader.Close()

			logger.Info("  Stream created successfully, processing response...")
			streamResult := converter.ConvertOpenAIStreamingToClaude(c, reader, &req, c.Request.Context())

			if streamResult != nil {
				h.responseHandler.logRequestWithStreamingDetails(c, configID, openAIReq.Model, streamResult, startTime, "success", "", &req)
				// 保存消息到会话
				if h.sessionHandler != nil && sessionID != "" {
					h.responseHandler.SaveMessagesToSession(h.sessionHandler, sessionID, &req, streamResult.Content, streamResult.InputTokens, streamResult.OutputTokens)
				}
			} else {
				h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", "Streaming failed", &req, nil)
			}
		}
	} else {
		// 非流式响应
		// Set beta headers for upstream request
		targetClient.BetaHeaders = betaHeaders
		openAIResp, err := targetClient.CreateChatCompletion(openAIReq)
		if err != nil {
			logger.Error("← [executeMessageRequestWithConfig] Request failed: %v", err)
			h.responseHandler.SendErrorResponse(c, err)
			h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), &req, nil)
			return fmt.Errorf("request failed: %w", err)
		}

		logger.Info("  Response received, converting to Claude format...")
		claudeResp := converter.ConvertOpenAIToClaudeResponse(openAIResp, &req)
		if claudeResp == nil {
			err := fmt.Errorf("failed to convert OpenAI response to Claude format: response or choices is empty")
			h.responseHandler.SendErrorResponse(c, err)
			h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), &req, nil)
			return fmt.Errorf("response conversion error: %w", err)
		}

		h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model,
			claudeResp.Usage.InputTokens,
			claudeResp.Usage.OutputTokens,
			startTime, "success", "", &req, claudeResp)

		c.JSON(http.StatusOK, claudeResp)
		logger.Info("  Response sent successfully")

		// 保存消息到会话
		if h.sessionHandler != nil && sessionID != "" {
			assistantContent := ""
			if len(claudeResp.Content) > 0 && claudeResp.Content[0].Type == "text" {
				assistantContent = claudeResp.Content[0].Text
			}
			h.responseHandler.SaveMessagesToSession(h.sessionHandler, sessionID, &req, assistantContent,
				claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens)
		}
	}

	logger.Info("← [executeMessageRequestWithConfig] Request completed in %v", time.Since(startTime))
	return nil
}

// handleMessageWithConfigAndManager 处理消息请求的核心逻辑（支持负载均衡器管理器）
func (h *Handler) handleMessageWithConfigAndManager(c *gin.Context, dbConfig *database.APIConfig, lbManager *LoadBalancerManager, betaHeaders []string) {
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

	// 处理会话和对话历史恢复
	var session *database.Session
	if h.sessionHandler != nil && dbConfig != nil {
		sessionID := h.sessionHandler.ExtractSessionID(&req)
		userID := fmt.Sprintf("%d", dbConfig.UserID)
		configID := dbConfig.ID

		// 获取或创建会话
		var err error
		session, err = h.sessionHandler.GetOrCreateSession(sessionID, userID, configID, &req)
		if err != nil {
			logger.Warn("  Failed to get or create session: %v", err)
		} else {
			logger.Info("  Session: %s", session.ID)

			// 如果有 session ID，加载对话历史
			if sessionID != "" && len(req.Messages) > 0 {
				// Claude Code 客户端会自己管理对话历史，每次请求都包含完整消息
				// 服务器不需要追加数据库历史，否则会导致消息重复
				// 只保存新消息到数据库用于记录和审计
				logger.Info("  Using client-provided conversation history (%d messages)", len(req.Messages))
			}

			// 将 session ID 添加到响应 header
			c.Header("X-Session-ID", session.ID)
		}
	}

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
			ReasoningEffort: dbConfig.ReasoningEffort, // 传递思考级别配置
		}
		targetClient = client.NewOpenAIClient(targetConfig)
	} else {
		logger.Debug("  Using default config")
		targetConfig = h.config
		targetClient = h.client
	}

	// 转换请求
	logger.Debug("  Converting Claude request to OpenAI format...")
	logger.Debug("  Target config: BigModel=%s, MiddleModel=%s, SmallModel=%s, ReasoningEffort=%s",
		targetConfig.BigModel, targetConfig.MiddleModel, targetConfig.SmallModel, targetConfig.ReasoningEffort)

	openAIReq := converter.ConvertClaudeToOpenAIWithConfig(&req, targetConfig, nil)

	// 检查是否包含工具调用
	hasTools := len(req.Tools) > 0
	forceNonStream := hasTools && req.Stream
	if forceNonStream {
		logger.Info("  Request has tools with streaming, forcing non-stream upstream request")
		openAIReq.Stream = false
	}

	logger.Info("  Converted to OpenAI model: %s", openAIReq.Model)
	logger.Info("  OpenAI request: messages=%d, max_tokens=%d, stream=%v (original stream=%v), reasoning_effort=%s, tools=%v",
		len(openAIReq.Messages), openAIReq.MaxTokens, openAIReq.Stream, req.Stream, openAIReq.ReasoningEffort, hasTools)

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

	// 准备会话参数
	var sessionID string
	if session != nil {
		sessionID = session.ID
	}

	if req.Stream {
		if forceNonStream {
			// 工具+流式：使用非流式请求，但包装成SSE流返回
			logger.Info("  Using non-stream request with SSE wrapper for tool calls")
			h.handleNonStreamAsStream(c, targetClient, openAIReq, &req, configID, startTime, sessionID, betaHeaders)
		} else {
			h.responseHandler.HandleStreamingResponse(c, targetClient, openAIReq, &req, configID, startTime, h.sessionHandler, sessionID)
		}
	} else {
		h.responseHandler.HandleNonStreamingResponse(c, targetClient, openAIReq, &req, configID, startTime, h.sessionHandler, sessionID)
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

// handleNonStreamAsStream 将非流式响应包装成SSE流格式返回
// 用于处理工具调用+流式请求的场景
func (h *Handler) handleNonStreamAsStream(
	c *gin.Context,
	targetClient *client.OpenAIClient,
	openAIReq *models.OpenAIRequest,
	claudeReq *models.ClaudeMessagesRequest,
	configID string,
	startTime time.Time,
	sessionID string,
	betaHeaders []string,
) {
	logger := utils.GetLogger()

	// Set beta headers for upstream request
	targetClient.BetaHeaders = betaHeaders

	// 执行非流式请求
	openAIResp, err := targetClient.CreateChatCompletion(openAIReq)
	if err != nil {
		logger.Error("← [handleNonStreamAsStream] Request failed: %v", err)
		h.responseHandler.SendErrorResponse(c, err)
		h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
		return
	}

	logger.Info("  Response received, converting to Claude SSE format...")
	claudeResp := converter.ConvertOpenAIToClaudeResponse(openAIResp, claudeReq)
	if claudeResp == nil {
		err := fmt.Errorf("failed to convert OpenAI response to Claude format")
		h.responseHandler.SendErrorResponse(c, err)
		h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
		return
	}

	// 设置SSE头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// 生成消息ID
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	if claudeResp.ID != "" {
		msgID = claudeResp.ID
	}

	// 发送 message_start
	msgStart := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         claudeReq.Model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  claudeResp.Usage.InputTokens,
				"output_tokens": 0,
			},
		},
	}
	h.sendSSEEvent(c, flusher, "message_start", msgStart)

	// 发送 ping
	h.sendSSEEvent(c, flusher, "ping", map[string]string{"type": "ping"})

	// 发送内容块
	for i, block := range claudeResp.Content {
		idx := i
		switch block.Type {
		case "text":
			// content_block_start
			startEvt := map[string]interface{}{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			}
			h.sendSSEEvent(c, flusher, "content_block_start", startEvt)

			// content_block_delta
			deltaEvt := map[string]interface{}{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": block.Text,
				},
			}
			h.sendSSEEvent(c, flusher, "content_block_delta", deltaEvt)

			// content_block_stop
			stopEvt := map[string]interface{}{
				"type":  "content_block_stop",
				"index": idx,
			}
			h.sendSSEEvent(c, flusher, "content_block_stop", stopEvt)

		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			// content_block_start
			startEvt := map[string]interface{}{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]interface{}{
					"type":  "tool_use",
					"id":    block.ID,
					"name":  block.Name,
					"input": map[string]interface{}{},
				},
			}
			h.sendSSEEvent(c, flusher, "content_block_start", startEvt)

			// content_block_delta (input_json_delta)
			deltaEvt := map[string]interface{}{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": string(inputJSON),
				},
			}
			h.sendSSEEvent(c, flusher, "content_block_delta", deltaEvt)

			// content_block_stop
			stopEvt := map[string]interface{}{
				"type":  "content_block_stop",
				"index": idx,
			}
			h.sendSSEEvent(c, flusher, "content_block_stop", stopEvt)
		}
	}

	// 发送 message_delta
	msgDelta := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   claudeResp.StopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": claudeResp.Usage.OutputTokens,
		},
	}
	h.sendSSEEvent(c, flusher, "message_delta", msgDelta)

	// 发送 message_stop
	h.sendSSEEvent(c, flusher, "message_stop", map[string]string{"type": "message_stop"})

	// 发送 [DONE]
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()

	logger.Info("  SSE stream completed")

	// 记录日志
	h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model,
		claudeResp.Usage.InputTokens,
		claudeResp.Usage.OutputTokens,
		startTime, "success", "", claudeReq, claudeResp)

	// 保存消息到会话
	if h.sessionHandler != nil && sessionID != "" {
		assistantContent := ""
		if len(claudeResp.Content) > 0 {
			if claudeResp.Content[0].Type == "text" {
				assistantContent = claudeResp.Content[0].Text
			}
		}
		h.responseHandler.SaveMessagesToSession(h.sessionHandler, sessionID, claudeReq, assistantContent,
			claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens)
	}
}

// sendSSEEvent 发送SSE事件
func (h *Handler) sendSSEEvent(c *gin.Context, flusher http.Flusher, eventType string, data interface{}) {
	dataBytes, _ := json.Marshal(data)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, string(dataBytes))
	flusher.Flush()
}

// extractBetaHeaders 从请求中提取 anthropic-beta headers
func extractBetaHeaders(c *gin.Context) []string {
	return c.Request.Header.Values("anthropic-beta")
}

// isClaudeCode 检测是否为 Claude Code 客户端
func isClaudeCode(c *gin.Context) bool {
	userAgent := c.Request.UserAgent()
	return strings.Contains(userAgent, "Claude/") || strings.Contains(userAgent, "claude-code")
}

// appendDefaultBetaHeaders 为 Claude Code 客户端添加默认 beta headers
func appendDefaultBetaHeaders(existing []string) []string {
	defaultBetaHeaders := []string{
		"prompt-caching-2024-07-31",
		"max-tokens-3-5-sonnet-2024-07-15",
	}

	// 合并现有 headers 和默认 headers，避免重复
	headerSet := make(map[string]bool)
	for _, h := range existing {
		headerSet[h] = true
	}
	for _, h := range defaultBetaHeaders {
		if !headerSet[h] {
			existing = append(existing, h)
			headerSet[h] = true
		}
	}
	return existing
}
