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

// LoadBalancer represents a load balancer configuration
type LoadBalancer struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Strategy        string       `json:"strategy"` // round_robin, random, weighted, least_connections
	ConfigNodes     []ConfigNode `json:"config_nodes"`
	ConfigNodesJSON string       `json:"-"` // JSON string for database storage
	Enabled         bool         `json:"enabled"`
	AnthropicAPIKey string       `json:"anthropic_api_key,omitempty"`
	
	// Health check configuration
	HealthCheckEnabled  bool `json:"health_check_enabled"`
	HealthCheckInterval int  `json:"health_check_interval"` // seconds
	FailureThreshold    int  `json:"failure_threshold"`
	RecoveryThreshold   int  `json:"recovery_threshold"`
	HealthCheckTimeout  int  `json:"health_check_timeout"` // seconds
	
	// Retry configuration
	MaxRetries        int `json:"max_retries"`
	InitialRetryDelay int `json:"initial_retry_delay"` // milliseconds
	MaxRetryDelay     int `json:"max_retry_delay"`     // milliseconds
	
	// Circuit breaker configuration
	CircuitBreakerEnabled bool    `json:"circuit_breaker_enabled"`
	ErrorRateThreshold    float64 `json:"error_rate_threshold"` // 0.0-1.0
	CircuitBreakerWindow  int     `json:"circuit_breaker_window"`  // seconds
	CircuitBreakerTimeout int     `json:"circuit_breaker_timeout"` // seconds
	HalfOpenRequests      int     `json:"half_open_requests"`
	
	// Dynamic weight configuration
	DynamicWeightEnabled bool `json:"dynamic_weight_enabled"`
	WeightUpdateInterval int  `json:"weight_update_interval"` // seconds
	
	// Log configuration
	LogLevel string `json:"log_level"` // minimal, standard, detailed
	
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// ConfigNode represents a configuration node in the load balancer
type ConfigNode struct {
	ConfigID string `json:"config_id"`
	Weight   int    `json:"weight"` // Weight for weighted strategy (1-100)
	Enabled  bool   `json:"enabled"`
}

