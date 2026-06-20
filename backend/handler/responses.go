package handler

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/converter"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/retry"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// CreateResponse handles POST /v1/responses (OpenAI Responses API), the ingress
// used by Codex CLI. It mirrors CreateMessage's config resolution: resolve the
// client API key to a config (load balancer first, then single config), then
// delegate to executeResponsesRequestWithConfig, which translates Responses ->
// Chat Completions upstream and back, reusing the proxy's retry + stall
// detection machinery.
func (h *Handler) CreateResponse(c *gin.Context) {
	logger := utils.GetLogger()
	startTime := time.Now()

	logger.Debug("→ [CreateResponse] %s %s remote=%s", c.Request.Method, c.Request.URL.String(), c.ClientIP())

	apiKey := h.authHandler.extractAPIKey(c)
	if apiKey == "" {
		logger.Warn("← [CreateResponse] Missing API key")
		utils.GetLogger().LogResponse(http.StatusUnauthorized, time.Since(startTime))
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type":    "authentication_error",
			"code":    "missing_api_key",
			"message": "Missing API key. Provide a Bearer token.",
		}})
		return
	}

	// Try a load balancer keyed on this API key first.
	if loadBalancer, lbErr := database.GetLoadBalancerByAPIKey(apiKey); lbErr == nil && loadBalancer != nil {
		logger.Info("  Found load balancer by API key: %s (%s)", loadBalancer.ID, loadBalancer.Name)
		if !loadBalancer.Enabled {
			utils.GetLogger().LogResponse(http.StatusForbidden, time.Since(startTime))
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"type": "permission_error", "message": "This load balancer is currently disabled.",
			}})
			return
		}
		lbManager, err := h.lbRegistry.GetManager(loadBalancer.ID)
		if err != nil {
			logger.Error("← [CreateResponse] Failed to get load balancer manager: %v", err)
			utils.GetLogger().LogResponse(http.StatusInternalServerError, time.Since(startTime))
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"type": "internal_error", "message": "Failed to initialize load balancer",
			}})
			return
		}
		var lastErr error
		executed := false
		_ = lbManager.ExecuteRequest(func(cfg *database.APIConfig) error {
			if executed {
				logger.Info("  Retrying with different config, previous error: %v", lastErr)
			}
			executed = true
			if loadBalancer.Strategy == "least_connections" {
				defer lbManager.GetSelector().ReleaseConnection(cfg.ID)
			}
			if err := h.executeResponsesRequestWithConfig(c, cfg); err != nil {
				lastErr = err
				return err
			}
			return nil
		})
		return
	}

	// Otherwise resolve a single config.
	dbConfig, err := database.GetConfigByAnthropicAPIKey(apiKey)
	if err != nil || dbConfig == nil {
		logger.Warn("← [CreateResponse] Invalid API key: %s", maskAPIKey(apiKey))
		utils.GetLogger().LogResponse(http.StatusUnauthorized, time.Since(startTime))
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type": "authentication_error", "message": "Invalid API key.",
		}})
		return
	}
	logger.Info("  Found config by API key: %s (%s)", dbConfig.ID, dbConfig.Name)
	if !dbConfig.Enabled {
		utils.GetLogger().LogResponse(http.StatusForbidden, time.Since(startTime))
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"type": "permission_error", "message": "This configuration is currently disabled.",
		}})
		return
	}

	if err := h.executeResponsesRequestWithConfig(c, dbConfig); err != nil {
		utils.GetLogger().Error("← [CreateResponse] %v", err)
	}
}

// executeResponsesRequestWithConfig is the Responses equivalent of
// executeMessageRequestWithConfig. It reads the raw Responses body, translates
// it to a Chat Completions upstream request, and translates the response back.
// It reuses client.NewOpenAIClient, the retry engine, and WaitForFirstData
// stall-detection. There is intentionally no Claude request validator, no
// session saving, and no beta-header handling — those are Claude-Code-specific.
func (h *Handler) executeResponsesRequestWithConfig(c *gin.Context, dbConfig *database.APIConfig) error {
	logger := utils.GetLogger()
	startTime := time.Now()
	logger.Info("→ [executeResponsesRequestWithConfig] config=%s", dbConfig.ID)

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": fmt.Sprintf("Failed to read request body: %v", err),
		}})
		return fmt.Errorf("read body error: %w", err)
	}

	openAIReq, reqBody, err := converter.ConvertResponsesToOpenAIRequest(rawBody)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": err.Error(),
		}})
		return fmt.Errorf("responses parse error: %w", err)
	}

	// Build config + client from the DB config (model is passed through as-is;
	// Responses carries the real model name, no big/middle/small mapping).
	c.Set("user_id", dbConfig.UserID)
	targetConfig := &config.Config{
		ConfigID:           dbConfig.ID,
		ConfigName:         dbConfig.Name,
		OpenAIBaseURL:      dbConfig.OpenAIBaseURL,
		OpenAIAPIKey:       dbConfig.OpenAIAPIKey,
		BigModel:           dbConfig.BigModel,
		MiddleModel:        dbConfig.MiddleModel,
		SmallModel:         dbConfig.SmallModel,
		MaxTokensLimit:     dbConfig.MaxTokensLimit,
		RequestTimeout:     h.config.RequestTimeout,
		RetryCount:         dbConfig.RetryCount,
		RetryBackoffBase:   dbConfig.RetryBackoffBase,
		RetryBackoffMax:    dbConfig.RetryBackoffMax,
		AnthropicAPIKey:    dbConfig.AnthropicAPIKey,
		ReasoningEffort:    dbConfig.ReasoningEffort,
		ProxyURL:           dbConfig.ProxyURL,
		StreamStallTimeout: dbConfig.StreamStallTimeout,
		CustomHeaders:      dbConfig.CustomHeaders,
	}
	targetClient := client.NewOpenAIClient(targetConfig)

	configID := dbConfig.ID
	logger.Info("  Responses request: model=%s, messages=%d, stream=%v, tools=%v",
		openAIReq.Model, len(openAIReq.Messages), openAIReq.Stream, len(openAIReq.Tools) > 0)

	if c.Request.Context().Err() != nil {
		return fmt.Errorf("client disconnected before request")
	}

	if openAIReq.Stream {
		return h.executeResponsesStream(c, targetClient, targetConfig, openAIReq, reqBody, configID, startTime)
	}
	return h.executeResponsesNonStream(c, targetClient, openAIReq, reqBody, configID, startTime)
}

