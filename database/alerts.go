package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateAlert creates a new alert
func CreateAlert(alert *Alert) error {
	if alert.ID == "" {
		alert.ID = uuid.New().String()
	}

	query := `
		INSERT INTO alerts (
			id, load_balancer_id, level, type, message, details,
			acknowledged, acknowledged_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := DB.Exec(query,
		alert.ID, alert.LoadBalancerID, alert.Level, alert.Type,
		alert.Message, alert.Details, alert.Acknowledged, alert.AcknowledgedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create alert: %w", err)
	}

	return nil
}

// GetAlert retrieves an alert by ID
func GetAlert(id string) (*Alert, error) {
	query := `
		SELECT id, load_balancer_id, level, type, message, details,
			acknowledged, acknowledged_at, created_at
		FROM alerts WHERE id = ?
	`

	alert := &Alert{}
	var details sql.NullString
	var acknowledgedAt sql.NullTime
	err := DB.QueryRow(query, id).Scan(
		&alert.ID, &alert.LoadBalancerID, &alert.Level, &alert.Type,
		&alert.Message, &details, &alert.Acknowledged, &acknowledgedAt,
		&alert.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("alert not found")
		}
		return nil, fmt.Errorf("failed to query alert: %w", err)
	}

	if details.Valid {
		alert.Details = details.String
	}
	if acknowledgedAt.Valid {
		alert.AcknowledgedAt = &acknowledgedAt.Time
	}

	return alert, nil
}

// GetAlertsByLoadBalancer retrieves alerts for a load balancer
func GetAlertsByLoadBalancer(loadBalancerID string, acknowledged *bool, limit int) ([]*Alert, error) {
	query := `
		SELECT id, load_balancer_id, level, type, message, details,
			acknowledged, acknowledged_at, created_at
		FROM alerts
		WHERE load_balancer_id = ?
	`

	args := []interface{}{loadBalancerID}

	if acknowledged != nil {
		query += ` AND acknowledged = ?`
		args = append(args, *acknowledged)
	}

	query += ` ORDER BY created_at DESC`

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		alert := &Alert{}
		var details sql.NullString
		var acknowledgedAt sql.NullTime
		err := rows.Scan(
			&alert.ID, &alert.LoadBalancerID, &alert.Level, &alert.Type,
			&alert.Message, &details, &alert.Acknowledged, &acknowledgedAt,
			&alert.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alert: %w", err)
		}

		if details.Valid {
			alert.Details = details.String
		}
		if acknowledgedAt.Valid {
			alert.AcknowledgedAt = &acknowledgedAt.Time
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// GetAllAlerts retrieves all alerts with optional filtering
func GetAllAlerts(acknowledged *bool, level string, limit int) ([]*Alert, error) {
	query := `
		SELECT id, load_balancer_id, level, type, message, details,
			acknowledged, acknowledged_at, created_at
		FROM alerts
		WHERE 1=1
	`

	args := []interface{}{}

	if acknowledged != nil {
		query += ` AND acknowledged = ?`
		args = append(args, *acknowledged)
	}

	if level != "" {
		query += ` AND level = ?`
		args = append(args, level)
	}

	query += ` ORDER BY created_at DESC`

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		alert := &Alert{}
		var details sql.NullString
		var acknowledgedAt sql.NullTime
		err := rows.Scan(
			&alert.ID, &alert.LoadBalancerID, &alert.Level, &alert.Type,
			&alert.Message, &details, &alert.Acknowledged, &acknowledgedAt,
			&alert.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alert: %w", err)
		}

		if details.Valid {
			alert.Details = details.String
		}
		if acknowledgedAt.Valid {
			alert.AcknowledgedAt = &acknowledgedAt.Time
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// AcknowledgeAlert marks an alert as acknowledged
func AcknowledgeAlert(id string) error {
	now := time.Now()
	query := `
		UPDATE alerts
		SET acknowledged = 1, acknowledged_at = ?
		WHERE id = ?
	`

	_, err := DB.Exec(query, now, id)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}

	return nil
}

// DeleteAlert deletes an alert
func DeleteAlert(id string) error {
	_, err := DB.Exec("DELETE FROM alerts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete alert: %w", err)
	}
	return nil
}

// DeleteOldAlerts deletes alerts older than the specified days
func DeleteOldAlerts(days int) error {
	query := `
		DELETE FROM alerts
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`

	result, err := DB.Exec(query, days)
	if err != nil {
		return fmt.Errorf("failed to delete old alerts: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("Deleted %d old alerts\n", rowsAffected)
	}

	return nil
}

// CountUnacknowledgedAlerts counts unacknowledged alerts for a load balancer
func CountUnacknowledgedAlerts(loadBalancerID string) (int64, error) {
	query := `
		SELECT COUNT(*)
		FROM alerts
		WHERE load_balancer_id = ? AND acknowledged = 0
	`

	var count int64
	err := DB.QueryRow(query, loadBalancerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count unacknowledged alerts: %w", err)
	}

	return count, nil
}

// CheckAndCreateAllNodesDownAlert checks if all nodes are down and creates an alert
func CheckAndCreateAllNodesDownAlert(loadBalancerID string) error {
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return err
	}

	allDown := true
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}

		status, err := GetHealthStatus(node.ConfigID)
		if err == nil && status.Status == "healthy" {
			allDown = false
			break
		}
	}

	if allDown {
		// Check if alert already exists
		alerts, err := GetAlertsByLoadBalancer(loadBalancerID, boolPtr(false), 10)
		if err == nil {
			for _, alert := range alerts {
				if alert.Type == "all_nodes_down" {
					return nil // Alert already exists
				}
			}
		}

		// Create new alert
		alert := &Alert{
			LoadBalancerID: loadBalancerID,
			Level:          "critical",
			Type:           "all_nodes_down",
			Message:        "All configuration nodes are unhealthy",
			Details:        "All nodes in the load balancer are currently marked as unhealthy. Service may be unavailable.",
		}
		return CreateAlert(alert)
	}

	return nil
}

// CheckAndCreateLowHealthyNodesAlert checks if healthy nodes are below threshold
func CheckAndCreateLowHealthyNodesAlert(loadBalancerID string, minHealthyNodes int) error {
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return err
	}

	healthyCount := 0
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}

		status, err := GetHealthStatus(node.ConfigID)
		if err == nil && status.Status == "healthy" {
			healthyCount++
		}
	}

	if healthyCount < minHealthyNodes {
		// Check if alert already exists
		alerts, err := GetAlertsByLoadBalancer(loadBalancerID, boolPtr(false), 10)
		if err == nil {
			for _, alert := range alerts {
				if alert.Type == "low_healthy_nodes" {
					return nil // Alert already exists
				}
			}
		}

		// Create new alert
		alert := &Alert{
			LoadBalancerID: loadBalancerID,
			Level:          "warning",
			Type:           "low_healthy_nodes",
			Message:        fmt.Sprintf("Healthy nodes (%d) below threshold (%d)", healthyCount, minHealthyNodes),
			Details:        fmt.Sprintf("The number of healthy nodes is below the configured minimum threshold."),
		}
		return CreateAlert(alert)
	}

	return nil
}

// CheckAndCreateHighErrorRateAlert checks if error rate exceeds threshold
func CheckAndCreateHighErrorRateAlert(loadBalancerID string, errorRateThreshold float64, windowMinutes int) error {
	startTime := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

	query := `
		SELECT
			COUNT(*) as total_requests,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_requests
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND request_time >= ?
	`

	var totalRequests, failedRequests int64
	err := DB.QueryRow(query, loadBalancerID, startTime).Scan(&totalRequests, &failedRequests)
	if err != nil {
		return err
	}

	if totalRequests == 0 {
		return nil // No requests in window
	}

	errorRate := float64(failedRequests) / float64(totalRequests)
	if errorRate > errorRateThreshold {
		// Check if alert already exists
		alerts, err := GetAlertsByLoadBalancer(loadBalancerID, boolPtr(false), 10)
		if err == nil {
			for _, alert := range alerts {
				if alert.Type == "high_error_rate" {
					return nil // Alert already exists
				}
			}
		}

		// Create new alert
		alert := &Alert{
			LoadBalancerID: loadBalancerID,
			Level:          "warning",
			Type:           "high_error_rate",
			Message:        fmt.Sprintf("Error rate (%.2f%%) exceeds threshold (%.2f%%)", errorRate*100, errorRateThreshold*100),
			Details:        fmt.Sprintf("In the last %d minutes: %d failed out of %d total requests", windowMinutes, failedRequests, totalRequests),
		}
		return CreateAlert(alert)
	}

	return nil
}

// Helper function to create a bool pointer
func boolPtr(b bool) *bool {
	return &b
}
