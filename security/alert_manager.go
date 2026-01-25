package security

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// Alert represents a quota or usage alert
type Alert struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Type      string    `json:"type"` // quota_warning, quota_critical, rate_limit, etc.
	Level     string    `json:"level"` // warning, critical
	Message   string    `json:"message"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
}

// AlertConfig defines alert configuration for a tenant
type AlertConfig struct {
	TenantID              string
	EnableQuotaAlerts     bool
	WarningThreshold      float64 // Percentage (0.0 - 1.0)
	CriticalThreshold     float64 // Percentage (0.0 - 1.0)
	WebhookURL            string
	EmailAddress          string
	DuplicateWindow       time.Duration // Minimum time between duplicate alerts
}

// AlertManager interface for managing alerts
type AlertManager interface {
	CheckThresholds(ctx context.Context, tenantID string, usage database.Usage, quota database.Quota) error
	SendAlert(ctx context.Context, alert *Alert) error
	ConfigureAlerts(ctx context.Context, tenantID string, config AlertConfig) error
	GetAlertConfig(ctx context.Context, tenantID string) (*AlertConfig, error)
	GetAlerts(ctx context.Context, tenantID string, limit int) ([]*Alert, error)
	Start(ctx context.Context) error
	Stop() error
}

// alertManager implements AlertManager interface
type alertManager struct {
	db              *sql.DB
	httpClient      *http.Client
	smtpConfig      SMTPConfig
	alertChan       chan *Alert
	recentAlerts    map[string]time.Time // Track recent alerts to prevent duplicates
	recentAlertsMu  sync.RWMutex
	wg              sync.WaitGroup
	stopChan        chan struct{}
	mu              sync.RWMutex
	stopped         bool
}

// SMTPConfig holds SMTP configuration for email alerts
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// AlertManagerConfig holds configuration for alert manager
type AlertManagerConfig struct {
	DB         *sql.DB
	HTTPClient *http.Client
	SMTPConfig SMTPConfig
	BufferSize int
}

// NewAlertManager creates a new alert manager instance
func NewAlertManager(config AlertManagerConfig) AlertManager {
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	if config.BufferSize == 0 {
		config.BufferSize = 100
	}

	return &alertManager{
		db:           config.DB,
		httpClient:   config.HTTPClient,
		smtpConfig:   config.SMTPConfig,
		alertChan:    make(chan *Alert, config.BufferSize),
		recentAlerts: make(map[string]time.Time),
		stopChan:     make(chan struct{}),
	}
}

// Start begins the alert manager background worker
func (a *alertManager) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return fmt.Errorf("alert manager already stopped")
	}

	a.wg.Add(1)
	go a.worker(ctx)

	// Start cleanup goroutine for recent alerts map
	a.wg.Add(1)
	go a.cleanupWorker(ctx)

	return nil
}

// Stop gracefully stops the alert manager
func (a *alertManager) Stop() error {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return nil
	}
	a.stopped = true
	a.mu.Unlock()

	close(a.stopChan)
	close(a.alertChan)
	a.wg.Wait()

	return nil
}

// CheckThresholds checks if usage has exceeded alert thresholds
func (a *alertManager) CheckThresholds(ctx context.Context, tenantID string, usage database.Usage, quota database.Quota) error {
	// Get alert configuration
	config, err := a.GetAlertConfig(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get alert config: %w", err)
	}

	if !config.EnableQuotaAlerts {
		return nil
	}

	// Calculate usage percentage
	var usagePercent float64
	if quota.Limit > 0 {
		usagePercent = float64(quota.CurrentUsage) / float64(quota.Limit)
	}

	// Check critical threshold (95%)
	if usagePercent >= config.CriticalThreshold {
		alert := &Alert{
			ID:       uuid.New().String(),
			TenantID: tenantID,
			Type:     "quota_critical",
			Level:    "critical",
			Message:  fmt.Sprintf("Quota usage at %.1f%% for %s", usagePercent*100, quota.QuotaType),
			Details: fmt.Sprintf(`{
				"quota_type": "%s",
				"current_usage": %d,
				"limit": %d,
				"percentage": %.2f
			}`, quota.QuotaType, quota.CurrentUsage, quota.Limit, usagePercent*100),
			CreatedAt: time.Now(),
		}

		if err := a.queueAlert(ctx, alert, config.DuplicateWindow); err != nil {
			return fmt.Errorf("failed to queue critical alert: %w", err)
		}
	} else if usagePercent >= config.WarningThreshold {
		// Check warning threshold (80%)
		alert := &Alert{
			ID:       uuid.New().String(),
			TenantID: tenantID,
			Type:     "quota_warning",
			Level:    "warning",
			Message:  fmt.Sprintf("Quota usage at %.1f%% for %s", usagePercent*100, quota.QuotaType),
			Details: fmt.Sprintf(`{
				"quota_type": "%s",
				"current_usage": %d,
				"limit": %d,
				"percentage": %.2f
			}`, quota.QuotaType, quota.CurrentUsage, quota.Limit, usagePercent*100),
			CreatedAt: time.Now(),
		}

		if err := a.queueAlert(ctx, alert, config.DuplicateWindow); err != nil {
			return fmt.Errorf("failed to queue warning alert: %w", err)
		}
	}

	return nil
}

// SendAlert sends an alert via configured channels
func (a *alertManager) SendAlert(ctx context.Context, alert *Alert) error {
	// Get alert configuration
	config, err := a.GetAlertConfig(ctx, alert.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get alert config: %w", err)
	}

	// Store alert in database
	if err := a.storeAlert(ctx, alert); err != nil {
		return fmt.Errorf("failed to store alert: %w", err)
	}

	// Send via webhook if configured
	if config.WebhookURL != "" {
		if err := a.sendWebhook(ctx, config.WebhookURL, alert); err != nil {
			// Log error but don't fail
			fmt.Printf("Failed to send webhook alert: %v\n", err)
		}
	}

	// Send via email if configured
	if config.EmailAddress != "" {
		if err := a.sendEmail(config.EmailAddress, alert); err != nil {
			// Log error but don't fail
			fmt.Printf("Failed to send email alert: %v\n", err)
		}
	}

	// Update sent timestamp
	now := time.Now()
	alert.SentAt = &now

	return nil
}

// ConfigureAlerts configures alert settings for a tenant
func (a *alertManager) ConfigureAlerts(ctx context.Context, tenantID string, config AlertConfig) error {
	query := `
		INSERT INTO alert_configs (tenant_id, enable_quota_alerts, warning_threshold, critical_threshold, webhook_url, email_address, duplicate_window)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id) DO UPDATE SET
			enable_quota_alerts = EXCLUDED.enable_quota_alerts,
			warning_threshold = EXCLUDED.warning_threshold,
			critical_threshold = EXCLUDED.critical_threshold,
			webhook_url = EXCLUDED.webhook_url,
			email_address = EXCLUDED.email_address,
			duplicate_window = EXCLUDED.duplicate_window,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := a.db.ExecContext(ctx, query,
		tenantID,
		config.EnableQuotaAlerts,
		config.WarningThreshold,
		config.CriticalThreshold,
		config.WebhookURL,
		config.EmailAddress,
		int(config.DuplicateWindow.Seconds()),
	)

	if err != nil {
		return fmt.Errorf("failed to configure alerts: %w", err)
	}

	return nil
}

