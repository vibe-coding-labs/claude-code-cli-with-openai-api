package client

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	_ "github.com/mattn/go-sqlite3"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		err        error
		want       string
	}{
		{"network timeout", 0, fmt.Errorf("connection timeout exceeded"), database.ErrorCategoryTimeout},
		{"connection refused", 0, fmt.Errorf("connection refused"), database.ErrorCategoryNetwork},
		{"no such host", 0, fmt.Errorf("no such host"), database.ErrorCategoryNetwork},
		{"auth 401", 401, nil, database.ErrorCategoryAuth},
		{"auth 403", 403, nil, database.ErrorCategoryAuth},
		{"rate limit 429", 429, nil, database.ErrorCategoryRateLimit},
		{"upstream 500", 500, nil, database.ErrorCategoryUpstream},
		{"upstream 502", 502, nil, database.ErrorCategoryUpstream},
		{"protocol 400", 400, nil, database.ErrorCategoryProtocol},
		{"unknown", 0, nil, database.ErrorCategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.statusCode, tt.err)
			if got != tt.want {
				t.Errorf("classifyError(%d, %v) = %q, want %q", tt.statusCode, tt.err, got, tt.want)
			}
		})
	}
}

func TestLogProxyError_SkipsWithoutConfigID(t *testing.T) {
	cfg := &config.Config{}
	c := NewOpenAIClient(cfg)
	// Should not panic or error when ConfigID is empty
	c.logProxyError("test-model", "", 500, nil, "test error", "", database.StageRequest, 0, 100, "")
}

func TestLogProxyError_RecordsToDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS proxy_errors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		config_id TEXT NOT NULL,
		config_name TEXT NOT NULL,
		session_id TEXT,
		request_id TEXT,
		model TEXT NOT NULL,
		upstream_model TEXT,
		error_type TEXT NOT NULL,
		error_category TEXT NOT NULL,
		error_message TEXT NOT NULL,
		upstream_status_code INTEGER,
		upstream_error_body TEXT,
		request_stage TEXT NOT NULL,
		retry_attempt INTEGER DEFAULT 0,
		request_duration_ms INTEGER,
		request_preview TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	database.DB = db

	cfg := &config.Config{
		ConfigID:   "test-config-123",
		ConfigName: "Test Config",
	}
	c := NewOpenAIClient(cfg)

	c.logProxyError("gpt-4o", "gpt-4o", 500, nil, "Internal Server Error", `{"error":"server error"}`, database.StageRequest, 2, 1500, "test preview")

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM proxy_errors WHERE config_id = ?", "test-config-123").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 error record, got %d", count)
	}

	var errorCategory, errorMsg string
	var statusCode int
	err = db.QueryRow("SELECT error_category, error_message, upstream_status_code FROM proxy_errors WHERE config_id = ?", "test-config-123").Scan(&errorCategory, &errorMsg, &statusCode)
	if err != nil {
		t.Fatalf("Failed to query error details: %v", err)
	}
	if errorCategory != database.ErrorCategoryUpstream {
		t.Errorf("error_category = %q, want %q", errorCategory, database.ErrorCategoryUpstream)
	}
	if errorMsg != "Internal Server Error" {
		t.Errorf("error_message = %q, want %q", errorMsg, "Internal Server Error")
	}
	if statusCode != 500 {
		t.Errorf("upstream_status_code = %d, want 500", statusCode)
	}
}
