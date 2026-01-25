package database

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateOrUpdateHealthStatus creates or updates a health status record
func CreateOrUpdateHealthStatus(status *HealthStatus) error {
	query := `
		INSERT INTO health_statuses (
			config_id, status, last_check_time, consecutive_successes,
			consecutive_failures, last_error, response_time_ms, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(config_id) DO UPDATE SET
			status = excluded.status,
			last_check_time = excluded.last_check_time,
			consecutive_successes = excluded.consecutive_successes,
			consecutive_failures = excluded.consecutive_failures,
			last_error = excluded.last_error,
			response_time_ms = excluded.response_time_ms,
			updated_at = datetime('now')
	`

	_, err := DB.Exec(query,
		status.ConfigID, status.Status, status.LastCheckTime,
		status.ConsecutiveSuccesses, status.ConsecutiveFailures,
		status.LastError, status.ResponseTimeMs,
	)

	if err != nil {
		return fmt.Errorf("failed to create/update health status: %w", err)
	}

	return nil
}

// GetHealthStatus retrieves the health status for a configuration node
func GetHealthStatus(configID string) (*HealthStatus, error) {
	query := `
		SELECT config_id, status, last_check_time, consecutive_successes,
			consecutive_failures, last_error, response_time_ms, created_at, updated_at
		FROM health_statuses WHERE config_id = ?
	`

	status := &HealthStatus{}
	var lastError sql.NullString
	err := DB.QueryRow(query, configID).Scan(
		&status.ConfigID, &status.Status, &status.LastCheckTime,
		&status.ConsecutiveSuccesses, &status.ConsecutiveFailures,
		&lastError, &status.ResponseTimeMs, &status.CreatedAt, &status.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("health status not found")
		}
		return nil, fmt.Errorf("failed to query health status: %w", err)
	}

	if lastError.Valid {
		status.LastError = lastError.String
	}

	return status, nil
}

// GetAllHealthStatuses retrieves all health statuses
func GetAllHealthStatuses() ([]*HealthStatus, error) {
	query := `
		SELECT config_id, status, last_check_time, consecutive_successes,
			consecutive_failures, last_error, response_time_ms, created_at, updated_at
		FROM health_statuses ORDER BY last_check_time DESC
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query health statuses: %w", err)
	}
	defer rows.Close()

	var statuses []*HealthStatus
	for rows.Next() {
		status := &HealthStatus{}
		var lastError sql.NullString
		err := rows.Scan(
			&status.ConfigID, &status.Status, &status.LastCheckTime,
			&status.ConsecutiveSuccesses, &status.ConsecutiveFailures,
			&lastError, &status.ResponseTimeMs, &status.CreatedAt, &status.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan health status: %w", err)
		}

		if lastError.Valid {
			status.LastError = lastError.String
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// GetHealthStatusesByLoadBalancer retrieves health statuses for all nodes in a load balancer
func GetHealthStatusesByLoadBalancer(loadBalancerID string) ([]*HealthStatus, error) {
	// First get the load balancer to get its config nodes
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer: %w", err)
	}

	var statuses []*HealthStatus
	for _, node := range lb.ConfigNodes {
		status, err := GetHealthStatus(node.ConfigID)
		if err != nil {
			// If health status doesn't exist, create a default one
			status = &HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "unknown",
				LastCheckTime: time.Now(),
			}
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// DeleteHealthStatus deletes a health status record
func DeleteHealthStatus(configID string) error {
	_, err := DB.Exec("DELETE FROM health_statuses WHERE config_id = ?", configID)
	if err != nil {
		return fmt.Errorf("failed to delete health status: %w", err)
	}
	return nil
}

// UpdateHealthStatusOnSuccess updates health status after a successful check
func UpdateHealthStatusOnSuccess(configID string, responseTimeMs int) error {
	status, err := GetHealthStatus(configID)
	if err != nil {
		// Create new status if it doesn't exist
		status = &HealthStatus{
			ConfigID:             configID,
			Status:               "healthy",
			LastCheckTime:        time.Now(),
			ConsecutiveSuccesses: 1,
			ConsecutiveFailures:  0,
			ResponseTimeMs:       responseTimeMs,
		}
		return CreateOrUpdateHealthStatus(status)
	}

	// Update existing status
	status.LastCheckTime = time.Now()
	status.ConsecutiveSuccesses++
	status.ConsecutiveFailures = 0
	status.ResponseTimeMs = responseTimeMs
	status.LastError = ""

	// Mark as healthy if it was unhealthy and reached recovery threshold
	// This will be handled by the health checker logic

	return CreateOrUpdateHealthStatus(status)
}

// UpdateHealthStatusOnFailure updates health status after a failed check
func UpdateHealthStatusOnFailure(configID string, errorMsg string) error {
	status, err := GetHealthStatus(configID)
	if err != nil {
		// Create new status if it doesn't exist
		status = &HealthStatus{
			ConfigID:             configID,
			Status:               "unhealthy",
			LastCheckTime:        time.Now(),
			ConsecutiveSuccesses: 0,
			ConsecutiveFailures:  1,
			LastError:            errorMsg,
		}
		return CreateOrUpdateHealthStatus(status)
	}

	// Update existing status
	status.LastCheckTime = time.Now()
	status.ConsecutiveSuccesses = 0
	status.ConsecutiveFailures++
	status.LastError = errorMsg

	// Mark as unhealthy if it was healthy and reached failure threshold
	// This will be handled by the health checker logic

	return CreateOrUpdateHealthStatus(status)
}
