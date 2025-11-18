package models

import "time"

// APIConfig represents a single OpenAI API configuration
type APIConfig struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	OpenAIAPIKey  string            `json:"openai_api_key"`
	OpenAIBaseURL string            `json:"openai_base_url"`
	AzureAPIVersion string          `json:"azure_api_version,omitempty"`
	AnthropicAPIKey string          `json:"anthropic_api_key,omitempty"` // For Claude Code CLI authentication
	BigModel      string            `json:"big_model"`
	MiddleModel   string            `json:"middle_model"`
	SmallModel    string            `json:"small_model"`
	MaxTokensLimit int              `json:"max_tokens_limit"`
	MinTokensLimit int              `json:"min_tokens_limit"`
	RequestTimeout int              `json:"request_timeout"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastTestedAt  *time.Time        `json:"last_tested_at,omitempty"`
	LastTestStatus string           `json:"last_test_status,omitempty"` // "success", "failed", ""
	LastTestError  string           `json:"last_test_error,omitempty"`
}

// APIConfigList represents a collection of API configurations
type APIConfigList struct {
	Configs []APIConfig `json:"configs"`
	DefaultConfigID string `json:"default_config_id,omitempty"`
}

// APIConfigRequest represents a request to create or update an API config
type APIConfigRequest struct {
	Name          string            `json:"name" binding:"required"`
	Description   string            `json:"description,omitempty"`
	OpenAIAPIKey  string            `json:"openai_api_key" binding:"required"`
	OpenAIBaseURL string            `json:"openai_base_url"`
	AzureAPIVersion string          `json:"azure_api_version,omitempty"`
	AnthropicAPIKey string          `json:"anthropic_api_key,omitempty"`
	BigModel      string            `json:"big_model"`
	MiddleModel   string            `json:"middle_model"`
	SmallModel    string            `json:"small_model"`
	MaxTokensLimit int              `json:"max_tokens_limit"`
	MinTokensLimit int              `json:"min_tokens_limit"`
	RequestTimeout int              `json:"request_timeout"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	Enabled       bool              `json:"enabled"`
}

// TestConfigResponse represents the response from testing a configuration
type TestConfigResponse struct {
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	ModelUsed   string    `json:"model_used,omitempty"`
	ResponseID  string    `json:"response_id,omitempty"`
	Error       string    `json:"error,omitempty"`
	ErrorType   string    `json:"error_type,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Suggestions []string  `json:"suggestions,omitempty"`
}

// ClaudeConfigFormat represents the configuration in Claude Code CLI format
type ClaudeConfigFormat struct {
	ANTHROPIC_BASE_URL string `json:"ANTHROPIC_BASE_URL"`
	ANTHROPIC_API_KEY  string `json:"ANTHROPIC_API_KEY"`
	ConfigID           string `json:"config_id"`
	ConfigName         string `json:"config_name"`
}