// GetAlertConfig retrieves alert configuration for a tenant
func (a *alertManager) GetAlertConfig(ctx context.Context, tenantID string) (*AlertConfig, error) {
	query := `
		SELECT enable_quota_alerts, warning_threshold, critical_threshold, webhook_url, email_address, duplicate_window
		FROM alert_configs
		WHERE tenant_id = $1
	`

	config := &AlertConfig{TenantID: tenantID}
	var duplicateWindowSec int

	err := a.db.QueryRowContext(ctx, query, tenantID).Scan(
		&config.EnableQuotaAlerts,
		&config.WarningThreshold,
		&config.CriticalThreshold,
		&config.WebhookURL,
		&config.EmailAddress,
		&duplicateWindowSec,
	)

	if err == sql.ErrNoRows {
		// Return default configuration
		return a.getDefaultAlertConfig(tenantID), nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get alert config: %w", err)
	}

	config.DuplicateWindow = time.Duration(duplicateWindowSec) * time.Second

	return config, nil
}

// GetAlerts retrieves recent alerts for a tenant
func (a *alertManager) GetAlerts(ctx context.Context, tenantID string, limit int) ([]*Alert, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, tenant_id, type, level, message, details, created_at, sent_at
		FROM alerts
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := a.db.QueryContext(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		alert := &Alert{}
		err := rows.Scan(
			&alert.ID,
			&alert.TenantID,
			&alert.Type,
			&alert.Level,
			&alert.Message,
			&alert.Details,
			&alert.CreatedAt,
			&alert.SentAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alert: %w", err)
		}
		alerts = append(alerts, alert)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating alerts: %w", err)
	}

	return alerts, nil
}

