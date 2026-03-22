package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/security"
)

// AdminAPIHandler handles admin API endpoints for security and multi-tenancy
type AdminAPIHandler struct {
	securityComponents *SecurityComponents
}

// NewAdminAPIHandler creates a new admin API handler
func NewAdminAPIHandler(securityComponents *SecurityComponents) *AdminAPIHandler {
	return &AdminAPIHandler{
		securityComponents: securityComponents,
	}
}

// ==================== Tenant Management Endpoints ====================

// ListTenants lists all tenants with optional filters
func (h *AdminAPIHandler) ListTenants(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse filters
	filters := database.TenantFilters{
		Status:    c.Query("status"),
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "DESC"),
	}

	if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 {
		filters.Limit = limit
	}
	if offset, err := strconv.Atoi(c.Query("offset")); err == nil && offset >= 0 {
		filters.Offset = offset
	}

	tenants, err := h.securityComponents.TenantManager.ListTenants(ctx, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list tenants: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenants": tenants})
}

// GetTenant retrieves a tenant by ID
func (h *AdminAPIHandler) GetTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("id")

	tenant, err := h.securityComponents.TenantManager.GetTenant(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenant": tenant})
}

// CreateTenant creates a new tenant
func (h *AdminAPIHandler) CreateTenant(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		Name        string `json:"name" binding:"required,min=3,max=100"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Metadata    string `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Set default status
	if req.Status == "" {
		req.Status = "active"
	}

	tenant := &database.Tenant{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		Metadata:    req.Metadata,
	}

	if err := h.securityComponents.TenantManager.CreateTenant(ctx, tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tenant: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"tenant": tenant})
}

// UpdateTenant updates an existing tenant
func (h *AdminAPIHandler) UpdateTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("id")

	var req struct {
		Name        string `json:"name" binding:"required,min=3,max=100"`
		Description string `json:"description"`
		Status      string `json:"status" binding:"required"`
		Metadata    string `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	tenant := &database.Tenant{
		ID:          tenantID,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		Metadata:    req.Metadata,
	}

	if err := h.securityComponents.TenantManager.UpdateTenant(ctx, tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update tenant: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenant": tenant})
}

// DeleteTenant deletes a tenant
func (h *AdminAPIHandler) DeleteTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("id")

	if err := h.securityComponents.TenantManager.DeleteTenant(ctx, tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete tenant: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tenant deleted successfully"})
}

// GetTenantConfig retrieves tenant configuration
func (h *AdminAPIHandler) GetTenantConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("id")

	config, err := h.securityComponents.TenantManager.GetTenantConfig(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tenant config not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"config": config})
}

// UpdateTenantConfig updates tenant configuration
func (h *AdminAPIHandler) UpdateTenantConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("id")

	var req struct {
		AllowedModels    []string `json:"allowed_models"`
		DefaultModel     string   `json:"default_model"`
		CustomRateLimits bool     `json:"custom_rate_limits"`
		RequireHMAC      bool     `json:"require_hmac"`
		WebhookURL       string   `json:"webhook_url"`
		AlertEmail       string   `json:"alert_email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	config := &database.TenantConfig{
		TenantID:         tenantID,
		AllowedModels:    req.AllowedModels,
		DefaultModel:     req.DefaultModel,
		CustomRateLimits: req.CustomRateLimits,
		RequireHMAC:      req.RequireHMAC,
		WebhookURL:       req.WebhookURL,
		AlertEmail:       req.AlertEmail,
	}

	if err := h.securityComponents.TenantManager.UpdateTenantConfig(ctx, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update tenant config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"config": config})
}

// ==================== API Key Management Endpoints ====================

