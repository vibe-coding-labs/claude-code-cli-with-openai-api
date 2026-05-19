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

// setupTestDB creates a test database with all required tables
func setupTestDB(t *testing.T) *sql.DB {
	// Create temporary database
	dbPath := "test_security.db"
	os.Remove(dbPath) // Clean up any existing test db

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	// Create tables
	schemas := []string{
		// Tenants table
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)`,
		// Tenant Configs table
		`CREATE TABLE IF NOT EXISTS tenant_configs (
			tenant_id TEXT PRIMARY KEY,
			allowed_models TEXT,
			default_model TEXT,
			custom_rate_limits INTEGER DEFAULT 0,
			require_hmac INTEGER DEFAULT 0,
			webhook_url TEXT,
			alert_email TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`,
		// API Keys table
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL UNIQUE,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP,
			last_used_at TIMESTAMP,
			hmac_secret TEXT,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`,
		// Quotas table
		`CREATE TABLE IF NOT EXISTS quotas (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			quota_type TEXT NOT NULL,
			period TEXT NOT NULL,
			[limit] INTEGER NOT NULL,
			current_usage INTEGER NOT NULL DEFAULT 0,
			reset_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
			UNIQUE(tenant_id, quota_type, period)
		)`,
		// Rate Limits table
		`CREATE TABLE IF NOT EXISTS rate_limits (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			dimension TEXT NOT NULL,
			algorithm TEXT NOT NULL,
			[limit] INTEGER NOT NULL,
			window INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		// IP Rules table
		`CREATE TABLE IF NOT EXISTS ip_rules (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			rule_type TEXT NOT NULL,
			ip_address TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		// Usage Records table
		`CREATE TABLE IF NOT EXISTS usage_records (
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
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
			FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE
		)`,
	}

	for _, schema := range schemas {
		_, err := db.Exec(schema)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})

	return db
}

// TestSecurityComponentsIntegration tests all security components together
func TestSecurityComponentsIntegration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Initialize components
	tenantMgr := NewTenantManager(db)
	apiKeyMgr := NewAPIKeyManager(db)
	quotaMgr := NewQuotaManager(db)
	usageTracker := NewUsageTracker(db)
	defer usageTracker.Close()

	// Test 1: Create a tenant
	tenant := &database.Tenant{
		ID:          "tenant-1",
		Name:        "Test Tenant",
		Description: "Integration test tenant",
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := tenantMgr.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Test 2: Create an API key for the tenant
	apiKey, err := apiKeyMgr.CreateKey(ctx, "tenant-1", "Test Key", nil)
	require.NoError(t, err)
	assert.Equal(t, "tenant-1", apiKey.TenantID)
	assert.NotEmpty(t, apiKey.PlainKey)

	// Get the plain key from the returned APIKey
	plainKey := apiKey.PlainKey

	// Test 3: Validate the API key
	validatedKey, err := apiKeyMgr.ValidateKey(ctx, plainKey)
	require.NoError(t, err)
	assert.Equal(t, apiKey.ID, validatedKey.ID)

	// Test 4: Set quotas for the tenant
	requestQuota := &database.Quota{
		TenantID:  "tenant-1",
		QuotaType: "requests",
		Period:    "daily",
		Limit:     1000,
	}
	err = quotaMgr.SetQuota(ctx, requestQuota)
	require.NoError(t, err)

	tokenQuota := &database.Quota{
		TenantID:  "tenant-1",
		QuotaType: "tokens",
		Period:    "daily",
		Limit:     100000,
	}
	err = quotaMgr.SetQuota(ctx, tokenQuota)
	require.NoError(t, err)

	// Test 5: Check quota (should allow)
	usage := database.Usage{
		Requests: 1,
		Tokens:   1000,
		Cost:     0.01,
	}
	allowed, _, err := quotaMgr.CheckQuota(ctx, "tenant-1", usage)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Test 6: Increment usage
	err = quotaMgr.IncrementUsage(ctx, "tenant-1", usage)
	require.NoError(t, err)

	// Test 7: Record usage
	usageRecord := &database.UsageRecord{
		TenantID:         "tenant-1",
		APIKeyID:         apiKey.ID,
		Model:            "claude-3-opus",
		PromptTokens:     500,
		CompletionTokens: 500,
		TotalTokens:      1000,
		Cost:             0.01,
		ResponseTime:     150,
		StatusCode:       200,
	}
	err = usageTracker.RecordUsage(ctx, usageRecord)
	require.NoError(t, err)

	// Wait for async processing (longer wait)
	time.Sleep(6 * time.Second)

	// Test 8: Get usage statistics
	period := database.TimePeriod{
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now().Add(1 * time.Hour),
	}
	stats, err := usageTracker.GetUsage(ctx, "tenant-1", period)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalRequests)
	assert.Equal(t, int64(1000), stats.TotalTokens)

	// Test 9: Get quotas
	quotas, err := quotaMgr.GetQuota(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Len(t, quotas, 2)

	// Test 10: Revoke API key
	err = apiKeyMgr.RevokeKey(ctx, apiKey.ID)
	require.NoError(t, err)

	// Test 11: Validate revoked key (should fail)
	_, err = apiKeyMgr.ValidateKey(ctx, plainKey)
	assert.Error(t, err)
}

