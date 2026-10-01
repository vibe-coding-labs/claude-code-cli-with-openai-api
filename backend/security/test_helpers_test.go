package security

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// setupFullSecurityTestDB creates a test database with every table used across
// the security package (tenants, api keys, quotas, rate limits, ip rules,
// usage records, audit logs, alerts, pricing tiers). Using a single shared
// schema lets tests for different components (e.g. HMAC verifier needing
// api_keys, IP filter needing ip_rules) be written without duplicating schema
// setup in every file.
func setupFullSecurityTestDB(t *testing.T) *sql.DB {
	dbPath := fmt.Sprintf("test_full_security_%s.db", t.Name())
	os.Remove(dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	schemas := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)`,
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
		`CREATE TABLE IF NOT EXISTS rate_limits (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			dimension TEXT NOT NULL,
			algorithm TEXT NOT NULL,
			[limit] INTEGER NOT NULL,
			window INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ip_rules (
			id TEXT PRIMARY KEY,
			tenant_id TEXT,
			rule_type TEXT NOT NULL,
			ip_address TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
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
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
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
		)`,
		`CREATE TABLE IF NOT EXISTS alert_configs (
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
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			type TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			details TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			sent_at TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS pricing_tiers (
			tenant_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			prompt_token_price REAL NOT NULL,
			completion_token_price REAL NOT NULL,
			request_price REAL NOT NULL,
			volume_discount_enabled BOOLEAN NOT NULL DEFAULT 0,
			volume_discount_tiers TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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
