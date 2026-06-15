package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/ratelimit"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/retry"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// ClassifyOpenAIError provides specific error guidance for common OpenAI API issues
func ClassifyOpenAIError(errorDetail string) string {
	// Handle empty error messages
	if strings.TrimSpace(errorDetail) == "" {
		return "Server error with no details provided. This may be a temporary issue, please retry."
	}

	errorStr := strings.ToLower(errorDetail)

	// Region/country restrictions
	if strings.Contains(errorStr, "unsupported_country_region_territory") ||
		strings.Contains(errorStr, "country, region, or territory not supported") {
		return "OpenAI API is not available in your region. Consider using a VPN or Azure OpenAI service."
	}

	// API key issues
	if strings.Contains(errorStr, "invalid_api_key") || strings.Contains(errorStr, "unauthorized") {
		return "Invalid API key. Please check your OPENAI_API_KEY configuration."
	}

	// Rate limiting
	if strings.Contains(errorStr, "rate_limit") || strings.Contains(errorStr, "quota") {
		return "Rate limit exceeded. Please wait and try again, or upgrade your API plan."
	}

	// Model not found / unknown aliased model (upstream model routing errors)
	if (strings.Contains(errorStr, "model") &&
		(strings.Contains(errorStr, "not found") || strings.Contains(errorStr, "does not exist"))) ||
		strings.Contains(errorStr, "unknown aliased model") ||
		strings.Contains(errorStr, "no available channel") {
		return "The requested model is temporarily unavailable. Please try again."
	}

	// Billing issues
	if strings.Contains(errorStr, "billing") || strings.Contains(errorStr, "payment") {
		return "Billing issue. Please check your OpenAI account billing status."
	}

	// Default: return original message
	return errorDetail
}


// IsModelRoutingError checks if an error message indicates a transient upstream
// model routing failure (e.g., "unknown aliased model", "no available channel").
// These errors should be returned as overloaded_error so Claude Code auto-retries.
func IsModelRoutingError(errorDetail string) bool {
	errorStr := strings.ToLower(errorDetail)
	return strings.Contains(errorStr, "unknown aliased model") ||
		strings.Contains(errorStr, "no available channel") ||
		(strings.Contains(errorStr, "model") &&
			(strings.Contains(errorStr, "not found") || strings.Contains(errorStr, "does not exist")))
}
// IsRetryableError delegates to the retry package for consistent error classification.
func IsRetryableError(err error) bool {
	return retry.IsRetryable(err)
}

// Retry configuration constants (kept for client inline retry loops)
const (
	DefaultRetryCount = 20
	MinRetryCount     = 3
	MaxRetryCount     = 50
)

// CalculateBackoff computes exponential backoff using the retry package strategy
func (c *OpenAIClient) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	backoff := c.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
	if backoff > c.RetryBackoffMax {
		backoff = c.RetryBackoffMax
	}
	return backoff
}

func normalizeToolCallIDsForRetry(openAIReq *models.OpenAIRequest, errorBody string) (*models.OpenAIRequest, bool) {
	if openAIReq == nil {
		return openAIReq, false
	}

	missingToolCallID := extractNoToolOutputCallID(errorBody)
	if missingToolCallID == "" {
		return openAIReq, false
	}

	assistantToolCallIDs := make(map[string]struct{})
	for _, msg := range openAIReq.Messages {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.ID != "" {
				assistantToolCallIDs[tc.ID] = struct{}{}
			}
		}
	}

	if _, ok := assistantToolCallIDs[missingToolCallID]; !ok {
		return openAIReq, false
	}

	var toolMsgIdxs []int
	hasTargetToolMessage := false
	for i, msg := range openAIReq.Messages {
		if msg.Role != "tool" || msg.ToolCallID == "" {
			continue
		}
		toolMsgIdxs = append(toolMsgIdxs, i)
		if msg.ToolCallID == missingToolCallID {
			hasTargetToolMessage = true
		}
	}

	if len(toolMsgIdxs) == 0 || hasTargetToolMessage {
		return openAIReq, false
	}

	targetCanonicalID := canonicalToolCallID(missingToolCallID)
	var candidateIdxs []int
	for _, idx := range toolMsgIdxs {
		if canonicalToolCallID(openAIReq.Messages[idx].ToolCallID) == targetCanonicalID {
			candidateIdxs = append(candidateIdxs, idx)
		}
	}

	if len(candidateIdxs) == 0 {
		if len(toolMsgIdxs) == 1 {
			candidateIdxs = append(candidateIdxs, toolMsgIdxs[0])
		} else {
			var unmatchedIdxs []int
			for _, idx := range toolMsgIdxs {
				if _, ok := assistantToolCallIDs[openAIReq.Messages[idx].ToolCallID]; !ok {
					unmatchedIdxs = append(unmatchedIdxs, idx)
				}
			}
			if len(unmatchedIdxs) == 1 {
				candidateIdxs = append(candidateIdxs, unmatchedIdxs[0])
			}
		}
	}

	if len(candidateIdxs) == 0 {
		return openAIReq, false
	}

	rewritten := *openAIReq
	rewritten.Messages = make([]models.OpenAIMessage, len(openAIReq.Messages))
	copy(rewritten.Messages, openAIReq.Messages)
	for _, idx := range candidateIdxs {
		rewritten.Messages[idx].ToolCallID = missingToolCallID
	}

	return &rewritten, true
}

