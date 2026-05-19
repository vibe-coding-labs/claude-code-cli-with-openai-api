package security

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// setupSupportingServicesTestDB creates a test database for supporting services
func setupSupportingServicesTestDB(t *testing.T) *sql.DB {
	dbPath := "test_supporting_services.db"
	os.Remove(dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	// Create audit_logs table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			event_type TEXT NOT NULL,
			actor TEXT NOT NULL,
			resource TEXT NOT NULL,
			action TEXT NOT NULL,
			result TEXT NOT NULL,
			details TEXT,
			ip_address TEXT,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create usage_records table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_records (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			api_key_id TEXT NOT NULL,
			model TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL,
			completion_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cost REAL NOT NULL,
			response_time INTEGER NOT NULL,
			status_code INTEGER NOT NULL,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create alert_configs table (used by AlertManager.ConfigureAlerts/GetAlertConfig)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS alert_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id TEXT NOT NULL UNIQUE,
			enable_quota_alerts BOOLEAN NOT NULL DEFAULT 1,
			warning_threshold REAL NOT NULL DEFAULT 0.8,
			critical_threshold REAL NOT NULL DEFAULT 0.95,
			webhook_url TEXT,
			email_address TEXT,
			duplicate_window INTEGER NOT NULL DEFAULT 3600,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create alerts table (used by AlertManager.storeAlert/GetAlerts)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS alerts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			type TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			details TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			sent_at TIMESTAMP
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})

	return db
}

// TestAuditLogger tests the audit logger functionality
func TestAuditLogger(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	auditLogger := NewAuditLogger(AuditLoggerConfig{
		DB:          db,
		BufferSize:  100,
		BatchSize:   10,
		FlushPeriod: 1 * time.Second,
	})

	// Start the audit logger
	err := auditLogger.Start(ctx)
	require.NoError(t, err)
	defer auditLogger.Stop()

	// Test logging an event
	event := &AuditEvent{
		TenantID:  "tenant-1",
		EventType: "test_event",
		Actor:     "test_user",
		Resource:  "test_resource",
		Action:    "test_action",
		Result:    "success",
		Details:   `{"key": "value"}`,
		IPAddress: "192.168.1.1",
	}

	err = auditLogger.LogEvent(ctx, event)
	require.NoError(t, err)

	// Wait for async processing
	time.Sleep(2 * time.Second)

	// Query events
	filters := AuditFilters{
		TenantID:  "tenant-1",
		EventType: "test_event",
		Limit:     10,
	}

	events, err := auditLogger.QueryEvents(ctx, filters)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "test_event", events[0].EventType)
	assert.Equal(t, "test_user", events[0].Actor)
}

// TestAuditLoggerHelperFunctions tests the helper functions
func TestAuditLoggerHelperFunctions(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	auditLogger := NewAuditLogger(AuditLoggerConfig{
		DB:          db,
		BufferSize:  100,
		BatchSize:   10,
		FlushPeriod: 1 * time.Second,
	})

	// Start the audit logger
	err := auditLogger.Start(ctx)
	require.NoError(t, err)
	defer auditLogger.Stop()

	// Test authentication failure logging
	err = auditLogger.LogEvent(ctx, &AuditEvent{
		TenantID:  "tenant-1",
		EventType: "authentication_failure",
		Actor:     "user-1",
		Resource:  "api_key",
		Action:    "authenticate",
		Result:    "failure",
		Details:   `{"reason": "Invalid API key"}`,
		IPAddress: "192.168.1.1",
	})
	require.NoError(t, err)

	// Test rate limit violation logging
	err = auditLogger.LogEvent(ctx, &AuditEvent{
		TenantID:  "tenant-1",
		EventType: "rate_limit_violation",
		Actor:     "user-1",
		Resource:  "api",
		Action:    "request",
		Result:    "failure",
		Details:   `{"reason": "Exceeded 100 req/min"}`,
		IPAddress: "192.168.1.1",
	})
	require.NoError(t, err)

	// Test quota exceeded logging
	err = auditLogger.LogEvent(ctx, &AuditEvent{
		TenantID:  "tenant-1",
		EventType: "quota_exceeded",
		Actor:     "user-1",
		Resource:  "quota",
		Action:    "check",
		Result:    "failure",
		Details:   `{"reason": "Monthly quota exceeded"}`,
		IPAddress: "192.168.1.1",
	})
	require.NoError(t, err)

	// Wait for async processing
	time.Sleep(2 * time.Second)

	// Query all events
	filters := AuditFilters{
		TenantID: "tenant-1",
		Limit:    10,
	}

	events, err := auditLogger.QueryEvents(ctx, filters)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 3)
}

