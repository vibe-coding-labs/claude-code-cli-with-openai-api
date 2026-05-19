package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Tenant represents a tenant in the multi-tenancy system
type Tenant struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Status      string    `json:"status" db:"status"` // active, suspended, deleted
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	Metadata    string    `json:"metadata" db:"metadata"` // JSON blob for custom fields
}

// Validate validates the tenant data
func (t *Tenant) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if t.Name == "" {
		return fmt.Errorf("tenant name is required")
	}
	if t.Status != "active" && t.Status != "suspended" && t.Status != "deleted" {
		return fmt.Errorf("invalid tenant status: %s", t.Status)
	}
	// Validate metadata is valid JSON if present
	if t.Metadata != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(t.Metadata), &js); err != nil {
			return fmt.Errorf("invalid metadata JSON: %w", err)
		}
	}
	return nil
}

// TenantConfig represents tenant-specific configuration
type TenantConfig struct {
	TenantID         string         `json:"tenant_id" db:"tenant_id"`
	AllowedModels    []string       `json:"allowed_models" db:"allowed_models"`
	DefaultModel     string         `json:"default_model" db:"default_model"`
	CustomRateLimits bool           `json:"custom_rate_limits" db:"custom_rate_limits"`
	RequireHMAC      bool           `json:"require_hmac" db:"require_hmac"`
	WebhookURL       string         `json:"webhook_url" db:"webhook_url"`
	AlertEmail       string         `json:"alert_email" db:"alert_email"`
	CreatedAt        time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at" db:"updated_at"`
}