func extractNoToolOutputCallID(errorBody string) string {
	const marker = "No tool output found for function call "
	idx := strings.Index(errorBody, marker)
	if idx < 0 {
		return ""
	}

	remainder := errorBody[idx+len(marker):]
	remainder = strings.TrimLeft(remainder, "\"'`")
	if remainder == "" {
		return ""
	}

	end := len(remainder)
	for i, r := range remainder {
		switch r {
		case ' ', '.', ',', '"', '\'', '\\', '\n', '\r', '\t', '}', ']', ')':
			end = i
			goto done
		}
	}

done:
	return strings.TrimSpace(remainder[:end])
}

func canonicalToolCallID(id string) string {
	normalized := strings.TrimSpace(id)
	if normalized == "" {
		return ""
	}

	if strings.HasPrefix(normalized, "call_") {
		normalized = "fc_" + strings.TrimPrefix(normalized, "call_")
	}
	if strings.HasPrefix(normalized, "fc_") {
		normalized = "fc" + strings.TrimPrefix(normalized, "fc_")
	}

	normalized = strings.ReplaceAll(normalized, "_", "")
	return strings.ToLower(normalized)
}

func isRetryableHTTPStatus(statusCode int, errorBody string) bool {
	if statusCode == 429 {
		cat := retry.ClassifyError(fmt.Errorf("status 429: %s", errorBody)); return cat == retry.CategoryRateLimit
	}

	switch statusCode {
	case 408, 406, 502, 503, 504, 506, 507, 508, 509, 510, 511:
		return true
	}

	return statusCode >= 500
}

// isRetryableModelRoutingError checks if a 400 error is a transient upstream
// model routing failure that should be retried rather than propagated immediately.
func isRetryableModelRoutingError(errorBody string) bool {
	retryablePatterns := []string{
		"unknown aliased model",
		"model not found",
		"no available channel",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errorBody, pattern) {
			return true
		}
	}
	return false
}

type OpenAIClient struct {
	ConfigID         string // Database config ID for error tracking
	ConfigName       string // Database config name for error tracking
	RequestID        string // Unique request ID for log correlation
	SessionID        string // Session ID for log correlation
	APIKey           string
	BaseURL          string
	Timeout          time.Duration
	CustomHeaders    map[string]string
	BetaHeaders      []string // Beta headers for upstream API (e.g., anthropic-beta)
	APIVersion       string
	RetryCount       int           // 重试次数
	RetryBackoffBase time.Duration // 指数退避基数
	RetryBackoffMax  time.Duration // 指数退避最大上限
	httpClient       *http.Client
}

