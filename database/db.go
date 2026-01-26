package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

// InitDB initializes the SQLite database
func InitDB(dbPath string) error {
	// Create data directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open database connection with WAL mode for better concurrency
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for better concurrency
	// WAL mode allows multiple readers with one writer
	db.SetMaxOpenConns(25) // Allow up to 25 concurrent connections
	db.SetMaxIdleConns(5)  // Keep 5 idle connections ready
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	DB = db
	log.Printf("✅ Database initialized at: %s", dbPath)

	// Create tables
	if err := createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// createTables creates the necessary database tables
func createTables() error {
	// API configurations table
	apiConfigsTable := `
	CREATE TABLE IF NOT EXISTS api_configs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		user_id INTEGER,
		openai_api_key_encrypted TEXT NOT NULL,
		openai_base_url TEXT NOT NULL,
		big_model TEXT NOT NULL,
		middle_model TEXT NOT NULL,
		small_model TEXT NOT NULL,
		max_tokens_limit INTEGER DEFAULT 16384,
		request_timeout INTEGER DEFAULT 180,
		retry_count INTEGER DEFAULT 3,
		anthropic_api_key TEXT,
		enabled BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	// Token usage statistics table
	tokenStatsTable := `
	CREATE TABLE IF NOT EXISTS token_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		config_id TEXT NOT NULL,
		user_id INTEGER,
		model TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		request_count INTEGER DEFAULT 1,
		error_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
	);`

	// Request logs table for detailed tracking
	requestLogsTable := `
	CREATE TABLE IF NOT EXISTS request_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		config_id TEXT NOT NULL,
		user_id INTEGER,
		model TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		status TEXT NOT NULL,
		error_message TEXT,
		request_body TEXT,
		response_body TEXT,
		request_summary TEXT,
		response_preview TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
	);`

	// Load balancers table
	loadBalancersTable := `
	CREATE TABLE IF NOT EXISTS load_balancers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		user_id INTEGER,
		strategy TEXT NOT NULL,
		config_nodes TEXT NOT NULL,
		enabled BOOLEAN DEFAULT 1,
		anthropic_api_key TEXT UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	// Health statuses table for load balancer enhancements
	healthStatusesTable := `
	CREATE TABLE IF NOT EXISTS health_statuses (
		config_id TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		last_check_time DATETIME NOT NULL,
		consecutive_successes INTEGER DEFAULT 0,
		consecutive_failures INTEGER DEFAULT 0,
		last_error TEXT,
		response_time_ms INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
	);`

	// Circuit breaker states table
	circuitBreakerStatesTable := `
	CREATE TABLE IF NOT EXISTS circuit_breaker_states (
		config_id TEXT PRIMARY KEY,
		state TEXT NOT NULL,
		failure_count INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		last_state_change DATETIME NOT NULL,
		next_retry_time DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
	);`

	// Load balancer request logs table
	lbRequestLogsTable := `
	CREATE TABLE IF NOT EXISTS load_balancer_request_logs (
		id TEXT PRIMARY KEY,
		load_balancer_id TEXT NOT NULL,
		selected_config_id TEXT NOT NULL,
		request_time DATETIME NOT NULL,
		response_time DATETIME NOT NULL,
		duration_ms INTEGER NOT NULL,
		status_code INTEGER NOT NULL,
		success BOOLEAN NOT NULL,
		retry_count INTEGER DEFAULT 0,
		error_message TEXT,
		request_summary TEXT,
		response_preview TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE
	);`

	// Load balancer stats table
	lbStatsTable := `
	CREATE TABLE IF NOT EXISTS load_balancer_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		load_balancer_id TEXT NOT NULL,
		time_bucket DATETIME NOT NULL,
		total_requests INTEGER DEFAULT 0,
		success_requests INTEGER DEFAULT 0,
		failed_requests INTEGER DEFAULT 0,
		total_duration_ms INTEGER DEFAULT 0,
		min_duration_ms INTEGER DEFAULT 0,
		max_duration_ms INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE
	);`

	// Node stats table
	nodeStatsTable := `
	CREATE TABLE IF NOT EXISTS node_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		load_balancer_id TEXT NOT NULL,
		config_id TEXT NOT NULL,
		time_bucket DATETIME NOT NULL,
		request_count INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		failed_count INTEGER DEFAULT 0,
		total_duration_ms INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE,
		FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
	);`

	// Alerts table
	alertsTable := `
	CREATE TABLE IF NOT EXISTS alerts (
		id TEXT PRIMARY KEY,
		load_balancer_id TEXT NOT NULL,
		level TEXT NOT NULL,
		type TEXT NOT NULL,
		message TEXT NOT NULL,
		details TEXT,
		acknowledged BOOLEAN DEFAULT FALSE,
		acknowledged_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (load_balancer_id) REFERENCES load_balancers(id) ON DELETE CASCADE
	);`

	// Execute table creation
	tables := []string{
		apiConfigsTable,
		tokenStatsTable,
		requestLogsTable,
		loadBalancersTable,
		healthStatusesTable,
		circuitBreakerStatesTable,
		lbRequestLogsTable,
		lbStatsTable,
		nodeStatsTable,
		alertsTable,
	}
	for _, table := range tables {
		if _, err := DB.Exec(table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	log.Println("✅ Database tables created successfully")

	// Create users table
	if err := CreateUserTable(); err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Run legacy migrations (for backward compatibility)
	if err := runMigrations(); err != nil {
		return fmt.Errorf("failed to run legacy migrations: %w", err)
	}

	// Run new migration system
	if err := RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if err := createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

type indexDefinition struct {
	name    string
	table   string
	columns []string
	sql     string
}

func createIndexes() error {
	indexes := []indexDefinition{
		{name: "idx_token_stats_config_id", table: "token_stats", columns: []string{"config_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_token_stats_config_id ON token_stats(config_id);"},
		{name: "idx_token_stats_user_id", table: "token_stats", columns: []string{"user_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_token_stats_user_id ON token_stats(user_id);"},
		{name: "idx_token_stats_created_at", table: "token_stats", columns: []string{"created_at"}, sql: "CREATE INDEX IF NOT EXISTS idx_token_stats_created_at ON token_stats(created_at);"},
		{name: "idx_request_logs_config_id", table: "request_logs", columns: []string{"config_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_request_logs_config_id ON request_logs(config_id);"},
		{name: "idx_request_logs_user_id", table: "request_logs", columns: []string{"user_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON request_logs(user_id);"},
		{name: "idx_request_logs_created_at", table: "request_logs", columns: []string{"created_at"}, sql: "CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);"},
		{name: "idx_api_configs_user_id", table: "api_configs", columns: []string{"user_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_api_configs_user_id ON api_configs(user_id);"},
		{name: "idx_users_role", table: "users", columns: []string{"role"}, sql: "CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);"},
		{name: "idx_users_status", table: "users", columns: []string{"status"}, sql: "CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);"},
		{name: "idx_users_created_at", table: "users", columns: []string{"created_at"}, sql: "CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);"},
		{name: "idx_load_balancers_api_key", table: "load_balancers", columns: []string{"anthropic_api_key"}, sql: "CREATE INDEX IF NOT EXISTS idx_load_balancers_api_key ON load_balancers(anthropic_api_key);"},
		{name: "idx_load_balancers_enabled", table: "load_balancers", columns: []string{"enabled"}, sql: "CREATE INDEX IF NOT EXISTS idx_load_balancers_enabled ON load_balancers(enabled);"},
		{name: "idx_load_balancers_user_id", table: "load_balancers", columns: []string{"user_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_load_balancers_user_id ON load_balancers(user_id);"},
		{name: "idx_lb_request_logs_lb_id", table: "load_balancer_request_logs", columns: []string{"load_balancer_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_lb_request_logs_lb_id ON load_balancer_request_logs(load_balancer_id);"},
		{name: "idx_lb_request_logs_time", table: "load_balancer_request_logs", columns: []string{"request_time"}, sql: "CREATE INDEX IF NOT EXISTS idx_lb_request_logs_time ON load_balancer_request_logs(request_time);"},
		{name: "idx_lb_stats_lb_id_time", table: "load_balancer_stats", columns: []string{"load_balancer_id", "time_bucket"}, sql: "CREATE INDEX IF NOT EXISTS idx_lb_stats_lb_id_time ON load_balancer_stats(load_balancer_id, time_bucket);"},
		{name: "idx_node_stats_lb_id_time", table: "node_stats", columns: []string{"load_balancer_id", "time_bucket"}, sql: "CREATE INDEX IF NOT EXISTS idx_node_stats_lb_id_time ON node_stats(load_balancer_id, time_bucket);"},
		{name: "idx_node_stats_config_id", table: "node_stats", columns: []string{"config_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_node_stats_config_id ON node_stats(config_id);"},
		{name: "idx_alerts_lb_id", table: "alerts", columns: []string{"load_balancer_id"}, sql: "CREATE INDEX IF NOT EXISTS idx_alerts_lb_id ON alerts(load_balancer_id);"},
		{name: "idx_alerts_acknowledged", table: "alerts", columns: []string{"acknowledged"}, sql: "CREATE INDEX IF NOT EXISTS idx_alerts_acknowledged ON alerts(acknowledged);"},
		{name: "idx_alerts_created_at", table: "alerts", columns: []string{"created_at"}, sql: "CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at);"},
	}

	for _, index := range indexes {
		columnsReady := true
		for _, column := range index.columns {
			exists, err := columnExists(index.table, column)
			if err != nil {
				return fmt.Errorf("failed to inspect table %s: %w", index.table, err)
			}
			if !exists {
				columnsReady = false
				break
			}
		}
		if !columnsReady {
			continue
		}
		if _, err := DB.Exec(index.sql); err != nil {
			return fmt.Errorf("failed to create index %s: %w", index.name, err)
		}
	}

	return nil
}

func columnExists(tableName, columnName string) (bool, error) {
	rows, err := DB.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}

	return false, nil
}

// runMigrations runs database migrations
func runMigrations() error {
	// 迁移1: 为 request_logs 添加新字段（如果不存在）
	migrations := []string{
		`ALTER TABLE request_logs ADD COLUMN request_body TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN response_body TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN request_summary TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN response_preview TEXT;`,
		`ALTER TABLE api_configs ADD COLUMN retry_count INTEGER DEFAULT 3;`,
		// 迁移2: 为 api_configs 添加 supported_models 字段（JSON格式存储模型列表）
		`ALTER TABLE api_configs ADD COLUMN supported_models TEXT;`,
		// 迁移3: 为 request_logs 添加客户端追踪字段
		`ALTER TABLE request_logs ADD COLUMN client_ip TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN user_agent TEXT;`,
		// 迁移4: 为 api_configs 添加密钥过期时间字段
		`ALTER TABLE api_configs ADD COLUMN expires_at DATETIME;`,
		// 迁移5: 为 load_balancers 添加健康检查配置字段
		`ALTER TABLE load_balancers ADD COLUMN health_check_enabled BOOLEAN DEFAULT 1;`,
		`ALTER TABLE load_balancers ADD COLUMN health_check_interval INTEGER DEFAULT 30;`,
		`ALTER TABLE load_balancers ADD COLUMN failure_threshold INTEGER DEFAULT 3;`,
		`ALTER TABLE load_balancers ADD COLUMN recovery_threshold INTEGER DEFAULT 2;`,
		`ALTER TABLE load_balancers ADD COLUMN health_check_timeout INTEGER DEFAULT 5;`,
		// 迁移6: 为 load_balancers 添加重试配置字段
		`ALTER TABLE load_balancers ADD COLUMN max_retries INTEGER DEFAULT 3;`,
		`ALTER TABLE load_balancers ADD COLUMN initial_retry_delay INTEGER DEFAULT 100;`,
		`ALTER TABLE load_balancers ADD COLUMN max_retry_delay INTEGER DEFAULT 5000;`,
		// 迁移7: 为 load_balancers 添加熔断器配置字段
		`ALTER TABLE load_balancers ADD COLUMN circuit_breaker_enabled BOOLEAN DEFAULT 1;`,
		`ALTER TABLE load_balancers ADD COLUMN error_rate_threshold REAL DEFAULT 0.5;`,
		`ALTER TABLE load_balancers ADD COLUMN circuit_breaker_window INTEGER DEFAULT 60;`,
		`ALTER TABLE load_balancers ADD COLUMN circuit_breaker_timeout INTEGER DEFAULT 30;`,
		`ALTER TABLE load_balancers ADD COLUMN half_open_requests INTEGER DEFAULT 3;`,
		// 迁移8: 为 load_balancers 添加动态权重配置字段
		`ALTER TABLE load_balancers ADD COLUMN dynamic_weight_enabled BOOLEAN DEFAULT 0;`,
		`ALTER TABLE load_balancers ADD COLUMN weight_update_interval INTEGER DEFAULT 300;`,
		// 迁移9: 为 load_balancers 添加日志配置字段
		`ALTER TABLE load_balancers ADD COLUMN log_level TEXT DEFAULT 'standard';`,
	}

	for _, migration := range migrations {
		// 忽略已存在的列错误
		_, err := DB.Exec(migration)
		if err != nil && !contains(err.Error(), "duplicate column name") {
			// 如果不是重复列名错误，则记录但继续
			log.Printf("⚠️  Migration warning: %v", err)
		}
	}

	return nil
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// IsInitialized checks if the database is initialized
func IsInitialized() bool {
	return DB != nil
}

// CloseDB closes the database connection
func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