// TestQuotaEnforcement tests quota enforcement
func TestQuotaEnforcement(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	tenantMgr := NewTenantManager(db)
	quotaMgr := NewQuotaManager(db)

	// Create tenant
	tenant := &database.Tenant{
		ID:        "tenant-2",
		Name:      "Quota Test Tenant",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := tenantMgr.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Set a low quota
	quota := &database.Quota{
		TenantID:  "tenant-2",
		QuotaType: "requests",
		Period:    "daily",
		Limit:     5,
	}
	err = quotaMgr.SetQuota(ctx, quota)
	require.NoError(t, err)

	// Use up the quota
	usage := database.Usage{Requests: 1}
	for i := 0; i < 5; i++ {
		allowed, _, err := quotaMgr.CheckQuota(ctx, "tenant-2", usage)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)

		err = quotaMgr.IncrementUsage(ctx, "tenant-2", usage)
		require.NoError(t, err)
	}

	// Next request should be denied
	allowed, quota, err := quotaMgr.CheckQuota(ctx, "tenant-2", usage)
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied after quota exceeded")
	assert.NotNil(t, quota)
	assert.True(t, quota.IsExceeded())
}

// TestRateLimiterPerformance tests rate limiter performance
func TestRateLimiterPerformance(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	rateLimiter := NewRateLimiter(db)

	// Set a rate limit
	limit := database.RateLimit{
		ID:        "rl-1",
		Dimension: "api_key",
		Algorithm: "fixed_window",
		Limit:     100,
		Window:    60, // 60 seconds
		CreatedAt: time.Now(),
	}
	err := rateLimiter.SetLimit(ctx, limit)
	require.NoError(t, err)

	// Measure performance
	start := time.Now()
	iterations := 1000

	for i := 0; i < iterations; i++ {
		_, _, err := rateLimiter.CheckLimit(ctx, "test-key", limit)
		require.NoError(t, err)
	}

	elapsed := time.Since(start)
	avgLatency := elapsed / time.Duration(iterations)

	t.Logf("Rate limiter average latency: %v", avgLatency)
	assert.Less(t, avgLatency, 1*time.Millisecond, "Rate limiter should be faster than 1ms")
}

// TestQuotaManagerPerformance tests quota manager performance
func TestQuotaManagerPerformance(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	tenantMgr := NewTenantManager(db)
	quotaMgr := NewQuotaManager(db)

	// Create tenant
	tenant := &database.Tenant{
		ID:        "tenant-perf",
		Name:      "Performance Test Tenant",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := tenantMgr.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Set quota
	quota := &database.Quota{
		TenantID:  "tenant-perf",
		QuotaType: "requests",
		Period:    "daily",
		Limit:     1000000,
	}
	err = quotaMgr.SetQuota(ctx, quota)
	require.NoError(t, err)

	// Measure performance
	start := time.Now()
	iterations := 1000
	usage := database.Usage{Requests: 1}

	for i := 0; i < iterations; i++ {
		_, _, err := quotaMgr.CheckQuota(ctx, "tenant-perf", usage)
		require.NoError(t, err)
	}

	elapsed := time.Since(start)
	avgLatency := elapsed / time.Duration(iterations)

	t.Logf("Quota manager average latency: %v", avgLatency)
	assert.Less(t, avgLatency, 2*time.Millisecond, "Quota manager should be faster than 2ms")
}
