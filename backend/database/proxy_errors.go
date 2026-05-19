package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ProxyError represents a structured proxy error record
type ProxyError struct {
	ID                int64  `json:"id"`
	ConfigID          string `json:"config_id"`
	ConfigName        string `json:"config_name"`
	SessionID         string `json:"session_id,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	Model             string `json:"model"`
	UpstreamModel     string `json:"upstream_model,omitempty"`
	ErrorType         string `json:"error_type"`
	ErrorCategory     string `json:"error_category"`
	ErrorMessage      string `json:"error_message"`
	UpstreamStatusCode int   `json:"upstream_status_code,omitempty"`
	UpstreamErrorBody string `json:"upstream_error_body,omitempty"`
	RequestStage      string `json:"request_stage"`
	RetryAttempt      int    `json:"retry_attempt"`
	RequestDurationMs int64  `json:"request_duration_ms,omitempty"`
	RequestPreview    string `json:"request_preview,omitempty"`
	CreatedAt         string `json:"created_at"`
}

// Error categories
const (
	ErrorCategoryNetwork   = "network"
	ErrorCategoryUpstream  = "upstream"
	ErrorCategoryProtocol  = "protocol"
	ErrorCategoryConfig    = "config"
	ErrorCategoryTimeout   = "timeout"
	ErrorCategoryAuth      = "auth"
	ErrorCategoryRateLimit = "rate_limit"
	ErrorCategoryUnknown   = "unknown"
)

// Request stages
const (
	StageConversion  = "conversion"
	StageRequest     = "request"
	StageStreaming   = "streaming"
	StageResponse    = "response"
	StageValidation  = "validation"
)

// LogProxyError inserts a proxy error record into the database
func LogProxyError(err *ProxyError) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	// Truncate long fields
	if len(err.ErrorMessage) > 2000 {
		err.ErrorMessage = err.ErrorMessage[:2000]
	}
	if len(err.UpstreamErrorBody) > 4000 {
		err.UpstreamErrorBody = err.UpstreamErrorBody[:4000]
	}
	if len(err.RequestPreview) > 2000 {
		err.RequestPreview = err.RequestPreview[:2000]
	}

	_, dbErr := DB.Exec(`INSERT INTO proxy_errors
		(config_id, config_name, session_id, request_id, model, upstream_model,
		 error_type, error_category, error_message, upstream_status_code,
		 upstream_error_body, request_stage, retry_attempt, request_duration_ms,
		 request_preview, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		err.ConfigID, err.ConfigName, err.SessionID, err.RequestID,
		err.Model, err.UpstreamModel, err.ErrorType, err.ErrorCategory,
		err.ErrorMessage, err.UpstreamStatusCode, err.UpstreamErrorBody,
		err.RequestStage, err.RetryAttempt, err.RequestDurationMs,
		err.RequestPreview, time.Now().UTC().Format(time.RFC3339),
	)
	return dbErr
}

// GetProxyErrors retrieves recent proxy errors with optional filters
func GetProxyErrors(configID, errorCategory, model string, limit, offset int) ([]ProxyError, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var conditions []string
	var args []interface{}

	if configID != "" {
		conditions = append(conditions, "config_id = ?")
		args = append(args, configID)
	}
	if errorCategory != "" {
		conditions = append(conditions, "error_category = ?")
		args = append(args, errorCategory)
	}
	if model != "" {
		conditions = append(conditions, "model LIKE ?")
		args = append(args, "%"+model+"%")
	}

	query := "SELECT id, config_id, config_name, COALESCE(session_id,''), COALESCE(request_id,''), model, COALESCE(upstream_model,''), error_type, error_category, error_message, COALESCE(upstream_status_code,0), COALESCE(upstream_error_body,''), request_stage, retry_attempt, COALESCE(request_duration_ms,0), COALESCE(request_preview,''), created_at FROM proxy_errors"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy errors: %w", err)
	}
	defer rows.Close()

	var errors []ProxyError
	for rows.Next() {
		var e ProxyError
		var createdAt string
		err := rows.Scan(&e.ID, &e.ConfigID, &e.ConfigName, &e.SessionID, &e.RequestID,
			&e.Model, &e.UpstreamModel, &e.ErrorType, &e.ErrorCategory, &e.ErrorMessage,
			&e.UpstreamStatusCode, &e.UpstreamErrorBody, &e.RequestStage, &e.RetryAttempt,
			&e.RequestDurationMs, &e.RequestPreview, &createdAt)
		if err != nil {
			continue
		}
		e.CreatedAt = createdAt
		errors = append(errors, e)
	}
	return errors, nil
}

