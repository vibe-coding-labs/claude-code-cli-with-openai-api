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
		max_tokens_limit INTEGER DEFAULT 4096,
		request_timeout INTEGER DEFAULT 90,
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
	return nil
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