// executeResponsesNonStream performs a non-streaming Responses request.
func (h *Handler) executeResponsesNonStream(c *gin.Context, targetClient *client.OpenAIClient, openAIReq *models.OpenAIRequest, reqBody map[string]interface{}, configID string, startTime time.Time) error {
	logger := utils.GetLogger()

	openAIResp, err := targetClient.CreateChatCompletion(openAIReq)
	if err != nil {
		logger.Error("← [executeResponsesNonStream] request failed: %v", err)
		h.responseHandler.SendErrorResponse(c, err)
		h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), nil, nil)
		return fmt.Errorf("request failed: %w", err)
	}

	responsesObj := converter.ConvertOpenAIResponseToResponses(openAIResp, openAIReq.Model, reqBody)
	h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model,
		openAIResp.Usage.PromptTokens, openAIResp.Usage.CompletionTokens,
		startTime, "success", "", nil, nil)

	c.JSON(http.StatusOK, responsesObj)
	logger.Info("  Responses (non-stream) sent successfully")
	return nil
}

// executeResponsesStream performs a streaming Responses request with the same
// pre-stream stall-detection + transparent recreate-on-stall retry as the
// Claude path (handler.go:524-603).
func (h *Handler) executeResponsesStream(c *gin.Context, targetClient *client.OpenAIClient, targetConfig *config.Config, openAIReq *models.OpenAIRequest, reqBody map[string]interface{}, configID string, startTime time.Time) error {
	logger := utils.GetLogger()

	var reader io.ReadCloser
	createResult := retry.NewEngine().Execute(c.Request.Context(), func() error {
		var err error
		reader, err = targetClient.CreateChatCompletionStream(openAIReq)
		return err
	})
	if !createResult.Succeeded {
		logger.Error("← [executeResponsesStream] stream creation failed after %d attempts: %v",
			createResult.Attempts, createResult.LastErr)
		h.responseHandler.SendErrorResponse(c, createResult.LastErr)
		h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", createResult.LastErr.Error(), nil, nil)
		return fmt.Errorf("stream creation failed after %d attempts: %w", createResult.Attempts, createResult.LastErr)
	}

	stallTimeout := time.Duration(targetConfig.StreamStallTimeout) * time.Second
	if stallTimeout <= 0 {
		stallTimeout = 60 * time.Second
	}
	maxStallRetries := 3

	var streamResult *converter.StreamingResult
	for stallRetry := 0; stallRetry <= maxStallRetries; stallRetry++ {
		stallResult := WaitForFirstData(c.Request.Context(), reader, stallTimeout)
		if stallResult.Err != nil {
			reader.Close()
			if stallResult.Err == ErrUpstreamStalled {
				if stallRetry < maxStallRetries {
					logger.Warn("  [responses stall-retry] %d/%d upstream stalled, retrying...", stallRetry+1, maxStallRetries)
					createResult := retry.NewEngine().Execute(c.Request.Context(), func() error {
						var err error
						reader, err = targetClient.CreateChatCompletionStream(openAIReq)
						return err
					})
					if !createResult.Succeeded {
						c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
							"type":    "overloaded_error",
							"message": fmt.Sprintf("Upstream provider unresponsive after %d retries.", stallRetry+1),
						}})
						h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", "upstream_stalled_after_retries", nil, nil)
						return fmt.Errorf("upstream stalled and recreation failed after %d retries", stallRetry+1)
					}
					continue
				}
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
					"type":    "overloaded_error",
					"message": fmt.Sprintf("Upstream provider unresponsive after %d retries.", maxStallRetries),
				}})
				h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", "upstream_stalled_after_retries", nil, nil)
				return fmt.Errorf("upstream stalled after %d retries", maxStallRetries)
			}
			if c.Request.Context().Err() != nil {
				return fmt.Errorf("client disconnected during stall check: %w", stallResult.Err)
			}
			logger.Error("  [responses stall-retry] read error during pre-stream check: %v", stallResult.Err)
			h.responseHandler.SendErrorResponse(c, stallResult.Err)
			return stallResult.Err
		}

		defer reader.Close()
		logger.Info("  Responses stream verified (stall retries: %d)", stallRetry)
		streamResult = converter.ConvertOpenAIStreamingToResponses(c, stallResult.Reader, openAIReq.Model, reqBody, stallTimeout)
		break
	}

	if streamResult != nil {
		h.responseHandler.logRequestWithStreamingDetails(c, configID, openAIReq.Model, streamResult, startTime, "success", "", nil)
	} else {
		logger.Warn("  Responses stream ended without result for config %s (client disconnected or terminal error)", configID)
	}
	return nil
}