// Validate validates the tenant config data
func (tc *TenantConfig) Validate() error {
	if tc.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	return nil
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID         string     `json:"id" db:"id"`
	KeyHash    string     `json:"-" db:"key_hash"` // Hashed in database, not exposed in JSON
	TenantID   string     `json:"tenant_id" db:"tenant_id"`
	Name       string     `json:"name" db:"name"`
	Status     string     `json:"status" db:"status"` // active, expired, revoked
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at" db:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at" db:"last_used_at"`
	HMACSecret string     `json:"-" db:"hmac_secret"` // Not exposed in JSON
}

// Validate validates the API key data
func (ak *APIKey) Validate() error {
	if ak.ID == "" {
		return fmt.Errorf("API key ID is required")
	}
	if ak.KeyHash == "" {
		return fmt.Errorf("API key hash is required")
	}
	if ak.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if ak.Status != "active" && ak.Status != "expired" && ak.Status != "revoked" {
		return fmt.Errorf("invalid API key status: %s", ak.Status)
	}
	return nil
}

// IsExpired checks if the API key has expired
func (ak *APIKey) IsExpired() bool {
	if ak.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*ak.ExpiresAt)
}

// IsActive checks if the API key is active and not expired
func (ak *APIKey) IsActive() bool {
	return ak.Status == "active" && !ak.IsExpired()
}

// Quota represents usage quotas for a tenant
type Quota struct {
	ID           string    `json:"id" db:"id"`
	TenantID     string    `json:"tenant_id" db:"tenant_id"`
	QuotaType    string    `json:"quota_type" db:"quota_type"` // requests, tokens, cost
	Period       string    `json:"period" db:"period"`         // daily, monthly
	Limit        int64     `json:"limit" db:"limit"`
	CurrentUsage int64     `json:"current_usage" db:"current_usage"`
	ResetAt      time.Time `json:"reset_at" db:"reset_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// Validate validates the quota data
func (q *Quota) Validate() error {
	if q.ID == "" {
		return fmt.Errorf("quota ID is required")
	}
	if q.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if q.QuotaType != "requests" && q.QuotaType != "tokens" && q.QuotaType != "cost" {
		return fmt.Errorf("invalid quota type: %s", q.QuotaType)
	}
	if q.Period != "daily" && q.Period != "monthly" {
		return fmt.Errorf("invalid quota period: %s", q.Period)
	}
	if q.Limit < 0 {
		return fmt.Errorf("quota limit cannot be negative")
	}
	if q.CurrentUsage < 0 {
		return fmt.Errorf("current usage cannot be negative")
	}
	return nil
}

// IsExceeded checks if the quota has been exceeded
func (q *Quota) IsExceeded() bool {
	return q.CurrentUsage >= q.Limit
}

// RemainingQuota returns the remaining quota
func (q *Quota) RemainingQuota() int64 {
	remaining := q.Limit - q.CurrentUsage
	if remaining < 0 {
		return 0
	}
	return remaining
}

// PercentageUsed returns the percentage of quota used
func (q *Quota) PercentageUsed() float64 {
	if q.Limit == 0 {
		return 0
	}
	return float64(q.CurrentUsage) / float64(q.Limit) * 100
}

// RateLimit represents rate limiting configuration
type RateLimit struct {
	ID        string    `json:"id" db:"id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"` // Empty for global
	Dimension string    `json:"dimension" db:"dimension"`  // api_key, ip, tenant
	Algorithm string    `json:"algorithm" db:"algorithm"`  // fixed_window, sliding_window, token_bucket
	Limit     int       `json:"limit" db:"limit"`
	Window    int       `json:"window" db:"window"` // Seconds
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Validate validates the rate limit data
func (rl *RateLimit) Validate() error {
	if rl.ID == "" {
		return fmt.Errorf("rate limit ID is required")
	}
	if rl.Dimension != "api_key" && rl.Dimension != "ip" && rl.Dimension != "tenant" {
		return fmt.Errorf("invalid rate limit dimension: %s", rl.Dimension)
	}
	if rl.Algorithm != "fixed_window" && rl.Algorithm != "sliding_window" && rl.Algorithm != "token_bucket" {
		return fmt.Errorf("invalid rate limit algorithm: %s", rl.Algorithm)
	}
	if rl.Limit <= 0 {
		return fmt.Errorf("rate limit must be positive")
	}
	if rl.Window <= 0 {
		return fmt.Errorf("rate limit window must be positive")
	}
	return nil
}

// IPRule represents IP access control rules
type IPRule struct {
	ID          string    `json:"id" db:"id"`
	TenantID    string    `json:"tenant_id" db:"tenant_id"` // Empty for global
	RuleType    string    `json:"rule_type" db:"rule_type"` // whitelist, blacklist
	IPAddress   string    `json:"ip_address" db:"ip_address"` // Supports CIDR
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// Validate validates the IP rule data
func (ir *IPRule) Validate() error {
	if ir.ID == "" {
		return fmt.Errorf("IP rule ID is required")
	}
	if ir.RuleType != "whitelist" && ir.RuleType != "blacklist" {
		return fmt.Errorf("invalid IP rule type: %s", ir.RuleType)
	}
	if ir.IPAddress == "" {
		return fmt.Errorf("IP address is required")
	}
	return nil
}

// UsageRecord represents a usage record for tracking API usage
type UsageRecord struct {
	ID               string    `json:"id" db:"id"`
	TenantID         string    `json:"tenant_id" db:"tenant_id"`
	APIKeyID         string    `json:"api_key_id" db:"api_key_id"`
	Model            string    `json:"model" db:"model"`
	PromptTokens     int       `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens" db:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens" db:"total_tokens"`
	Cost             float64   `json:"cost" db:"cost"`
	ResponseTime     int       `json:"response_time" db:"response_time"` // Milliseconds
	StatusCode       int       `json:"status_code" db:"status_code"`
	Timestamp        time.Time `json:"timestamp" db:"timestamp"`
}

// Validate validates the usage record data
func (ur *UsageRecord) Validate() error {
	if ur.ID == "" {
		return fmt.Errorf("usage record ID is required")
	}
	if ur.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if ur.APIKeyID == "" {
		return fmt.Errorf("API key ID is required")
	}
	if ur.Model == "" {
		return fmt.Errorf("model is required")
	}
	if ur.PromptTokens < 0 || ur.CompletionTokens < 0 || ur.TotalTokens < 0 {
		return fmt.Errorf("token counts cannot be negative")
	}
	if ur.Cost < 0 {
		return fmt.Errorf("cost cannot be negative")
	}
	if ur.ResponseTime < 0 {
		return fmt.Errorf("response time cannot be negative")
	}
	return nil
}

// AuditEvent represents a security audit event
type AuditEvent struct {
	ID        string    `json:"id" db:"id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"` // NULL for system-level events
	EventType string    `json:"event_type" db:"event_type"`
	Actor     string    `json:"actor" db:"actor"`       // User or system component
	Resource  string    `json:"resource" db:"resource"` // What was affected
	Action    string    `json:"action" db:"action"`     // What happened
	Result    string    `json:"result" db:"result"`     // success, failure
	Details   string    `json:"details" db:"details"`   // JSON blob
	IPAddress string    `json:"ip_address" db:"ip_address"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

// Validate validates the audit event data
func (ae *AuditEvent) Validate() error {
	if ae.ID == "" {
		return fmt.Errorf("audit event ID is required")
	}
	if ae.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	if ae.Actor == "" {
		return fmt.Errorf("actor is required")
	}
	if ae.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	if ae.Action == "" {
		return fmt.Errorf("action is required")
	}
	if ae.Result != "success" && ae.Result != "failure" {
		return fmt.Errorf("invalid result: %s", ae.Result)
	}
	// Validate details is valid JSON if present
	if ae.Details != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(ae.Details), &js); err != nil {
			return fmt.Errorf("invalid details JSON: %w", err)
		}
	}
	return nil
}

// UsageStats represents aggregated usage statistics
type UsageStats struct {
	TenantID         string    `json:"tenant_id"`
	Period           string    `json:"period"` // daily, monthly
	TotalRequests    int64     `json:"total_requests"`
	TotalTokens      int64     `json:"total_tokens"`
	TotalCost        float64   `json:"total_cost"`
	AvgResponseTime  float64   `json:"avg_response_time"`
	ErrorRate        float64   `json:"error_rate"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
}

// BillingReport represents a billing report for a tenant
type BillingReport struct {
	TenantID      string         `json:"tenant_id"`
	Period        string         `json:"period"`
	StartDate     time.Time      `json:"start_date"`
	EndDate       time.Time      `json:"end_date"`
	TotalCost     float64        `json:"total_cost"`
	TotalRequests int64          `json:"total_requests"`
	TotalTokens   int64          `json:"total_tokens"`
	ModelBreakdown map[string]float64 `json:"model_breakdown"`
	GeneratedAt   time.Time      `json:"generated_at"`
}

// QuotaAlert represents a quota alert
type QuotaAlert struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AlertType   string    `json:"alert_type"` // quota_warning, quota_critical
	QuotaType   string    `json:"quota_type"` // requests, tokens, cost
	Threshold   float64   `json:"threshold"`  // Percentage
	CurrentUsage float64  `json:"current_usage"`
	Limit       int64     `json:"limit"`
	Message     string    `json:"message"`
	Sent        bool      `json:"sent"`
	SentAt      *time.Time `json:"sent_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// AlertConfig represents alert configuration for a tenant
type AlertConfig struct {
	TenantID          string  `json:"tenant_id"`
	WarningThreshold  float64 `json:"warning_threshold"`  // Default: 80%
	CriticalThreshold float64 `json:"critical_threshold"` // Default: 95%
	WebhookURL        string  `json:"webhook_url"`
	EmailAddress      string  `json:"email_address"`
	Enabled           bool    `json:"enabled"`
}

// PricingTier represents a pricing tier for billing
type PricingTier struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	PricePerToken   float64 `json:"price_per_token"`
	MinimumTokens   int64   `json:"minimum_tokens"`
	DiscountPercent float64 `json:"discount_percent"`
}

// TenantFilters represents filters for querying tenants
type TenantFilters struct {
	Status    string
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string
}

// AuditFilters represents filters for querying audit logs
type AuditFilters struct {
	TenantID  string
	EventType string
	Actor     string
	Result    string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// TimePeriod represents a time period for queries
type TimePeriod struct {
	Start time.Time
	End   time.Time
}

// Usage represents usage data for quota checking
type Usage struct {
	Requests int64
	Tokens   int64
	Cost     float64
}

// ExportFormat represents export format for billing reports
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatJSON ExportFormat = "json"
)

// QuotaType represents the type of quota
type QuotaType string

const (
	QuotaTypeRequests QuotaType = "requests"
	QuotaTypeTokens   QuotaType = "tokens"
	QuotaTypeCost     QuotaType = "cost"
)

// Helper function to convert sql.NullTime to *time.Time
func NullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

// Helper function to convert *time.Time to sql.NullTime
func PtrToNullTime(t *time.Time) sql.NullTime {
	if t != nil {
		return sql.NullTime{Time: *t, Valid: true}
	}
	return sql.NullTime{Valid: false}
}
