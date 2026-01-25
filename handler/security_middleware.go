package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/security"
)

// SecurityMiddleware contains all security components
type SecurityMiddleware struct {
	tenantManager security.TenantManager
	apiKeyManager security.APIKeyManager
	rateLimiter   security.RateLimiter
	ipFilter      security.IPFilter
	hmacVerifier  security.HMACVerifier
	quotaManager  *security.QuotaManager
	auditLogger   security.AuditLogger
	enabled       bool
}

// NewSecurityMiddleware creates a new security middleware instance
func NewSecurityMiddleware(
	tenantManager security.TenantManager,
	apiKeyManager security.APIKeyManager,
	rateLimiter security.RateLimiter,
	ipFilter security.IPFilter,
	hmacVerifier security.HMACVerifier,
	quotaManager *security.QuotaManager,
	auditLogger security.AuditLogger,
) *SecurityMiddleware {
	return &SecurityMiddleware{
		tenantManager: tenantManager,
		apiKeyManager: apiKeyManager,
		rateLimiter:   rateLimiter,
		ipFilter:      ipFilter,
		hmacVerifier:  hmacVerifier,
		quotaManager:  quotaManager,
		auditLogger:   auditLogger,
		enabled:       true,
	}
}

// Enable enables the security middleware
func (sm *SecurityMiddleware) Enable() {
	sm.enabled = true
}

// Disable disables the security middleware
func (sm *SecurityMiddleware) Disable() {
	sm.enabled = false
}

// AuthenticationMiddleware validates API keys and extracts tenant context
func (sm *SecurityMiddleware) AuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Extract API key from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			sm.logAuthFailure(ctx, "", "unknown", c.ClientIP(), "Missing Authorization header")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Missing Authorization header",
			})
			c.Abort()
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			sm.logAuthFailure(ctx, "", "unknown", c.ClientIP(), "Invalid Authorization header format")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid Authorization header format",
			})
			c.Abort()
			return
		}

		apiKey := parts[1]

		// Validate API key
		validatedKey, err := sm.apiKeyManager.ValidateKey(ctx, apiKey)
		if err != nil {
			sm.logAuthFailure(ctx, "", apiKey, c.ClientIP(), fmt.Sprintf("Invalid API key: %v", err))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired API key",
			})
			c.Abort()
			return
		}

		// Store tenant context
		c.Set("tenant_id", validatedKey.TenantID)
		c.Set("api_key_id", validatedKey.ID)
		c.Set("api_key", validatedKey)

		c.Next()
	}
}