// HealthStatus represents the health status of a configuration node
type HealthStatus struct {
	ConfigID             string    `json:"config_id"`
	Status               string    `json:"status"` // healthy, unhealthy, unknown
	LastCheckTime        time.Time `json:"last_check_time"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	ConsecutiveFailures  int       `json:"consecutive_failures"`
	LastError            string    `json:"last_error,omitempty"`
	ResponseTimeMs       int       `json:"response_time_ms"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// CircuitBreakerState represents the state of a circuit breaker for a configuration node
type CircuitBreakerState struct {
	ConfigID        string    `json:"config_id"`
	State           string    `json:"state"` // closed, open, half_open
	FailureCount    int       `json:"failure_count"`
	SuccessCount    int       `json:"success_count"`
	LastStateChange time.Time `json:"last_state_change"`
	NextRetryTime   *time.Time `json:"next_retry_time,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// LoadBalancerRequestLog represents a request log for load balancer
type LoadBalancerRequestLog struct {
	ID               string    `json:"id"`
	LoadBalancerID   string    `json:"load_balancer_id"`
	SelectedConfigID string    `json:"selected_config_id"`
	RequestTime      time.Time `json:"request_time"`
	ResponseTime     time.Time `json:"response_time"`
	DurationMs       int       `json:"duration_ms"`
	StatusCode       int       `json:"status_code"`
	Success          bool      `json:"success"`
	RetryCount       int       `json:"retry_count"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	RequestSummary   string    `json:"request_summary,omitempty"`
	ResponsePreview  string    `json:"response_preview,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// LoadBalancerStats represents aggregated statistics for a load balancer
type LoadBalancerStats struct {
	LoadBalancerID    string      `json:"load_balancer_id"`
	TimeWindow        string      `json:"time_window"` // 1h, 24h, 7d, 30d
	TotalRequests     int64       `json:"total_requests"`
	SuccessRequests   int64       `json:"success_requests"`
	FailedRequests    int64       `json:"failed_requests"`
	AvgResponseTimeMs float64     `json:"avg_response_time_ms"`
	P50ResponseTimeMs int         `json:"p50_response_time_ms"`
	P95ResponseTimeMs int         `json:"p95_response_time_ms"`
	P99ResponseTimeMs int         `json:"p99_response_time_ms"`
	ErrorRate         float64     `json:"error_rate"`
	ActiveConnections int         `json:"active_connections"`
	NodeStats         []NodeStats `json:"node_stats"`
}

// NodeStats represents statistics for a configuration node in a load balancer
type NodeStats struct {
	ConfigID            string  `json:"config_id"`
	ConfigName          string  `json:"config_name"`
	HealthStatus        string  `json:"health_status"`
	CircuitBreakerState string  `json:"circuit_breaker_state"`
	RequestCount        int64   `json:"request_count"`
	SuccessRate         float64 `json:"success_rate"`
	AvgResponseTimeMs   float64 `json:"avg_response_time_ms"`
	CurrentWeight       int     `json:"current_weight"`
	BaseWeight          int     `json:"base_weight"`
}

// Alert represents an alert for a load balancer
type Alert struct {
	ID              string     `json:"id"`
	LoadBalancerID  string     `json:"load_balancer_id"`
	Level           string     `json:"level"` // critical, warning, info
	Type            string     `json:"type"`  // all_nodes_down, low_healthy_nodes, high_error_rate, circuit_breaker_open
	Message         string     `json:"message"`
	Details         string     `json:"details,omitempty"`
	Acknowledged    bool       `json:"acknowledged"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// LoadBalancerConfig extends LoadBalancer with enhancement configuration
type LoadBalancerConfig struct {
	LoadBalancer
	// Health check configuration
	HealthCheckEnabled  bool `json:"health_check_enabled"`
	HealthCheckInterval int  `json:"health_check_interval"` // seconds
	FailureThreshold    int  `json:"failure_threshold"`
	RecoveryThreshold   int  `json:"recovery_threshold"`
	HealthCheckTimeout  int  `json:"health_check_timeout"` // seconds
	
	// Retry configuration
	MaxRetries        int `json:"max_retries"`
	InitialRetryDelay int `json:"initial_retry_delay"` // milliseconds
	MaxRetryDelay     int `json:"max_retry_delay"`     // milliseconds
	
	// Circuit breaker configuration
	CircuitBreakerEnabled bool    `json:"circuit_breaker_enabled"`
	ErrorRateThreshold    float64 `json:"error_rate_threshold"` // 0.0-1.0
	CircuitBreakerWindow  int     `json:"circuit_breaker_window"`  // seconds
	CircuitBreakerTimeout int     `json:"circuit_breaker_timeout"` // seconds
	HalfOpenRequests      int     `json:"half_open_requests"`
	
	// Dynamic weight configuration
	DynamicWeightEnabled bool `json:"dynamic_weight_enabled"`
	WeightUpdateInterval int  `json:"weight_update_interval"` // seconds
	
	// Log configuration
	LogLevel string `json:"log_level"` // minimal, standard, detailed
}

// RealTimeMetrics represents real-time metrics for a load balancer
type RealTimeMetrics struct {
	LoadBalancerID      string               `json:"load_balancer_id"`
	Timestamp           time.Time            `json:"timestamp"`
	RequestsPerSecond   float64              `json:"requests_per_second"`
	SuccessRate         float64              `json:"success_rate"`
	AvgResponseTimeMs   float64              `json:"avg_response_time_ms"`
	ActiveConnections   int                  `json:"active_connections"`
	HealthyNodes        int                  `json:"healthy_nodes"`
	TotalNodes          int                  `json:"total_nodes"`
	TotalRequests       int64                `json:"total_requests"`
	SuccessRequests     int64                `json:"success_requests"`
	FailedRequests      int64                `json:"failed_requests"`
	NodeMetrics         []NodeRealTimeMetrics `json:"node_metrics"`
}

// NodeRealTimeMetrics represents real-time metrics for a node
type NodeRealTimeMetrics struct {
	ConfigID            string    `json:"config_id"`
	ConfigName          string    `json:"config_name"`
	HealthStatus        string    `json:"health_status"`
	CircuitBreakerState string    `json:"circuit_breaker_state"`
	RequestsPerSecond   float64   `json:"requests_per_second"`
	SuccessRate         float64   `json:"success_rate"`
	AvgResponseTimeMs   float64   `json:"avg_response_time_ms"`
	LastRequestTime     *time.Time `json:"last_request_time,omitempty"`
}