// GetProxyErrorStats returns error counts grouped by category and config
func GetProxyErrorStats(configID string, since time.Time) ([]map[string]interface{}, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT config_id, config_name, error_category, COUNT(*) as count
		FROM proxy_errors
		WHERE created_at >= ?`
	args := []interface{}{since.UTC().Format(time.RFC3339)}

	if configID != "" {
		query += " AND config_id = ?"
		args = append(args, configID)
	}
	query += " GROUP BY config_id, config_name, error_category ORDER BY count DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query error stats: %w", err)
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var cfgID, cfgName, category string
		var count int
		if err := rows.Scan(&cfgID, &cfgName, &category, &count); err != nil {
			continue
		}
		stats = append(stats, map[string]interface{}{
			"config_id":     cfgID,
			"config_name":   cfgName,
			"error_category": category,
			"count":         count,
		})
	}
	return stats, nil
}

// CleanupOldProxyErrors removes error records older than the specified duration
func CleanupOldProxyErrors(olderThan time.Duration) (int64, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	result, err := DB.Exec("DELETE FROM proxy_errors WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup proxy errors: %w", err)
	}
	return result.RowsAffected()
}

// GetProxyErrorCount returns total error count for a config in a time window
func GetProxyErrorCount(configID string, since time.Time) (int, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var count int
	err := DB.QueryRow(
		"SELECT COUNT(*) FROM proxy_errors WHERE config_id = ? AND created_at >= ?",
		configID, since.UTC().Format(time.RFC3339),
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// GetProxyErrorsByRequestID returns all proxy errors for a specific request ID.
// Used to correlate errors across the request lifecycle (e.g., multiple 429 retries).
func GetProxyErrorsByRequestID(requestID string) ([]ProxyError, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := "SELECT id, config_id, config_name, COALESCE(session_id,''), COALESCE(request_id,''), model, COALESCE(upstream_model,''), error_type, error_category, error_message, COALESCE(upstream_status_code,0), COALESCE(upstream_error_body,''), request_stage, retry_attempt, COALESCE(request_duration_ms,0), COALESCE(request_preview,''), created_at FROM proxy_errors WHERE request_id = ? ORDER BY created_at ASC"

	rows, err := DB.Query(query, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy errors by request_id: %w", err)
	}
	defer rows.Close()

	var errors []ProxyError
	for rows.Next() {
		var e ProxyError
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ConfigID, &e.ConfigName, &e.SessionID, &e.RequestID, &e.Model, &e.UpstreamModel, &e.ErrorType, &e.ErrorCategory, &e.ErrorMessage, &e.UpstreamStatusCode, &e.UpstreamErrorBody, &e.RequestStage, &e.RetryAttempt, &e.RequestDurationMs, &e.RequestPreview, &createdAt); err != nil {
			continue
		}
		e.CreatedAt = createdAt
		errors = append(errors, e)
	}
	return errors, nil
}

// GetRecentProxyErrors returns the most recent proxy errors across all configs.
// Useful for dashboard "recent errors" views.
func GetRecentProxyErrors(limit int) ([]ProxyError, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := "SELECT id, config_id, config_name, COALESCE(session_id,''), COALESCE(request_id,''), model, COALESCE(upstream_model,''), error_type, error_category, error_message, COALESCE(upstream_status_code,0), COALESCE(upstream_error_body,''), request_stage, retry_attempt, COALESCE(request_duration_ms,0), COALESCE(request_preview,''), created_at FROM proxy_errors ORDER BY created_at DESC LIMIT ?"

	rows, err := DB.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent proxy errors: %w", err)
	}
	defer rows.Close()

	var errors []ProxyError
	for rows.Next() {
		var e ProxyError
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ConfigID, &e.ConfigName, &e.SessionID, &e.RequestID, &e.Model, &e.UpstreamModel, &e.ErrorType, &e.ErrorCategory, &e.ErrorMessage, &e.UpstreamStatusCode, &e.UpstreamErrorBody, &e.RequestStage, &e.RetryAttempt, &e.RequestDurationMs, &e.RequestPreview, &createdAt); err != nil {
			continue
		}
		e.CreatedAt = createdAt
		errors = append(errors, e)
	}
	return errors, nil
}