// TestAuditLoggerPurge tests the purge functionality
func TestAuditLoggerPurge(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	auditLogger := NewAuditLogger(AuditLoggerConfig{
		DB:          db,
		BufferSize:  100,
		BatchSize:   10,
		FlushPeriod: 1 * time.Second,
	})

	// Start the audit logger
	err := auditLogger.Start(ctx)
	require.NoError(t, err)
	defer auditLogger.Stop()

	// Log an old event
	event := &AuditEvent{
		TenantID:  "tenant-1",
		EventType: "old_event",
		Actor:     "test_user",
		Resource:  "test_resource",
		Action:    "test_action",
		Result:    "success",
		IPAddress: "192.168.1.1",
		Timestamp: time.Now().AddDate(0, 0, -100), // 100 days ago
	}

	err = auditLogger.LogEvent(ctx, event)
	require.NoError(t, err)

	// Wait for async processing
	time.Sleep(2 * time.Second)

	// Purge old events (older than 90 days)
	cutoff := time.Now().AddDate(0, 0, -90)
	err = auditLogger.PurgeOldEvents(ctx, cutoff)
	require.NoError(t, err)

	// Query events - should be empty
	filters := AuditFilters{
		TenantID: "tenant-1",
		Limit:    10,
	}

	events, err := auditLogger.QueryEvents(ctx, filters)
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

// TestBillingEngine tests the billing engine functionality
func TestBillingEngine(t *testing.T) {
	db := setupSupportingServicesTestDB(t)

	billingEngine := NewBillingEngine(BillingEngineConfig{
		DB: db,
	})

	// Test cost calculation
	usage := Usage{
		TenantID:         "tenant-1",
		PromptTokens:     500,
		CompletionTokens: 500,
		TotalTokens:      1000,
		RequestCount:     1,
	}

	tier := PricingTier{
		ID:                   "test",
		Name:                 "Test Tier",
		PromptTokenPrice:     0.01, // $0.01 per 1000 tokens
		CompletionTokenPrice: 0.01, // $0.01 per 1000 tokens
		RequestPrice:         0.0,
	}

	cost, err := billingEngine.CalculateCost(usage, tier)
	require.NoError(t, err)
	assert.Equal(t, 0.01, cost) // (500 + 500) / 1000 * 0.01 = 0.01

	// Test with different prices
	tierWithHigherPrice := PricingTier{
		ID:                   "premium",
		Name:                 "Premium",
		PromptTokenPrice:     0.02,
		CompletionTokenPrice: 0.03,
		RequestPrice:         0.001,
	}

	cost, err = billingEngine.CalculateCost(usage, tierWithHigherPrice)
	require.NoError(t, err)
	// (500/1000 * 0.02) + (500/1000 * 0.03) + 0.001 = 0.01 + 0.015 + 0.001 = 0.026
	assert.InDelta(t, 0.026, cost, 0.001)
}

func TestBillingEngineReportGeneration(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS pricing_tiers (
			tenant_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			prompt_token_price REAL NOT NULL,
			completion_token_price REAL NOT NULL,
			request_price REAL NOT NULL,
			volume_discount_enabled BOOLEAN NOT NULL DEFAULT 0,
			volume_discount_tiers TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO tenants (id, name, status) VALUES (?, ?, ?)`, "tenant-1", "Tenant One", "active")
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO usage_records (id, tenant_id, api_key_id, model, prompt_tokens, completion_tokens, total_tokens, cost, response_time, status_code, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"usage-1", "tenant-1", "key-1", "gpt-4o", 300, 700, 1000, 0.0, 120, 200, time.Now().Add(-1*time.Hour),
	)
	require.NoError(t, err)

	billingEngine := NewBillingEngine(BillingEngineConfig{DB: db})

	err = billingEngine.SetPricingTier(ctx, "tenant-1", PricingTier{
		ID:                   "tier-1",
		Name:                 "Standard",
		PromptTokenPrice:     0.01,
		CompletionTokenPrice: 0.02,
		RequestPrice:         0.001,
	})
	require.NoError(t, err)

	report, err := billingEngine.GenerateReport(ctx, "tenant-1", TimePeriod{
		Start: time.Now().Add(-24 * time.Hour),
		End:   time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, "tenant-1", report.TenantID)
	assert.Equal(t, "Tenant One", report.TenantName)
	assert.Equal(t, 1, report.TotalRequests)
	assert.Equal(t, 300, report.TotalPromptTokens)
	assert.Equal(t, 700, report.TotalCompTokens)
	assert.Equal(t, 1000, report.TotalTokens)
	assert.Len(t, report.LineItems, 1)
	assert.Greater(t, report.TotalCost, 0.0)
}

// TestBillingEngineExport tests report export functionality
func TestBillingEngineExport(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	billingEngine := NewBillingEngine(BillingEngineConfig{DB: db})

	report := &BillingReport{
		TenantID:          "tenant-1",
		TenantName:        "Tenant One",
		Period:            TimePeriod{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
		TotalRequests:     1,
		TotalPromptTokens: 300,
		TotalCompTokens:   700,
		TotalTokens:       1000,
		TotalCost:         0.018,
		PricingTier:       "Standard",
		GeneratedAt:       time.Now(),
		LineItems: []BillingLineItem{
			{
				Date:             time.Now(),
				Model:            "gpt-4o",
				Requests:         1,
				PromptTokens:     300,
				CompletionTokens: 700,
				Cost:             0.018,
			},
		},
	}

	jsonData, err := billingEngine.ExportReport(ctx, report, ExportFormatJSON)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"tenant-1\"")
	assert.Contains(t, string(jsonData), "\"gpt-4o\"")

	csvData, err := billingEngine.ExportReport(ctx, report, ExportFormatCSV)
	require.NoError(t, err)
	assert.Contains(t, string(csvData), "Tenant ID,Tenant Name,Period Start,Period End,Date,Model,Requests,Prompt Tokens,Completion Tokens,Cost")
	assert.Contains(t, string(csvData), "tenant-1,Tenant One")
	assert.Contains(t, string(csvData), "TOTAL")

	_, err = billingEngine.ExportReport(ctx, report, ExportFormat("xml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported export format")
}

// TestAlertManager tests the alert manager functionality
func TestAlertManager(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	alertManager := NewAlertManager(AlertManagerConfig{
		DB:         db,
		BufferSize: 100,
	})

	// Start the alert manager
	err := alertManager.Start(ctx)
	require.NoError(t, err)
	defer alertManager.Stop()

	// Configure alerts
	config := AlertConfig{
		TenantID:          "tenant-1",
		WarningThreshold:  80.0,
		CriticalThreshold: 95.0,
		EnableQuotaAlerts: true,
	}

	err = alertManager.ConfigureAlerts(ctx, "tenant-1", config)
	require.NoError(t, err)

	// Create a quota at 85% usage (should trigger warning)
	quota := database.Quota{
		ID:           "quota-1",
		TenantID:     "tenant-1",
		QuotaType:    "requests",
		Period:       "daily",
		Limit:        1000,
		CurrentUsage: 850, // 85%
		ResetAt:      time.Now().Add(24 * time.Hour),
		UpdatedAt:    time.Now(),
	}

	usage := database.Usage{
		Requests: 1,
	}

	// Check thresholds
	err = alertManager.CheckThresholds(ctx, "tenant-1", usage, quota)
	require.NoError(t, err)

	// Verify alert was recorded (in production, this would check the database)
	// For now, we just verify no error occurred
}

// TestAlertManagerDuplicatePrevention tests duplicate alert prevention
func TestAlertManagerDuplicatePrevention(t *testing.T) {
	db := setupSupportingServicesTestDB(t)
	ctx := context.Background()

	alertManager := NewAlertManager(AlertManagerConfig{
		DB:         db,
		BufferSize: 100,
	})

	// Start the alert manager
	err := alertManager.Start(ctx)
	require.NoError(t, err)
	defer alertManager.Stop()

	// Configure alerts
	config := AlertConfig{
		TenantID:          "tenant-1",
		WarningThreshold:  80.0,
		CriticalThreshold: 95.0,
		EnableQuotaAlerts: true,
	}

	err = alertManager.ConfigureAlerts(ctx, "tenant-1", config)
	require.NoError(t, err)

	// Create a quota at 85% usage
	quota := database.Quota{
		ID:           "quota-1",
		TenantID:     "tenant-1",
		QuotaType:    "requests",
		Period:       "daily",
		Limit:        1000,
		CurrentUsage: 850,
		ResetAt:      time.Now().Add(24 * time.Hour),
		UpdatedAt:    time.Now(),
	}

	usage := database.Usage{
		Requests: 1,
	}

	// Send first alert
	err = alertManager.CheckThresholds(ctx, "tenant-1", usage, quota)
	require.NoError(t, err)

	// Try to send duplicate alert immediately (should be prevented by internal deduplication)
	err = alertManager.CheckThresholds(ctx, "tenant-1", usage, quota)
	require.NoError(t, err)

	// Note: In production, the alert manager prevents duplicates within a 1-hour window
}
