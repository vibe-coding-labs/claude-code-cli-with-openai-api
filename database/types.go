package database

import "time"

// APIConfig represents an API configuration
type APIConfig struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Description           string            `json:"description"`
	OpenAIAPIKey          string            `json:"openai_api_key,omitempty"`        // Decrypted, not stored
	OpenAIAPIKeyEncrypted string            `json:"-"`                               // Encrypted, for DB only
	OpenAIAPIKeyMasked    string            `json:"openai_api_key_masked,omitempty"` // Masked for display
	OpenAIBaseURL         string            `json:"openai_base_url"`
	BigModel              string            `json:"big_model"`
	MiddleModel           string            `json:"middle_model"`
	SmallModel            string            `json:"small_model"`
	SupportedModels       []string          `json:"supported_models,omitempty"` // 支持的模型列表
	ModelMappings         map[string]string `json:"model_mappings,omitempty"`   // 高级模型映射
	MaxTokensLimit        int               `json:"max_tokens_limit"`
	RequestTimeout        int               `json:"request_timeout"`
	RetryCount            int               `json:"retry_count"` // 重试次数，默认3次，最大100次
	AnthropicAPIKey       string            `json:"anthropic_api_key,omitempty"`
	CustomHeaders         map[string]string `json:"custom_headers,omitempty"` // 自定义请求头
	Enabled               bool              `json:"enabled"`
	ExpiresAt             *time.Time        `json:"expires_at,omitempty"` // API密钥过期时间（可选）
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// TokenStats represents token usage statistics
type TokenStats struct {
	ID           int64     `json:"id"`
	ConfigID     string    `json:"config_id"`
	Model        string    `json:"model"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	RequestCount int64     `json:"request_count"`
	ErrorCount   int64     `json:"error_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// RequestLog represents a single API request log
type RequestLog struct {
	ID              int64     `json:"id"`
	ConfigID        string    `json:"config_id"`
	Model           string    `json:"model"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	TotalTokens     int       `json:"total_tokens"`
	DurationMs      int       `json:"duration_ms"`
	Status          string    `json:"status"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	RequestBody     string    `json:"request_body,omitempty"`     // 原始请求体（JSON）
	ResponseBody    string    `json:"response_body,omitempty"`    // 原始响应体（JSON）
	RequestSummary  string    `json:"request_summary,omitempty"`  // 请求摘要（便于快速查看）
	ResponsePreview string    `json:"response_preview,omitempty"` // 响应预览（前500字符）
	ClientIP        string    `json:"client_ip,omitempty"`        // 客户端IP地址
	UserAgent       string    `json:"user_agent,omitempty"`       // 客户端User-Agent
	CreatedAt       time.Time `json:"created_at"`
}

// ActiveClient represents an active Claude Code client
type ActiveClient struct {
	ClientIP      string    `json:"client_ip"`
	UserAgent     string    `json:"user_agent"`
	LastRequestAt time.Time `json:"last_request_at"`
	RequestCount  int       `json:"request_count"`
	IsActive      bool      `json:"is_active"` // 最近5分钟内有请求
}

// ClientStats represents statistics about active clients for a config
type ClientStats struct {
	ConfigID         string         `json:"config_id"`
	ActiveClients    int            `json:"active_clients"`    // 活跃客户端数（最近5分钟，基于IP+UA）
	EstimatedClients int            `json:"estimated_clients"` // 估算的实际客户端数（考虑并发）
	TotalClients     int            `json:"total_clients"`     // 总客户端数（最近24小时）
	HasConcurrent    bool           `json:"has_concurrent"`    // 是否检测到并发连接
	Clients          []ActiveClient `json:"clients"`
	LastRequestAt    *time.Time     `json:"last_request_at,omitempty"` // 最后一次请求时间
}

// ConfigStats represents aggregated statistics for a config
type ConfigStats struct {
	ConfigID          string  `json:"config_id"`
	TotalRequests     int64   `json:"total_requests"`
	SuccessRequests   int64   `json:"success_requests"`
	ErrorRequests     int64   `json:"error_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
}
