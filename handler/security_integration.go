package handler

import (
	"database/sql"
	"fmt"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/security"
)

// SecurityComponents holds all security-related components
type SecurityComponents struct {
	Middleware    *SecurityMiddleware
	TenantManager security.TenantManager
	APIKeyManager security.APIKeyManager
	RateLimiter   security.RateLimiter
	IPFilter      security.IPFilter
	HMACVerifier  security.HMACVerifier
	QuotaManager  *security.QuotaManager
	UsageTracker  *security.UsageTracker
	AuditLogger   *security.AuditLogger
	BillingEngine *security.BillingEngine
	AlertManager  *security.AlertManager
}

// InitializeSecurityComponents initializes all security components
func InitializeSecurityComponents(db *sql.DB) (*SecurityComponents, error) {
	// Initialize core components
	tenantManager := security.NewTenantManager(db)
	apiKeyManager := security.NewAPIKeyManager(db)
	rateLimiter := security.NewRateLimiter(db)
	
	ipFilter, err := security.NewIPFilter(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize IP filter: %w", err)
	}
	
	hmacVerifier := security.NewHMACVerifier(db)
	quotaManager := security.NewQuotaManager(db)
	usageTracker := security.NewUsageTracker(db)
	
	// Initialize supporting services (BillingEngine needs UsageTracker)
	auditLogger := security.NewAuditLogger(db)
	billingEngine := security.NewBillingEngine(db, usageTracker)
	alertManager := security.NewAlertManager(db, quotaManager)
	
	// Create security middleware
	middleware := NewSecurityMiddleware(
		tenantManager,
		apiKeyManager,
		rateLimiter,
		ipFilter,
		hmacVerifier,
		quotaManager,
		auditLogger,
	)
	
	return &SecurityComponents{
		Middleware:    middleware,
		TenantManager: tenantManager,
		APIKeyManager: apiKeyManager,
		RateLimiter:   rateLimiter,
		IPFilter:      ipFilter,
		HMACVerifier:  hmacVerifier,
		QuotaManager:  quotaManager,
		UsageTracker:  usageTracker,
		AuditLogger:   auditLogger,
		BillingEngine: billingEngine,
		AlertManager:  alertManager,
	}, nil
}

// Close gracefully shuts down all security components
func (sc *SecurityComponents) Close() error {
	if sc.UsageTracker != nil {
		if err := sc.UsageTracker.Close(); err != nil {
			return fmt.Errorf("failed to close usage tracker: %w", err)
		}
	}
	
	if sc.AuditLogger != nil {
		if err := sc.AuditLogger.Close(); err != nil {
			return fmt.Errorf("failed to close audit logger: %w", err)
		}
	}
	
	return nil
}
