package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ClassifyOpenAIError provides specific error guidance for common OpenAI API issues
func ClassifyOpenAIError(errorDetail string) string {
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
	httpClient    *http.Client
}

func NewOpenAIClient(cfg *config.Config) *OpenAIClient {
	return &OpenAIClient{
		APIKey:        cfg.OpenAIAPIKey,
		BaseURL:       cfg.OpenAIBaseURL,
		Timeout:       time.Duration(cfg.RequestTimeout) * time.Second,
		CustomHeaders: cfg.CustomHeaders,
		APIVersion:    cfg.AzureAPIVersion,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
		},
	}
}

func (c *OpenAIClient) CreateChatCompletion(openAIReq *models.OpenAIRequest) (*models.OpenAIResponse, error) {
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.BaseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/") + "/chat/completions"
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := string(body)
		classifiedError := ClassifyOpenAIError(errorMsg)
		// Return error with status code information for proper error handling
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
	}

	var openAIResp models.OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &openAIResp, nil
}

func (c *OpenAIClient) CreateChatCompletionStream(openAIReq *models.OpenAIRequest) (io.ReadCloser, error) {
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.BaseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/") + "/chat/completions"
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		errorMsg := string(body)
		classifiedError := ClassifyOpenAIError(errorMsg)
		// Return error with status code information for proper error handling
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, classifiedError)
	}

	return resp.Body, nil
}