func NewOpenAIClient(cfg *config.Config) *OpenAIClient {
	retryCount := cfg.RetryCount
	if retryCount < MinRetryCount {
		retryCount = DefaultRetryCount
	}
	if retryCount > MaxRetryCount {
		retryCount = MaxRetryCount
	}

	// Set default retry backoff values
	retryBackoffBase := cfg.RetryBackoffBase
	if retryBackoffBase <= 0 {
		retryBackoffBase = 1.0
	}
	retryBackoffMax := cfg.RetryBackoffMax
	if retryBackoffMax <= 0 {
		retryBackoffMax = 60
	}

		// Configure proxy: per-config proxy_url takes priority
		var proxyFunc func(*http.Request) (*url.URL, error)
		if cfg.ProxyURL != "" {
			parsed, err := url.Parse(cfg.ProxyURL)
			if err != nil {
				proxyFunc = http.ProxyFromEnvironment
			} else {
				proxyFunc = http.ProxyURL(parsed)
			}
		} else {
			proxyFunc = http.ProxyFromEnvironment
		}

		transport := &http.Transport{
			Proxy:                 proxyFunc,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			MaxConnsPerHost:       0,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout: 10 * time.Second,
			// Bound the header wait by the same per-request budget (RequestTimeout,
			// default 300s) instead of a hard 30s. Large-context requests (e.g. a
			// 1000+ message tool-use turn) legitimately need >30s for the upstream to
			// process and emit its first byte; the old 30s cap aborted them with
			// "timeout awaiting response headers" → retry loop → 503. The overall
			// request is still bounded by the per-request context deadline and the
			// stream stall detector (StreamStallTimeout).
			ResponseHeaderTimeout: time.Duration(cfg.RequestTimeout) * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

	return &OpenAIClient{
		ConfigID:         cfg.ConfigID,
		ConfigName:       cfg.ConfigName,
		APIKey:           cfg.OpenAIAPIKey,
		BaseURL:          cfg.OpenAIBaseURL,
		Timeout:          time.Duration(cfg.RequestTimeout) * time.Second,
		CustomHeaders:    cfg.CustomHeaders,
		APIVersion:       cfg.AzureAPIVersion,
		RetryCount:       retryCount,
		RetryBackoffBase: time.Duration(retryBackoffBase * float64(time.Second)),
		RetryBackoffMax:  time.Duration(retryBackoffMax) * time.Second,
		httpClient: &http.Client{
			// Don't set a global timeout - we'll handle timeouts per-request
			// to avoid timing out during long response body reads
			Timeout:   0,
			Transport: transport,
		},
	}
}


// saveDebugRequest saves the failed request body to a file for offline analysis
func saveDebugRequest(model string, reqBody []byte, upstreamError string) {
	debugDir := filepath.Join(".", "data", "debug")
	os.MkdirAll(debugDir, 0755)
	timestamp := time.Now().Format("20060102-150405")
	filename := filepath.Join(debugDir, fmt.Sprintf("error-%s-%s.json", model, timestamp))

	// Build a debug envelope with context
	envelope := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"model":         model,
		"upstream_error": upstreamError,
		"request_body":  json.RawMessage(reqBody),
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		utils.GetLogger().Warn("[debug] Failed to marshal debug envelope: %v", err)
		return
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		utils.GetLogger().Warn("[debug] Failed to write debug file: %v", err)
		return
	}
	utils.GetLogger().Error("[debug] Saved failed request to %s (model=%s, %d bytes)", filename, model, len(reqBody))

	// Also log the full request body for immediate debugging
	msgRoles := make([]string, 0)
	var parsed struct {
		Messages []struct {
			Role      string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct{} `json:"tool_calls"`
		} `json:"messages"`
	}
	if json.Unmarshal(reqBody, &parsed) == nil {
		for i, m := range parsed.Messages {
			r := fmt.Sprintf("%d:%s", i, m.Role)
			if m.ToolCallID != "" {
				r += "(tid=" + m.ToolCallID + ")"
			}
			if len(m.ToolCalls) > 0 {
				r += fmt.Sprintf("(tc=%d)", len(m.ToolCalls))
			}
			msgRoles = append(msgRoles, r)
		}
	}
	utils.GetLogger().Error("[debug] Failed request message sequence: %v", msgRoles)
}


// classifyError determines the error category based on the error context
func classifyError(statusCode int, err error) string {
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "context deadline") {
			return database.ErrorCategoryTimeout
		}
		if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "TLS handshake") || strings.Contains(errStr, "network") {
			return database.ErrorCategoryNetwork
		}
		return database.ErrorCategoryNetwork
	}
	if statusCode == 401 || statusCode == 403 {
		return database.ErrorCategoryAuth
	}
	if statusCode == 429 {
		return database.ErrorCategoryRateLimit
	}
	if statusCode >= 500 {
		return database.ErrorCategoryUpstream
	}
	if statusCode >= 400 {
		return database.ErrorCategoryProtocol
	}
	return database.ErrorCategoryUnknown
}