// ListAPIKeys lists all API keys for a tenant
func (h *AdminAPIHandler) ListAPIKeys(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	keys, err := h.securityComponents.APIKeyManager.ListKeys(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list API keys: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

// CreateAPIKey creates a new API key for a tenant
func (h *AdminAPIHandler) CreateAPIKey(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	var req struct {
		Name      string     `json:"name" binding:"required,min=3,max=100"`
		ExpiresAt *time.Time `json:"expires_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	key, err := h.securityComponents.APIKeyManager.CreateKey(ctx, tenantID, req.Name, req.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"key": key})
}

// RevokeAPIKey revokes an API key
func (h *AdminAPIHandler) RevokeAPIKey(c *gin.Context) {
	ctx := c.Request.Context()
	keyID := c.Param("key_id")

	if err := h.securityComponents.APIKeyManager.RevokeKey(ctx, keyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke API key: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked successfully"})
}

// RotateAPIKey rotates an API key
func (h *AdminAPIHandler) RotateAPIKey(c *gin.Context) {
	ctx := c.Request.Context()
	keyID := c.Param("key_id")

	var req struct {
		GracePeriod int `json:"grace_period_seconds"` // Default: 86400 (24 hours)
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	gracePeriod := time.Duration(req.GracePeriod) * time.Second
	if gracePeriod == 0 {
		gracePeriod = 24 * time.Hour
	}

	newKey, err := h.securityComponents.APIKeyManager.RotateKey(ctx, keyID, gracePeriod)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rotate API key: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"key": newKey})
}

// ==================== Quota Management Endpoints ====================

// GetTenantQuota retrieves quota information for a tenant
func (h *AdminAPIHandler) GetTenantQuota(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	quotas, err := h.securityComponents.QuotaManager.GetQuota(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get quotas: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"quotas": quotas})
}

// SetTenantQuota sets or updates quota for a tenant
func (h *AdminAPIHandler) SetTenantQuota(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	var req struct {
		QuotaType string `json:"quota_type" binding:"required"` // requests, tokens, cost
		Period    string `json:"period" binding:"required"`     // daily, monthly
		Limit     int64  `json:"limit" binding:"required,min=0"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Validate quota type
	if req.QuotaType != "requests" && req.QuotaType != "tokens" && req.QuotaType != "cost" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quota type. Must be: requests, tokens, or cost"})
		return
	}

	// Validate period
	if req.Period != "daily" && req.Period != "monthly" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid period. Must be: daily or monthly"})
		return
	}

	quota := &database.Quota{
		TenantID:  tenantID,
		QuotaType: req.QuotaType,
		Period:    req.Period,
		Limit:     req.Limit,
	}

	if err := h.securityComponents.QuotaManager.SetQuota(ctx, quota); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set quota: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"quota": quota})
}

// ResetTenantQuota resets usage for a specific quota type
func (h *AdminAPIHandler) ResetTenantQuota(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")
	quotaType := c.Param("quota_type")

	// Validate quota type
	if quotaType != "requests" && quotaType != "tokens" && quotaType != "cost" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quota type. Must be: requests, tokens, or cost"})
		return
	}

	if err := h.securityComponents.QuotaManager.ResetQuota(ctx, tenantID, database.QuotaType(quotaType)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset quota: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Quota reset successfully"})
}

// ==================== Usage and Reporting Endpoints ====================

// GetTenantUsage retrieves usage statistics for a tenant
func (h *AdminAPIHandler) GetTenantUsage(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	// Parse date range
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -30).Format("YYYY-MM-DD")
	}
	if endDate == "" {
		endDate = time.Now().Format("YYYY-MM-DD")
	}

	// Parse period for aggregation
	period := c.DefaultQuery("period", "daily") // daily, monthly

	// Query usage from database
	usage, err := h.queryTenantUsage(ctx, tenantID, startDate, endDate, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get usage: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tenant_id":  tenantID,
		"start_date": startDate,
		"end_date":   endDate,
		"period":     period,
		"usage":      usage,
	})
}

// queryTenantUsage queries usage records for a tenant
func (h *AdminAPIHandler) queryTenantUsage(ctx context.Context, tenantID, startDate, endDate, period string) ([]*database.UsageRecord, error) {
	// This would query the usage_records table
	// For now, return empty result
	return []*database.UsageRecord{}, nil
}

// GenerateBillingReport generates a billing report for a tenant
func (h *AdminAPIHandler) GenerateBillingReport(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	var req struct {
		StartDate time.Time `json:"start_date" binding:"required"`
		EndDate   time.Time `json:"end_date" binding:"required"`
		Format    string    `json:"format"` // json, csv
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if req.Format == "" {
		req.Format = "json"
	}

	period := security.TimePeriod{
		Start: req.StartDate,
		End:   req.EndDate,
	}

	report, err := h.securityComponents.BillingEngine.GenerateReport(ctx, tenantID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate billing report: " + err.Error()})
		return
	}

	if req.Format == "csv" {
		// Generate CSV format
		csvData, err := h.securityComponents.BillingEngine.ExportReport(ctx, report, security.ExportFormatCSV)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export report: " + err.Error()})
			return
		}
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=billing-report-%s.csv", tenantID))
		c.String(http.StatusOK, string(csvData))
		return
	}

	c.JSON(http.StatusOK, gin.H{"report": report})
}

