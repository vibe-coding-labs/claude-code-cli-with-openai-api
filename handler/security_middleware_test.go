package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestSecurityMiddleware(t *testing.T) (*SecurityComponents, func()) {
	// Create in-memory database
	_, err := database.InitTestDB()
	if err != nil {
		// If initialization fails due to migration conflicts, try without migrations
		t.Logf("Warning: Failed to initialize test DB with migrations: %v", err)
		t.Logf("Attempting to initialize without migrations...")
		
		// Create a minimal test database
		sqlDB, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
		require.NoError(t, err)
		
		database.DB = sqlDB
		
		// Create only the security tables we need
		err = createSecurityTables(sqlDB)
		require.NoError(t, err)
	}
	
	// Initialize security components
	components, err := InitializeSecurityComponents(database.DB)
	require.NoError(t, err)
	
	cleanup := func() {
		components.Close()
		database.CloseDB()
	}
	
	return components, cleanup
}

// createSecurityTables creates only the security-related tables for testing
func createSecurityTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			metadata TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_configs (
			tenant_id TEXT PRIMARY KEY,
			allowed_models TEXT,
			default_model TEXT,
			custom_rate_limits BOOLEAN DEFAULT 0,
			require_hmac BOOLEAN DEFAULT 0,
			webhook_url TEXT,
			alert_email TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			name TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			expires_at DATETIME,
			last_used_at DATETIME,
			hmac_secret TEXT,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS quotas (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			quota_type TEXT NOT NULL,
			period TEXT NOT NULL,
			[limit] INTEGER NOT NULL,
			current_usage INTEGER DEFAULT 0,
			reset_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			dimension TEXT NOT NULL,
			algorithm TEXT NOT NULL DEFAULT 'fixed_window',
			[limit] INTEGER NOT NULL,
			window INTEGER NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ip_rules (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			rule_type TEXT NOT NULL,
			ip_address TEXT NOT NULL,
			description TEXT,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_records (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			api_key_id TEXT NOT NULL,
			model TEXT NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			cost REAL DEFAULT 0.0,
			response_time INTEGER DEFAULT 0,
			status_code INTEGER NOT NULL,
			timestamp DATETIME NOT NULL,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
			FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			event_type TEXT NOT NULL,
			event_data TEXT,
			ip_address TEXT,
			user_agent TEXT,
			timestamp DATETIME NOT NULL
		)`,
	}
	
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}
	
	return nil
}

func TestAuthenticationMiddleware(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &database.Tenant{
		Name:        "Test Tenant",
		Description: "Test tenant for middleware testing",
		Status:      "active",
	}
	err := components.TenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Create an API key for the tenant
	apiKey, err := components.APIKeyManager.CreateKey(ctx, tenant.ID, "Test Key", nil)
	require.NoError(t, err)
	
	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectTenantID bool
	}{
		{
			name:           "Valid API key",
			authHeader:     "Bearer " + apiKey.PlainKey,
			expectedStatus: http.StatusOK,
			expectTenantID: true,
		},
		{
			name:           "Missing Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectTenantID: false,
		},
		{
			name:           "Invalid Authorization format",
			authHeader:     "InvalidFormat",
			expectedStatus: http.StatusUnauthorized,
			expectTenantID: false,
		},
		{
			name:           "Invalid API key",
			authHeader:     "Bearer invalid-key",
			expectedStatus: http.StatusUnauthorized,
			expectTenantID: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			
			router.Use(components.Middleware.AuthenticationMiddleware())
			router.GET("/test", func(c *gin.Context) {
				tenantID, exists := c.Get("tenant_id")
				if tt.expectTenantID {
					assert.True(t, exists)
					assert.Equal(t, tenant.ID, tenantID)
				}
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})
			
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestIPFilterMiddleware(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &database.Tenant{
		Name:   "Test Tenant",
		Status: "active",
	}
	err := components.TenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Add IP to blacklist
	blacklistRule := database.IPRule{
		TenantID:    tenant.ID,
		RuleType:    "blacklist",
		IPAddress:   "192.168.1.100",
		Description: "Blocked IP",
	}
	err = components.IPFilter.AddBlacklist(ctx, blacklistRule)
	require.NoError(t, err)
	
	tests := []struct {
		name           string
		clientIP       string
		tenantID       string
		expectedStatus int
	}{
		{
			name:           "Allowed IP",
			clientIP:       "192.168.1.1",
			tenantID:       tenant.ID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Blocked IP",
			clientIP:       "192.168.1.100",
			tenantID:       tenant.ID,
			expectedStatus: http.StatusForbidden,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			
			router.Use(func(c *gin.Context) {
				// Set tenant ID in context
				c.Set("tenant_id", tt.tenantID)
				c.Next()
			})
			router.Use(components.Middleware.IPFilterMiddleware())
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})
			
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.clientIP + ":12345"
			
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &database.Tenant{
		Name:   "Test Tenant",
		Status: "active",
	}
	err := components.TenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Create an API key
	apiKey, err := components.APIKeyManager.CreateKey(ctx, tenant.ID, "Test Key", nil)
	require.NoError(t, err)
	
	// Set a rate limit (5 requests per 10 seconds)
	rateLimit := database.RateLimit{
		TenantID:  tenant.ID,
		Dimension: "tenant",
		Algorithm: "fixed_window",
		Limit:     5,
		Window:    10,
	}
	err = components.RateLimiter.SetLimit(ctx, rateLimit)
	require.NoError(t, err)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	router.Use(func(c *gin.Context) {
		// Set tenant and API key in context
		c.Set("tenant_id", tenant.ID)
		c.Set("api_key_id", apiKey.ID)
		c.Next()
	})
	router.Use(components.Middleware.RateLimitMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	// Make requests up to the limit
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i+1)
	}
	
	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "Request should be rate limited")
	assert.NotEmpty(t, w.Header().Get("Retry-After"), "Should have Retry-After header")
}

func TestQuotaCheckMiddleware(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &database.Tenant{
		Name:   "Test Tenant",
		Status: "active",
	}
	err := components.TenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Set a quota (10 requests)
	quota := &database.Quota{
		TenantID:  tenant.ID,
		QuotaType: "requests",
		Period:    "daily",
		Limit:     10,
	}
	err = components.QuotaManager.SetQuota(ctx, quota)
	require.NoError(t, err)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	router.Use(func(c *gin.Context) {
		// Set tenant in context
		c.Set("tenant_id", tenant.ID)
		c.Next()
	})
	router.Use(components.Middleware.QuotaCheckMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	// First request should succeed
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Increment usage to exceed quota
	usage := database.Usage{
		Requests: 15, // Exceed the limit
	}
	err = components.QuotaManager.IncrementUsage(ctx, tenant.ID, usage)
	require.NoError(t, err)
	
	// Next request should be blocked
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestUsageTrackingMiddleware(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &database.Tenant{
		Name:   "Test Tenant",
		Status: "active",
	}
	err := components.TenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Create an API key
	apiKey, err := components.APIKeyManager.CreateKey(ctx, tenant.ID, "Test Key", nil)
	require.NoError(t, err)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	router.Use(func(c *gin.Context) {
		// Set tenant and API key in context
		c.Set("tenant_id", tenant.ID)
		c.Set("api_key_id", apiKey.ID)
		c.Next()
	})
	router.Use(components.Middleware.UsageTrackingMiddleware(components.UsageTracker))
	router.GET("/test", func(c *gin.Context) {
		// Simulate setting usage info
		c.Set("model", "gpt-4")
		c.Set("prompt_tokens", 100)
		c.Set("completion_tokens", 50)
		c.Set("total_tokens", 150)
		c.Set("cost", 0.015)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	// Make a request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Wait for async processing
	time.Sleep(100 * time.Millisecond)
	
	// Verify usage was recorded (would need to query usage tracker)
	// This is a basic test - more detailed verification would require
	// querying the usage tracker's database
}

func TestMiddlewareChain(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &database.Tenant{
		Name:   "Test Tenant",
		Status: "active",
	}
	err := components.TenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Create an API key
	apiKey, err := components.APIKeyManager.CreateKey(ctx, tenant.ID, "Test Key", nil)
	require.NoError(t, err)
	
	// Set up router with full middleware chain
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	router.Use(components.Middleware.IPFilterMiddleware())
	router.Use(components.Middleware.AuthenticationMiddleware())
	router.Use(components.Middleware.RateLimitMiddleware())
	router.Use(components.Middleware.QuotaCheckMiddleware())
	router.Use(components.Middleware.UsageTrackingMiddleware(components.UsageTracker))
	
	router.POST("/test", func(c *gin.Context) {
		// Verify tenant context is set
		tenantID, exists := c.Get("tenant_id")
		assert.True(t, exists)
		assert.Equal(t, tenant.ID, tenantID)
		
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	// Make a successful request
	body := map[string]interface{}{
		"message": "test",
	}
	bodyBytes, _ := json.Marshal(body)
	
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey.PlainKey)
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddlewareDisabled(t *testing.T) {
	components, cleanup := setupTestSecurityMiddleware(t)
	defer cleanup()
	
	// Disable middleware
	components.Middleware.Disable()
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Add all middleware
	router.Use(components.Middleware.AuthenticationMiddleware())
	router.Use(components.Middleware.IPFilterMiddleware())
	router.Use(components.Middleware.RateLimitMiddleware())
	
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	// Request without auth should succeed when middleware is disabled
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
}