// queueAlert queues an alert for sending, checking for duplicates
func (a *alertManager) queueAlert(ctx context.Context, alert *Alert, duplicateWindow time.Duration) error {
	// Check for duplicate alerts
	alertKey := fmt.Sprintf("%s:%s:%s", alert.TenantID, alert.Type, alert.Level)

	a.recentAlertsMu.RLock()
	lastSent, exists := a.recentAlerts[alertKey]
	a.recentAlertsMu.RUnlock()

	if exists && time.Since(lastSent) < duplicateWindow {
		// Duplicate alert within window, skip
		return nil
	}

	// Update recent alerts map
	a.recentAlertsMu.Lock()
	a.recentAlerts[alertKey] = time.Now()
	a.recentAlertsMu.Unlock()

	// Queue alert for sending
	select {
	case a.alertChan <- alert:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel full, send synchronously
		return a.SendAlert(ctx, alert)
	}
}

// worker processes alerts from the channel
func (a *alertManager) worker(ctx context.Context) {
	defer a.wg.Done()

	for {
		select {
		case alert, ok := <-a.alertChan:
			if !ok {
				return
			}

			if err := a.SendAlert(ctx, alert); err != nil {
				fmt.Printf("Error sending alert: %v\n", err)
			}

		case <-a.stopChan:
			return

		case <-ctx.Done():
			return
		}
	}
}

// cleanupWorker periodically cleans up old entries from recent alerts map
func (a *alertManager) cleanupWorker(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.cleanupRecentAlerts()

		case <-a.stopChan:
			return

		case <-ctx.Done():
			return
		}
	}
}

// cleanupRecentAlerts removes old entries from recent alerts map
func (a *alertManager) cleanupRecentAlerts() {
	a.recentAlertsMu.Lock()
	defer a.recentAlertsMu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for key, timestamp := range a.recentAlerts {
		if timestamp.Before(cutoff) {
			delete(a.recentAlerts, key)
		}
	}
}

// storeAlert stores an alert in the database
func (a *alertManager) storeAlert(ctx context.Context, alert *Alert) error {
	query := `
		INSERT INTO alerts (id, tenant_id, type, level, message, details, created_at, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := a.db.ExecContext(ctx, query,
		alert.ID,
		alert.TenantID,
		alert.Type,
		alert.Level,
		alert.Message,
		alert.Details,
		alert.CreatedAt,
		alert.SentAt,
	)

	if err != nil {
		return fmt.Errorf("failed to store alert: %w", err)
	}

	return nil
}

// sendWebhook sends an alert via webhook
func (a *alertManager) sendWebhook(ctx context.Context, webhookURL string, alert *Alert) error {
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned error status: %d", resp.StatusCode)
	}

	return nil
}

// sendEmail sends an alert via email
func (a *alertManager) sendEmail(to string, alert *Alert) error {
	if a.smtpConfig.Host == "" {
		return fmt.Errorf("SMTP not configured")
	}

	subject := fmt.Sprintf("[%s] %s", alert.Level, alert.Message)
	body := fmt.Sprintf(`
Alert Details:
--------------
Tenant ID: %s
Type: %s
Level: %s
Message: %s
Time: %s

Details:
%s
`, alert.TenantID, alert.Type, alert.Level, alert.Message, alert.CreatedAt.Format(time.RFC3339), alert.Details)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		a.smtpConfig.From, to, subject, body)

	auth := smtp.PlainAuth("", a.smtpConfig.Username, a.smtpConfig.Password, a.smtpConfig.Host)
	addr := fmt.Sprintf("%s:%d", a.smtpConfig.Host, a.smtpConfig.Port)

	err := smtp.SendMail(addr, auth, a.smtpConfig.From, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// getDefaultAlertConfig returns default alert configuration
func (a *alertManager) getDefaultAlertConfig(tenantID string) *AlertConfig {
	return &AlertConfig{
		TenantID:          tenantID,
		EnableQuotaAlerts: true,
		WarningThreshold:  0.80, // 80%
		CriticalThreshold: 0.95, // 95%
		DuplicateWindow:   1 * time.Hour,
	}
}
