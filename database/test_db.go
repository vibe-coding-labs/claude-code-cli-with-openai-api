package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// TestDB wraps a test database connection
type TestDB struct {
	db *sql.DB
}

// InitTestDB initializes an in-memory SQLite database for testing
func InitTestDB() (*TestDB, error) {
	// Open in-memory database
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open test database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping test database: %w", err)
	}

	// Set global DB for compatibility with existing code
	DB = db

	// Create base tables (this also runs migrations)
	if err := createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &TestDB{db: db}, nil
}

// GetDB returns the underlying database connection
func (tdb *TestDB) GetDB() *sql.DB {
	return tdb.db
}

// Close closes the test database connection
func (tdb *TestDB) Close() error {
	if tdb.db != nil {
		return tdb.db.Close()
	}
	return nil
}
