package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite works better with a single connection
	db.SetMaxIdleConns(1)

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

	// Create indexes for better query performance
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_token_stats_config_id ON token_stats(config_id);`,
		`CREATE INDEX IF NOT EXISTS idx_token_stats_created_at ON token_stats(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_config_id ON request_logs(config_id);`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);`,
	}

	// Execute table creation
	tables := []string{apiConfigsTable, tokenStatsTable, requestLogsTable}
	for _, table := range tables {
		if _, err := DB.Exec(table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Create indexes
	for _, index := range indexes {
		if _, err := DB.Exec(index); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Println("✅ Database tables created successfully")

	// Create users table
	if err := CreateUserTable(); err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Run migrations
	if err := runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
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