// IPFilterMiddleware checks IP whitelist/blacklist
func (sm *SecurityMiddleware) IPFilterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		clientIP := c.ClientIP()

		// Get tenant ID if available (may not be set yet if this runs before auth)
		tenantID, _ := c.Get("tenant_id")
		tenantIDStr := ""
		if tenantID != nil {
			tenantIDStr = tenantID.(string)
		}

		// Check IP filter
		allowed, err := sm.ipFilter.CheckIP(ctx, clientIP, tenantIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "IP filter check failed",
			})
			c.Abort()
			return
		}

		if !allowed {
			sm.logIPBlocked(ctx, tenantIDStr, clientIP, "IP address blocked by filter")
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied: IP address blocked",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitMiddleware enforces rate limits
func (sm *SecurityMiddleware) RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Get tenant ID and API key ID
		tenantID, _ := c.Get("tenant_id")
		apiKeyID, _ := c.Get("api_key_id")

		if tenantID == nil {
			// No tenant context, skip rate limiting
			c.Next()
			return
		}

		tenantIDStr := tenantID.(string)
		apiKeyIDStr := ""
		if apiKeyID != nil {
			apiKeyIDStr = apiKeyID.(string)
		}

		// Get rate limits for tenant
		limits, err := sm.rateLimiter.GetLimits(ctx, tenantIDStr)
		if err != nil {
			// Log error but don't block request
			fmt.Printf("Error getting rate limits: %v\n", err)
			c.Next()
			return
		}

		// Check each applicable rate limit
		for _, limit := range limits {
			var key string
			switch limit.Dimension {
			case "api_key":
				key = fmt.Sprintf("ratelimit:apikey:%s", apiKeyIDStr)
			case "ip":
				key = fmt.Sprintf("ratelimit:ip:%s", c.ClientIP())
			case "tenant":
				key = fmt.Sprintf("ratelimit:tenant:%s", tenantIDStr)
			default:
				continue
			}

			allowed, retryAfter, err := sm.rateLimiter.CheckLimit(ctx, key, limit)
			if err != nil {
				// Log error but don't block request
				fmt.Printf("Error checking rate limit: %v\n", err)
				continue
			}

			if !allowed {
				sm.logRateLimitViolation(ctx, tenantIDStr, apiKeyIDStr, c.ClientIP(),
					fmt.Sprintf("Rate limit exceeded: %s", limit.Dimension))

				c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "Rate limit exceeded",
					"retry_after": int(retryAfter.Seconds()),
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// HMACVerificationMiddleware validates HMAC signatures
func (sm *SecurityMiddleware) HMACVerificationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Get tenant ID
		tenantID, exists := c.Get("tenant_id")
		if !exists {
			c.Next()
			return
		}

		tenantIDStr := tenantID.(string)

		// Get tenant config to check if HMAC is required
		config, err := sm.tenantManager.GetTenantConfig(ctx, tenantIDStr)
		if err != nil || config == nil || !config.RequireHMAC {
			// HMAC not required for this tenant
			c.Next()
			return
		}

		// Get HMAC signature from header
		signature := c.GetHeader("X-HMAC-Signature")
		if signature == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "HMAC signature required but not provided",
			})
			c.Abort()
			return
		}

		// Verify signature
		valid, err := sm.hmacVerifier.VerifySignature(ctx, c.Request, signature, tenantIDStr)
		if err != nil || !valid {
			sm.logAuthFailure(ctx, tenantIDStr, "hmac", c.ClientIP(),
				fmt.Sprintf("HMAC verification failed: %v", err))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid HMAC signature",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// QuotaCheckMiddleware checks if tenant has sufficient quota
func (sm *SecurityMiddleware) QuotaCheckMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Get tenant ID
		tenantID, exists := c.Get("tenant_id")
		if !exists {
			c.Next()
			return
		}

		tenantIDStr := tenantID.(string)

		// Estimate usage for this request (will be updated after response)
		estimatedUsage := database.Usage{
			Requests: 1,
			Tokens:   1000, // Rough estimate
			Cost:     0.01, // Rough estimate
		}

		// Check quota
		allowed, quota, err := sm.quotaManager.CheckQuota(ctx, tenantIDStr, estimatedUsage)
		if err != nil {
			// Log error but don't block request
			fmt.Printf("Error checking quota: %v\n", err)
			c.Next()
			return
		}

		if !allowed {
			sm.logQuotaExceeded(ctx, tenantIDStr, "", c.ClientIP(),
				fmt.Sprintf("Quota exceeded: %s", quota.QuotaType))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Quota exceeded",
				"quota_type":  quota.QuotaType,
				"limit":       quota.Limit,
				"usage":       quota.CurrentUsage,
				"reset_at":    quota.ResetAt.Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// UsageTrackingMiddleware records usage after successful requests
func (sm *SecurityMiddleware) UsageTrackingMiddleware(usageTracker *security.UsageTracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.enabled {
			c.Next()
			return
		}

		// Record start time
		startTime := time.Now()

		// Process request
		c.Next()

		// Only record usage for successful requests
		if c.Writer.Status() >= 400 {
			return
		}

		// Get tenant and API key info
		tenantID, _ := c.Get("tenant_id")
		apiKeyID, _ := c.Get("api_key_id")

		if tenantID == nil || apiKeyID == nil {
			return
		}

		// Get usage info from response (would be set by proxy handler)
		promptTokens, _ := c.Get("prompt_tokens")
		completionTokens, _ := c.Get("completion_tokens")
		totalTokens, _ := c.Get("total_tokens")
		cost, _ := c.Get("cost")
		model, _ := c.Get("model")

		// Create usage record
		record := &database.UsageRecord{
			TenantID:         tenantID.(string),
			APIKeyID:         apiKeyID.(string),
			Model:            getStringOrDefault(model, "unknown"),
			PromptTokens:     getIntOrDefault(promptTokens, 0),
			CompletionTokens: getIntOrDefault(completionTokens, 0),
			TotalTokens:      getIntOrDefault(totalTokens, 0),
			Cost:             getFloat64OrDefault(cost, 0.0),
			ResponseTime:     int(time.Since(startTime).Milliseconds()),
			StatusCode:       c.Writer.Status(),
		}

		// Record usage asynchronously
		ctx := context.Background()
		if err := usageTracker.RecordUsage(ctx, record); err != nil {
			fmt.Printf("Error recording usage: %v\n", err)
		}

		// Increment quota
		usage := database.Usage{
			Requests: 1,
			Tokens:   int64(record.TotalTokens),
			Cost:     record.Cost,
		}
		if err := sm.quotaManager.IncrementUsage(ctx, tenantID.(string), usage); err != nil {
			fmt.Printf("Error incrementing quota: %v\n", err)
		}
	}
}

// Helper functions

func (sm *SecurityMiddleware) logAuthFailure(ctx context.Context, tenantID, actor, ipAddress, message string) {
	if sm.auditLogger != nil {
		event := security.NewAuditEvent(
			tenantID,
			"authentication",
			actor,
			"api_key",
			"validate",
			"failure",
			map[string]string{"message": message},
			ipAddress,
		)
		sm.auditLogger.LogEvent(ctx, event)
	}
}

func (sm *SecurityMiddleware) logIPBlocked(ctx context.Context, tenantID, ipAddress, message string) {
	if sm.auditLogger != nil {
		event := security.NewAuditEvent(
			tenantID,
			"ip_filter",
			"system",
			"ip_address",
			"block",
			"blocked",
			map[string]string{"message": message},
			ipAddress,
		)
		sm.auditLogger.LogEvent(ctx, event)
	}
}

func (sm *SecurityMiddleware) logRateLimitViolation(ctx context.Context, tenantID, apiKeyID, ipAddress, message string) {
	if sm.auditLogger != nil {
		event := security.NewAuditEvent(
			tenantID,
			"rate_limit",
			apiKeyID,
			"rate_limit",
			"check",
			"exceeded",
			map[string]string{"message": message},
			ipAddress,
		)
		sm.auditLogger.LogEvent(ctx, event)
	}
}

func (sm *SecurityMiddleware) logQuotaExceeded(ctx context.Context, tenantID, apiKeyID, ipAddress, message string) {
	if sm.auditLogger != nil {
		event := security.NewAuditEvent(
			tenantID,
			"quota",
			apiKeyID,
			"quota",
			"check",
			"exceeded",
			map[string]string{"message": message},
			ipAddress,
		)
		sm.auditLogger.LogEvent(ctx, event)
	}
}

func getStringOrDefault(value interface{}, defaultValue string) string {
	if value == nil {
		return defaultValue
	}
	if str, ok := value.(string); ok {
		return str
	}
	return defaultValue
}

func getIntOrDefault(value interface{}, defaultValue int) int {
	if value == nil {
		return defaultValue
	}
	if i, ok := value.(int); ok {
		return i
	}
	return defaultValue
}

func getFloat64OrDefault(value interface{}, defaultValue float64) float64 {
	if value == nil {
		return defaultValue
	}
	if f, ok := value.(float64); ok {
		return f
	}
	return defaultValue
}
