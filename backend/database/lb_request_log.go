package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateLoadBalancerRequestLog creates a new load balancer request log
func CreateLoadBalancerRequestLog(log *LoadBalancerRequestLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	query := `
		INSERT INTO load_balancer_request_logs (
			id, load_balancer_id, selected_config_id, request_time, response_time,
			duration_ms, status_code, success, retry_count, error_message,
			request_summary, response_preview, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := DB.Exec(query,
		log.ID, log.LoadBalancerID, log.SelectedConfigID, log.RequestTime,
		log.ResponseTime, log.DurationMs, log.StatusCode, log.Success,
		log.RetryCount, log.ErrorMessage, log.RequestSummary,
		log.ResponsePreview,
	)

	if err != nil {
		return fmt.Errorf("failed to create load balancer request log: %w", err)
	}

	return nil
}

// GetLoadBalancerRequestLog retrieves a load balancer request log by ID
func GetLoadBalancerRequestLog(id string) (*LoadBalancerRequestLog, error) {
	query := `
		SELECT id, load_balancer_id, selected_config_id, request_time, response_time,
			duration_ms, status_code, success, retry_count, error_message,
			request_summary, response_preview, created_at
		FROM load_balancer_request_logs WHERE id = ?
	`

	log := &LoadBalancerRequestLog{}
	var errorMessage, requestSummary, responsePreview sql.NullString
	err := DB.QueryRow(query, id).Scan(
		&log.ID, &log.LoadBalancerID, &log.SelectedConfigID, &log.RequestTime,
		&log.ResponseTime, &log.DurationMs, &log.StatusCode, &log.Success,
		&log.RetryCount, &errorMessage, &requestSummary, &responsePreview,
		&log.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("load balancer request log not found")
		}
		return nil, fmt.Errorf("failed to query load balancer request log: %w", err)
	}

	if errorMessage.Valid {
		log.ErrorMessage = errorMessage.String
	}
	if requestSummary.Valid {
		log.RequestSummary = requestSummary.String
	}
	if responsePreview.Valid {
		log.ResponsePreview = responsePreview.String
	}

	return log, nil
}

// GetLoadBalancerRequestLogs retrieves request logs for a load balancer with pagination
func GetLoadBalancerRequestLogs(loadBalancerID string, limit, offset int) ([]*LoadBalancerRequestLog, error) {
	query := `
		SELECT id, load_balancer_id, selected_config_id, request_time, response_time,
			duration_ms, status_code, success, retry_count, error_message,
			request_summary, response_preview, created_at
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ?
		ORDER BY request_time DESC
		LIMIT ? OFFSET ?
	`

	rows, err := DB.Query(query, loadBalancerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query load balancer request logs: %w", err)
	}
	defer rows.Close()

	var logs []*LoadBalancerRequestLog
	for rows.Next() {
		log := &LoadBalancerRequestLog{}
		var errorMessage, requestSummary, responsePreview sql.NullString
		err := rows.Scan(
			&log.ID, &log.LoadBalancerID, &log.SelectedConfigID, &log.RequestTime,
			&log.ResponseTime, &log.DurationMs, &log.StatusCode, &log.Success,
			&log.RetryCount, &errorMessage, &requestSummary, &responsePreview,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan load balancer request log: %w", err)
		}

		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}
		if requestSummary.Valid {
			log.RequestSummary = requestSummary.String
		}
		if responsePreview.Valid {
			log.ResponsePreview = responsePreview.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// GetLoadBalancerRequestLogsByTimeRange retrieves request logs within a time range
func GetLoadBalancerRequestLogsByTimeRange(loadBalancerID string, startTime, endTime time.Time, limit int) ([]*LoadBalancerRequestLog, error) {
	query := `
		SELECT id, load_balancer_id, selected_config_id, request_time, response_time,
			duration_ms, status_code, success, retry_count, error_message,
			request_summary, response_preview, created_at
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND request_time >= ? AND request_time <= ?
		ORDER BY request_time DESC
		LIMIT ?
	`

	rows, err := DB.Query(query, loadBalancerID, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query load balancer request logs: %w", err)
	}
	defer rows.Close()

	var logs []*LoadBalancerRequestLog
	for rows.Next() {
		log := &LoadBalancerRequestLog{}
		var errorMessage, requestSummary, responsePreview sql.NullString
		err := rows.Scan(
			&log.ID, &log.LoadBalancerID, &log.SelectedConfigID, &log.RequestTime,
			&log.ResponseTime, &log.DurationMs, &log.StatusCode, &log.Success,
			&log.RetryCount, &errorMessage, &requestSummary, &responsePreview,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan load balancer request log: %w", err)
		}

		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}
		if requestSummary.Valid {
			log.RequestSummary = requestSummary.String
		}
		if responsePreview.Valid {
			log.ResponsePreview = responsePreview.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// DeleteOldLoadBalancerRequestLogs deletes request logs older than the specified days
func DeleteOldLoadBalancerRequestLogs(days int) error {
	query := `
		DELETE FROM load_balancer_request_logs
		WHERE request_time < datetime('now', '-' || ? || ' days')
	`

	result, err := DB.Exec(query, days)
	if err != nil {
		return fmt.Errorf("failed to delete old load balancer request logs: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("Deleted %d old load balancer request logs\n", rowsAffected)
	}

	return nil
}

// CountLoadBalancerRequestLogs counts the total number of request logs for a load balancer
func CountLoadBalancerRequestLogs(loadBalancerID string) (int64, error) {
	query := `SELECT COUNT(*) FROM load_balancer_request_logs WHERE load_balancer_id = ?`

	var count int64
	err := DB.QueryRow(query, loadBalancerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count load balancer request logs: %w", err)
	}

	return count, nil
}

// BatchCreateLoadBalancerRequestLogs creates multiple load balancer request logs in a single transaction
func BatchCreateLoadBalancerRequestLogs(logs []*LoadBalancerRequestLog) error {
	if len(logs) == 0 {
		return nil
	}

	// Start transaction
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement
	query := `
		INSERT INTO load_balancer_request_logs (
			id, load_balancer_id, selected_config_id, request_time, response_time,
			duration_ms, status_code, success, retry_count, error_message,
			request_summary, response_preview, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert all logs
	for _, log := range logs {
		if log.ID == "" {
			log.ID = uuid.New().String()
		}

		_, err := stmt.Exec(
			log.ID, log.LoadBalancerID, log.SelectedConfigID, log.RequestTime,
			log.ResponseTime, log.DurationMs, log.StatusCode, log.Success,
			log.RetryCount, log.ErrorMessage, log.RequestSummary,
			log.ResponsePreview,
		)

		if err != nil {
			return fmt.Errorf("failed to insert log: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
