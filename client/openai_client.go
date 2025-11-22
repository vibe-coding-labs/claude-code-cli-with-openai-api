package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
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

	// Model not found
	if strings.Contains(errorStr, "model") &&
		(strings.Contains(errorStr, "not found") || strings.Contains(errorStr, "does not exist")) {
		return "Model not found. Please check your BIG_MODEL and SMALL_MODEL configuration."
	}

	// Billing issues
	if strings.Contains(errorStr, "billing") || strings.Contains(errorStr, "payment") {
		return "Billing issue. Please check your OpenAI account billing status."
	}

	// Default: return original message
	return errorDetail
}

type OpenAIClient struct {
	APIKey        string
	BaseURL       string
	Timeout       time.Duration
	CustomHeaders map[string]string
	APIVersion    string
	RetryCount    int // 重试次数
	httpClient    *http.Client
}

func NewOpenAIClient(cfg *config.Config) *OpenAIClient {
	retryCount := cfg.RetryCount
	if retryCount <= 0 {
		retryCount = 3
	}
	if retryCount > 100 {
		retryCount = 100
	}

	// Create HTTP client with optimized transport for connection pooling
	transport := &http.Transport{
		MaxIdleConns:          100,              // Maximum idle connections across all hosts
		MaxIdleConnsPerHost:   100,              // Maximum idle connections per host
		MaxConnsPerHost:       0,                // No limit on total connections per host
		IdleConnTimeout:       90 * time.Second, // How long an idle connection remains open
		DisableKeepAlives:     false,            // Enable HTTP keep-alive
		DisableCompression:    false,            // Enable compression
		ForceAttemptHTTP2:     true,             // Try HTTP/2 when possible
		TLSHandshakeTimeout:   10 * time.Second, // TLS handshake timeout
		ResponseHeaderTimeout: 30 * time.Second, // Wait for response headers
		ExpectContinueTimeout: 1 * time.Second,  // Expect: 100-continue timeout
		// Note: No DialContext timeout - let individual requests control their timeouts
	}

	return &OpenAIClient{
		APIKey:        cfg.OpenAIAPIKey,
		BaseURL:       cfg.OpenAIBaseURL,
		Timeout:       time.Duration(cfg.RequestTimeout) * time.Second,
		CustomHeaders: cfg.CustomHeaders,
		APIVersion:    cfg.AzureAPIVersion,
		RetryCount:    retryCount,
		httpClient: &http.Client{
			// Don't set a global timeout - we'll handle timeouts per-request
			// to avoid timing out during long response body reads
			Timeout:   0,
			Transport: transport,
		},
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

	url := c.BaseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/") + "/chat/completions"
	}
	logger.Debug("  Target URL: %s", url)

	// Retry logic with exponential backoff
	var lastErr error
	for attempt := 0; attempt <= c.RetryCount; attempt++ {
		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			logger.Error("← [OpenAIClient] Request timeout exceeded, aborting retries")
			return nil, fmt.Errorf("request timeout exceeded after %d attempts", attempt)
		}

		if attempt > 0 {
			// Calculate exponential backoff with longer delays for unstable APIs
			// Base delay: 3 seconds, exponential growth: 3s, 6s, 12s, 24s, 48s...
			// Maximum delay: 30 seconds to avoid excessive waiting
			baseDelay := 3 * time.Second
			backoffDuration := time.Duration(1<<uint(attempt-1)) * baseDelay
			maxBackoff := 30 * time.Second
			if backoffDuration > maxBackoff {
				backoffDuration = maxBackoff
			}

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
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
		if err != nil {
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

		if attempt == 0 {
			logger.Debug("  Sending request to OpenAI...")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			logger.Warn("← [OpenAIClient] Request failed (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errorMsg := string(body)

			// Provide more context when body is empty
			if strings.TrimSpace(errorMsg) == "" {
				errorMsg = fmt.Sprintf("HTTP %d error with no response body", resp.StatusCode)
			}

			classifiedError := ClassifyOpenAIError(errorMsg)

			// Check if error is retryable
			// Retryable: 5xx, 429 (rate limit), 408 (timeout), 406 (not acceptable), 502-504 (gateway errors)
			isRetryable := resp.StatusCode >= 500 ||
				resp.StatusCode == 429 ||
				resp.StatusCode == 408 ||
				resp.StatusCode == 406 ||
				resp.StatusCode == 502 ||
				resp.StatusCode == 503 ||
				resp.StatusCode == 504

			if !isRetryable || attempt >= c.RetryCount {
				logger.Error("← [OpenAIClient] OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
				if len(body) > 0 {
					logger.Debug("  Raw error: %s", string(body))
				}
				return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
			}

			lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
			logger.Warn("← [OpenAIClient] Retryable error (attempt %d/%d, status %d): %s", attempt+1, c.RetryCount+1, resp.StatusCode, classifiedError)
			continue
		}

		// Success!
		var openAIResp models.OpenAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
			resp.Body.Close()
			logger.Warn("← [OpenAIClient] Failed to decode response (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)

			// Treat decode errors as retryable (could be partial/corrupted response)
			if attempt < c.RetryCount {
				lastErr = fmt.Errorf("failed to decode response: %w", err)
				logger.Info("  Retrying due to decode error...")
				continue
			}

			logger.Error("← [OpenAIClient] Failed to decode response after %d attempts", c.RetryCount+1)
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
				continue
			}

			// Last attempt, return error with more context
			logger.Error("← [OpenAIClient] API consistently returns empty choices after %d attempts", c.RetryCount+1)
			errorMsg := fmt.Sprintf("API returned empty choices after %d attempts. Response ID: %s, Model: %s",
				c.RetryCount+1, openAIResp.ID, openAIResp.Model)
			if openAIResp.Error != nil {
				errorMsg += fmt.Sprintf(", API Error: %v", openAIResp.Error)
			}
			return nil, errors.New(errorMsg)
		}

		logger.Info("← [OpenAIClient] Chat completion successful (took %v)", time.Since(startTime))
		logger.Debug("  Response tokens: %+v", openAIResp.Usage)

		return &openAIResp, nil
	}

	// All retries failed
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
	for attempt := 0; attempt <= c.RetryCount; attempt++ {
		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			logger.Error("← [OpenAIClient] Request timeout exceeded, aborting retries")
			return nil, fmt.Errorf("request timeout exceeded after %d attempts", attempt)
		}

		if attempt > 0 {
			// Calculate exponential backoff with longer delays for unstable APIs
			// Base delay: 3 seconds, exponential growth: 3s, 6s, 12s, 24s, 48s...
			// Maximum delay: 30 seconds to avoid excessive waiting
			baseDelay := 3 * time.Second
			backoffDuration := time.Duration(1<<uint(attempt-1)) * baseDelay
			maxBackoff := 30 * time.Second
			if backoffDuration > maxBackoff {
				backoffDuration = maxBackoff
			}

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

		if attempt == 0 {
			logger.Debug("  Sending streaming request to OpenAI...")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
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

			classifiedError := ClassifyOpenAIError(errorMsg)

			// Check if error is retryable
			// Retryable: 5xx, 429 (rate limit), 408 (timeout), 406 (not acceptable), 502-504 (gateway errors)
			isRetryable := resp.StatusCode >= 500 ||
				resp.StatusCode == 429 ||
				resp.StatusCode == 408 ||
				resp.StatusCode == 406 ||
				resp.StatusCode == 502 ||
				resp.StatusCode == 503 ||
				resp.StatusCode == 504

			if !isRetryable || attempt >= c.RetryCount {
				logger.Error("← [OpenAIClient] OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
				if len(body) > 0 {
					logger.Debug("  Raw error: %s", string(body))
				}
				// Return error with status code information for proper error handling
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