// ==================== Rate Limit Management Endpoints ====================

// ListRateLimits lists all rate limits for a tenant
func (h *AdminAPIHandler) ListRateLimits(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	limits, err := h.securityComponents.RateLimiter.GetLimits(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list rate limits: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rate_limits": limits})
}

// CreateRateLimit creates a new rate limit for a tenant
func (h *AdminAPIHandler) CreateRateLimit(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	var req struct {
		Dimension string `json:"dimension" binding:"required"` // api_key, ip, tenant
		Algorithm string `json:"algorithm" binding:"required"` // fixed_window, sliding_window, token_bucket
		Limit     int    `json:"limit" binding:"required,min=1"`
		Window    int    `json:"window" binding:"required,min=1"` // Seconds
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Validate dimension
	if req.Dimension != "api_key" && req.Dimension != "ip" && req.Dimension != "tenant" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid dimension. Must be: api_key, ip, or tenant"})
		return
	}

	// Validate algorithm
	if req.Algorithm != "fixed_window" && req.Algorithm != "sliding_window" && req.Algorithm != "token_bucket" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid algorithm. Must be: fixed_window, sliding_window, or token_bucket"})
		return
	}

	rateLimit := database.RateLimit{
		TenantID:  tenantID,
		Dimension: req.Dimension,
		Algorithm: req.Algorithm,
		Limit:     req.Limit,
		Window:    req.Window,
		CreatedAt: time.Now(),
	}

	if err := h.securityComponents.RateLimiter.SetLimit(ctx, rateLimit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create rate limit: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"rate_limit": rateLimit})
}

// DeleteRateLimit deletes a rate limit
func (h *AdminAPIHandler) DeleteRateLimit(c *gin.Context) {
	ctx := c.Request.Context()
	limitID := c.Param("limit_id")

	if err := h.securityComponents.RateLimiter.DeleteLimit(ctx, limitID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete rate limit: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rate limit deleted successfully"})
}

// ==================== IP Rule Management Endpoints ====================

// ListIPRules lists all IP rules for a tenant
func (h *AdminAPIHandler) ListIPRules(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	rules, err := h.securityComponents.IPFilter.ListRules(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list IP rules: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ip_rules": rules})
}

// CreateIPRule creates a new IP rule for a tenant
func (h *AdminAPIHandler) CreateIPRule(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	var req struct {
		RuleType    string `json:"rule_type" binding:"required"` // whitelist, blacklist
		IPAddress   string `json:"ip_address" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Validate rule type
	if req.RuleType != "whitelist" && req.RuleType != "blacklist" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule type. Must be: whitelist or blacklist"})
		return
	}

	rule := &database.IPRule{
		TenantID:    tenantID,
		RuleType:    req.RuleType,
		IPAddress:   req.IPAddress,
		Description: req.Description,
		CreatedAt:   time.Now(),
	}

	var err error
	if req.RuleType == "whitelist" {
		err = h.securityComponents.IPFilter.AddWhitelist(ctx, *rule)
	} else {
		err = h.securityComponents.IPFilter.AddBlacklist(ctx, *rule)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create IP rule: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ip_rule": rule})
}

// DeleteIPRule deletes an IP rule
func (h *AdminAPIHandler) DeleteIPRule(c *gin.Context) {
	ctx := c.Request.Context()
	ruleID := c.Param("rule_id")

	if err := h.securityComponents.IPFilter.RemoveRule(ctx, ruleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete IP rule: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "IP rule deleted successfully"})
}

// ==================== Audit Log Endpoints ====================

// ListAuditLogs lists audit logs with optional filters
func (h *AdminAPIHandler) ListAuditLogs(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse filters
	filters := security.AuditFilters{
		TenantID:  c.Query("tenant_id"),
		EventType: c.Query("event_type"),
		Actor:     c.Query("actor"),
		Result:    c.Query("result"),
	}

	if limit, err := strconv.Atoi(c.DefaultQuery("limit", "100")); err == nil && limit > 0 {
		filters.Limit = limit
	}
	if offset, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil && offset >= 0 {
		filters.Offset = offset
	}

	logs, err := h.securityComponents.AuditLogger.QueryEvents(ctx, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit logs: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"audit_logs": logs})
}