// logProxyError records a proxy error to the database with full context.
func (c *OpenAIClient) logProxyError(model, upstreamModel string, statusCode int, err error, errorMsg, upstreamBody, stage string, attempt int, durationMs int64, reqPreview string) {
	if c.ConfigID == "" {
		return // Skip if no config context (global client)
	}

	category := classifyError(statusCode, err)
	errType := "http_error"
	if err != nil {
		errType = "request_error"
	}

	proxyErr := &database.ProxyError{
		ConfigID:           c.ConfigID,
		ConfigName:         c.ConfigName,
		SessionID:          c.SessionID,
		RequestID:          c.RequestID,
		Model:              model,
		UpstreamModel:      upstreamModel,
		ErrorType:          errType,
		ErrorCategory:      category,
		ErrorMessage:       errorMsg,
		UpstreamStatusCode: statusCode,
		UpstreamErrorBody:  upstreamBody,
		RequestStage:       stage,
		RetryAttempt:       attempt,
		RequestDurationMs:  durationMs,
		RequestPreview:     reqPreview,
	}

	if dbErr := database.LogProxyError(proxyErr); dbErr != nil {
		utils.GetLogger().Warn("[proxy-error] Failed to log proxy error to database: %v", dbErr)
	}
}

func (c *OpenAIClient) CreateChatCompletion(openAIReq *models.OpenAIRequest) (*models.OpenAIResponse, error) {
	logger := utils.GetLogger()
	startTime := time.Now()
	deadline := startTime.Add(c.Timeout)

	logger.Info("→ [OpenAIClient] Creating chat completion (non-streaming)")
	logger.Debug("  Model: %s", openAIReq.Model)
	logger.Debug("  Messages: %d", len(openAIReq.Messages))
	logger.Debug("  MaxTokens: %d", openAIReq.MaxTokens)
	logger.Debug("  Retry count: %d", c.RetryCount)

	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		logger.Error("← [OpenAIClient] Failed to marshal request: %v", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	logger.Debug("  Request body size: %d bytes", len(reqBody))
	logger.Debug("  Request body: %s", string(reqBody))

	url := c.BaseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/") + "/chat/completions"
	}
	logger.Debug("  Target URL: %s", url)

	// Retry logic with exponential backoff
	var lastErr error
	var currentCancel context.CancelFunc // track the latest context cancel for cleanup
		rateLimit429Count := 0 // Track 429-specific retries separately
	for attempt := 0; attempt <= c.RetryCount; attempt++ {
		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			logger.Error("← [OpenAIClient] Request timeout exceeded, aborting retries")
			return nil, fmt.Errorf("request timeout exceeded after %d attempts", attempt)
		}

		if attempt > 0 {
			// Calculate exponential backoff with cap at MaxBackoffDelay (1 minute)
			backoffDuration := c.CalculateBackoff(attempt)

			// Don't wait if it would exceed the deadline
			if time.Now().Add(backoffDuration).After(deadline) {
				logger.Error("← [OpenAIClient] Insufficient time for backoff, aborting retries")
				break
			}
			logger.Info("  ⏱️  Retry attempt %d/%d after %v backoff", attempt, c.RetryCount, backoffDuration)
				time.Sleep(backoffDuration)
		}

		// Create context with timeout for this attempt
		// Use 2x the configured timeout to allow for response body reading
		ctx, cancel := context.WithTimeout(context.Background(), c.Timeout*2)
		currentCancel = cancel

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)

		// Add Azure API version if set
		if c.APIVersion != "" {
			q := req.URL.Query()
			q.Add("api-version", c.APIVersion)
			req.URL.RawQuery = q.Encode()
		}

		// Add custom headers
		for key, value := range c.CustomHeaders {
			req.Header.Set(key, value)
		}

		// Add beta headers (e.g., anthropic-beta)
		for _, betaHeader := range c.BetaHeaders {
			req.Header.Add("anthropic-beta", betaHeader)
		}

		if attempt == 0 {
			logger.Debug("  Sending request to OpenAI...")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			logger.Warn("← [OpenAIClient] Request failed (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)
			c.logProxyError(openAIReq.Model, "", 0, err, err.Error(), "", database.StageRequest, attempt, time.Since(startTime).Milliseconds(), "")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errorMsg := string(body)

			// Provide more context when body is empty
			if strings.TrimSpace(errorMsg) == "" {
				errorMsg = fmt.Sprintf("HTTP %d error with no response body", resp.StatusCode)
			}

			// Log upstream error with full context for debugging
			logger.LogUpstreamError("openai", url, openAIReq.Model, resp.StatusCode, errorMsg)

			if resp.StatusCode == http.StatusBadRequest {
				if rewrittenReq, changed := normalizeToolCallIDsForRetry(openAIReq, errorMsg); changed {
					logger.Warn("  Detected tool_call_id mismatch, retrying once with normalized tool_call_id")
					openAIReq = rewrittenReq
					reqBody, err = json.Marshal(openAIReq)
					if err != nil {
						logger.Error("← [OpenAIClient] Failed to marshal normalized request: %v", err)
						return nil, fmt.Errorf("failed to marshal normalized request: %w", err)
					}
					lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, errorMsg)
					cancel()
					continue
				}
				// Retry on transient upstream routing errors (e.g., "unknown aliased model")
				if isRetryableModelRoutingError(errorMsg) && attempt < c.RetryCount {
					logger.Warn("  Transient upstream routing error (attempt %d/%d): %s", attempt+1, c.RetryCount+1, errorMsg)
					lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, errorMsg)
					cancel()
					continue
				}
			}

			classifiedError := ClassifyOpenAIError(errorMsg)

			// Check if error is retryable.
			// 429 is conditionally retryable: transient rate limits yes, quota/billing hard limits no.
			isRetryable := isRetryableHTTPStatus(resp.StatusCode, errorMsg)

			// For 429: smart multi-attempt exponential backoff
			// 429 is much more likely to succeed with patient retries than other errors.
			// We don't consume the main retry budget for 429s — they have their own
			// budget via rateLimit429Count, capped at Default429Config().MaxAttempts.
			if resp.StatusCode == 429 {
				rateLimit429Count++
				ratelimit429Cfg := ratelimit.Default429Config()

				// Log every 429 to database for observability
				c.logProxyError(openAIReq.Model, openAIReq.Model, resp.StatusCode, nil,
					fmt.Sprintf("429 rate limit (attempt %d): %s", rateLimit429Count, classifiedError),
					errorMsg, database.StageRequest, attempt, time.Since(startTime).Milliseconds(),
					string(reqBody[:min(500, len(reqBody))]))

				// Safety cap: if we've hit too many 429s, give up to prevent infinite loops
				if rateLimit429Count > ratelimit429Cfg.MaxAttempts {
					logger.Warn("← [OpenAIClient] Exceeded %d 429 retries, giving up", ratelimit429Cfg.MaxAttempts)
					return nil, fmt.Errorf("429 rate limit: exhausted %d retries", ratelimit429Cfg.MaxAttempts)
				}

				waitResult, waitErr := ratelimit.Global.WaitWith429Backoff(ctx, c.ConfigID, errorMsg, ratelimit429Cfg)
				if waitErr != nil {
					return nil, fmt.Errorf("cancelled while waiting for 429 backoff: %w", waitErr)
				}
				// Quota exhausted — no point retrying
				if waitResult.Severity == ratelimit.SeverityQuotaExhausted {
					logger.Warn("← [OpenAIClient] 429 quota exhausted, not retrying: %s", waitResult.Reason)
					return nil, fmt.Errorf("429 rate limit (quota exhausted): %s", waitResult.Reason)
				}
				if waitResult.Aborted {
					logger.Warn("← [OpenAIClient] 429 backoff aborted after %v wait: %s", waitResult.TotalWaited.Round(time.Second), waitResult.Reason)
					return nil, fmt.Errorf("429 rate limit: %s (waited %v)", waitResult.Reason, waitResult.TotalWaited.Round(time.Second))
				}
				logger.Warn("← [OpenAIClient] 429 rate limit, smart backoff waited %v (429_retry=%d/%d, config=%s, severity=%d, upstream_retry_after=%v)",
					waitResult.TotalWaited.Round(time.Second), rateLimit429Count, ratelimit429Cfg.MaxAttempts,
					c.ConfigID, waitResult.Severity, waitResult.UpstreamRetryAfter.Round(time.Second))
				lastErr = fmt.Errorf("OpenAI API error (status 429): %s", classifiedError)
				// Don't advance attempt counter — 429 has its own budget.
				attempt--
				cancel()
				continue
			}
			if !isRetryable || attempt >= c.RetryCount {
				logger.Error("← [OpenAIClient] OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
				if len(body) > 0 {
					logger.Debug("  Raw error: %s", string(body))
				}
				// Save the failed request body for debugging protocol issues
				saveDebugRequest(openAIReq.Model, reqBody, errorMsg)
				c.logProxyError(openAIReq.Model, "", resp.StatusCode, nil, classifiedError, errorMsg, database.StageRequest, attempt, time.Since(startTime).Milliseconds(), string(reqBody[:min(500, len(reqBody))]))
				return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
			}

			lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
			logger.Warn("← [OpenAIClient] Retryable error (attempt %d/%d, status %d): %s", attempt+1, c.RetryCount+1, resp.StatusCode, classifiedError)
			cancel()
			continue
		}

		// Success!
		// Read raw response body for debugging
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		logger.Info("  Raw response (first 500 chars): %s", string(bodyBytes[:min(500, len(bodyBytes))]))
		// Restore body for JSON decoder
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		var openAIResp models.OpenAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
			resp.Body.Close()
			logger.Warn("← [OpenAIClient] Failed to decode response (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)

			// Treat decode errors as retryable (could be partial/corrupted response)
			if attempt < c.RetryCount {
				lastErr = fmt.Errorf("failed to decode response: %w", err)
				logger.Info("  Retrying due to decode error...")
				cancel()
				continue
			}

			logger.Error("← [OpenAIClient] Failed to decode response after %d attempts", c.RetryCount+1)
				c.logProxyError(openAIReq.Model, "", 0, err,
					fmt.Sprintf("decode error after %d attempts: %v", c.RetryCount+1, err),
					"", database.StageResponse, attempt, time.Since(startTime).Milliseconds(),
					string(reqBody[:min(500, len(reqBody))]))
			return nil, fmt.Errorf("failed to decode response after %d attempts: %w", c.RetryCount+1, err)
		}

		// Check if response has valid choices
		if len(openAIResp.Choices) == 0 {
			logger.Warn("← [OpenAIClient] API returned empty choices (attempt %d/%d)", attempt+1, c.RetryCount+1)
			logger.Debug("  Response body: ID=%s, Model=%s, Usage=%+v", openAIResp.ID, openAIResp.Model, openAIResp.Usage)

			// Log the full response for debugging
			respJSON, _ := json.Marshal(openAIResp)
			logger.Warn("  Full response JSON: %s", string(respJSON))

			// Check for finish_reason that might explain empty choices
			if openAIResp.Error != nil {
				logger.Warn("  API Error in response: %+v", openAIResp.Error)
			}

			// Treat empty choices as retryable error
			if attempt < c.RetryCount {
				lastErr = fmt.Errorf("API returned empty choices")
				logger.Info("  Retrying due to empty response...")
				cancel()
				continue
			}

			// Last attempt, return error with more context
			logger.Error("← [OpenAIClient] API consistently returns empty choices after %d attempts", c.RetryCount+1)
				c.logProxyError(openAIReq.Model, openAIResp.Model, 0, nil,
					fmt.Sprintf("empty choices after %d attempts. Response ID: %s", c.RetryCount+1, openAIResp.ID),
					"", database.StageResponse, attempt, time.Since(startTime).Milliseconds(),
					string(reqBody[:min(500, len(reqBody))]))
			errorMsg := fmt.Sprintf("API returned empty choices after %d attempts. Response ID: %s, Model: %s",
				c.RetryCount+1, openAIResp.ID, openAIResp.Model)
			if openAIResp.Error != nil {
				errorMsg += fmt.Sprintf(", API Error: %v", openAIResp.Error)
			}
			return nil, errors.New(errorMsg)
		}

		logger.Info("← [OpenAIClient] Chat completion successful (took %v)", time.Since(startTime))
		logger.Debug("  Response tokens: %+v", openAIResp.Usage)

		cancel()
		return &openAIResp, nil
	}

	// All retries failed — clean up any outstanding context
	if currentCancel != nil {
		currentCancel()
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("all retry attempts failed")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *OpenAIClient) CreateChatCompletionStream(openAIReq *models.OpenAIRequest) (io.ReadCloser, error) {
	logger := utils.GetLogger()
	startTime := time.Now()
	deadline := startTime.Add(c.Timeout)

	logger.Info("→ [OpenAIClient] Creating chat completion (streaming)")
	logger.Debug("  Model: %s", openAIReq.Model)
	logger.Debug("  Messages: %d", len(openAIReq.Messages))
	logger.Debug("  Retry count: %d", c.RetryCount)

	// Ensure stream is enabled
	openAIReq.Stream = true
	// Add stream options to include usage information
	if openAIReq.StreamOptions == nil {
		openAIReq.StreamOptions = &models.StreamOptions{
			IncludeUsage: true,
		}
	} else {
		openAIReq.StreamOptions.IncludeUsage = true
	}

	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		logger.Error("← [OpenAIClient] Failed to marshal request: %v", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	logger.Debug("  Request body size: %d bytes", len(reqBody))

	url := c.BaseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/") + "/chat/completions"
	}
	logger.Debug("  Target URL: %s", url)

	// Retry logic for streaming requests
	var lastErr error
		rateLimit429Count := 0 // Track 429-specific retries separately
	for attempt := 0; attempt <= c.RetryCount; attempt++ {
		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			logger.Error("← [OpenAIClient] Request timeout exceeded, aborting retries")
			return nil, fmt.Errorf("request timeout exceeded after %d attempts", attempt)
		}

		if attempt > 0 {
			// Calculate exponential backoff with cap at MaxBackoffDelay (1 minute)
			backoffDuration := c.CalculateBackoff(attempt)

			// Don't wait if it would exceed the deadline
			if time.Now().Add(backoffDuration).After(deadline) {
				logger.Error("← [OpenAIClient] Insufficient time for backoff, aborting retries")
				break
			}
			logger.Info("  ⏱️  Retry attempt %d/%d after %v backoff", attempt, c.RetryCount, backoffDuration)
				time.Sleep(backoffDuration)
		}

		// For streaming, don't set timeout on context as response body will be read over time
		// Only use deadline check for connection establishment
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
		if err != nil {
			logger.Error("← [OpenAIClient] Failed to create request: %v", err)
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "text/event-stream")

		// Add Azure API version if set
		if c.APIVersion != "" {
			q := req.URL.Query()
			q.Add("api-version", c.APIVersion)
			req.URL.RawQuery = q.Encode()
		}

		// Add custom headers
		for key, value := range c.CustomHeaders {
			req.Header.Set(key, value)
		}

		// Add beta headers (e.g., anthropic-beta)
		for _, betaHeader := range c.BetaHeaders {
			req.Header.Add("anthropic-beta", betaHeader)
		}

		if attempt == 0 {
			logger.Debug("  Sending streaming request to OpenAI...")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logProxyError(openAIReq.Model, "", 0, err, err.Error(), "", database.StageStreaming, attempt, time.Since(startTime).Milliseconds(), "")
			logger.Warn("← [OpenAIClient] Request failed (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)
			continue
		}
		logger.Debug("  Response status: %d", resp.StatusCode)

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errorMsg := string(body)

			// Provide more context when body is empty
				if strings.TrimSpace(errorMsg) == "" {
					errorMsg = fmt.Sprintf("HTTP %d error with no response body", resp.StatusCode)
				}

				// Log upstream error with full context for debugging
				logger.LogUpstreamError("openai-stream", url, openAIReq.Model, resp.StatusCode, errorMsg)

				// Handle 400 Bad Request with potential retryable routing errors
				if resp.StatusCode == http.StatusBadRequest {
					// Retry on transient upstream routing errors (e.g., "unknown aliased model")
					if isRetryableModelRoutingError(errorMsg) && attempt < c.RetryCount {
						logger.Warn("  Transient upstream routing error in streaming (attempt %d/%d): %s", attempt+1, c.RetryCount+1, errorMsg)
						lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, errorMsg)
						continue
					}
				}

				classifiedError := ClassifyOpenAIError(errorMsg)

				// Check if error is retryable.
				// 429 is conditionally retryable: transient rate limits yes, quota/billing hard limits no.
				isRetryable := isRetryableHTTPStatus(resp.StatusCode, errorMsg)

			// For 429: smart multi-attempt exponential backoff
			// Same strategy as non-streaming: 429s don't consume main retry budget.
			// Uses rateLimit429Count to cap 429-specific retries.
			if resp.StatusCode == 429 {
				rateLimit429Count++
				ratelimit429Cfg := ratelimit.Default429Config()

				// Log every 429 to database for observability
				c.logProxyError(openAIReq.Model, openAIReq.Model, resp.StatusCode, nil,
					fmt.Sprintf("429 stream rate limit (attempt %d): %s", rateLimit429Count, classifiedError),
					errorMsg, database.StageStreaming, attempt, time.Since(startTime).Milliseconds(),
					string(reqBody[:min(500, len(reqBody))]))

				if rateLimit429Count > ratelimit429Cfg.MaxAttempts {
					logger.Warn("← [OpenAIClient] Exceeded %d 429 stream retries, giving up", ratelimit429Cfg.MaxAttempts)
					return nil, fmt.Errorf("429 rate limit: exhausted %d retries", ratelimit429Cfg.MaxAttempts)
				}

				ratelimit429Ctx, ratelimit429Cancel := context.WithTimeout(context.Background(), time.Until(deadline))
				defer ratelimit429Cancel()

				waitResult, waitErr := ratelimit.Global.WaitWith429Backoff(ratelimit429Ctx, c.ConfigID, errorMsg, ratelimit429Cfg)
				if waitErr != nil {
					return nil, fmt.Errorf("cancelled while waiting for 429 backoff: %w", waitErr)
				}
				if waitResult.Severity == ratelimit.SeverityQuotaExhausted {
					logger.Warn("← [OpenAIClient] 429 stream quota exhausted, not retrying: %s", waitResult.Reason)
					return nil, fmt.Errorf("429 rate limit (quota exhausted): %s", waitResult.Reason)
				}
				if waitResult.Aborted {
					logger.Warn("← [OpenAIClient] 429 backoff aborted after %v wait: %s", waitResult.TotalWaited.Round(time.Second), waitResult.Reason)
					return nil, fmt.Errorf("429 rate limit: %s (waited %v)", waitResult.Reason, waitResult.TotalWaited.Round(time.Second))
				}
				logger.Warn("← [OpenAIClient] 429 stream rate limit, smart backoff waited %v (429_retry=%d/%d, config=%s, severity=%d, upstream_retry_after=%v)",
					waitResult.TotalWaited.Round(time.Second), rateLimit429Count, ratelimit429Cfg.MaxAttempts,
					c.ConfigID, waitResult.Severity, waitResult.UpstreamRetryAfter.Round(time.Second))
				lastErr = fmt.Errorf("OpenAI API error (status 429): %s", classifiedError)
				attempt--
				continue
			}
			if !isRetryable || attempt >= c.RetryCount {
				logger.Error("← [OpenAIClient] OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
				if len(body) > 0 {
					logger.Debug("  Raw error: %s", string(body))
				}
				// Return error with status code information for proper error handling
				// Save the failed request body for debugging protocol issues
				saveDebugRequest(openAIReq.Model, reqBody, errorMsg)
				c.logProxyError(openAIReq.Model, "", resp.StatusCode, nil, classifiedError, errorMsg, database.StageStreaming, attempt, time.Since(startTime).Milliseconds(), string(reqBody[:min(500, len(reqBody))]))
				return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
			}

			lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
			logger.Warn("← [OpenAIClient] Retryable error (attempt %d/%d, status %d): %s", attempt+1, c.RetryCount+1, resp.StatusCode, classifiedError)
			continue
		}

		// Success!
		logger.Info("← [OpenAIClient] Streaming response started (took %v)", time.Since(startTime))
		return resp.Body, nil
	}

	// All retries failed
	if lastErr != nil {
		return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("all retry attempts failed")
}
