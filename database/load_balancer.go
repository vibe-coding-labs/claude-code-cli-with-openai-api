package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// CreateLoadBalancer creates a new load balancer
func CreateLoadBalancer(lb *LoadBalancer) error {
	if lb.ID == "" {
		lb.ID = uuid.New().String()
	}

	// Set Anthropic API Key if not provided
	if lb.AnthropicAPIKey == "" {
		lb.AnthropicAPIKey = uuid.New().String()
	} else {
		// Validate custom token
		if len(lb.AnthropicAPIKey) > 100 {
			return fmt.Errorf("custom API token must be 100 characters or less")
		}

		for _, ch := range lb.AnthropicAPIKey {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '_') {
				return fmt.Errorf("custom API token can only contain letters, numbers, and underscores")
			}
		}

		// Check uniqueness
		var count int
		checkQuery := `SELECT COUNT(*) FROM load_balancers WHERE anthropic_api_key = ?`
		err := DB.QueryRow(checkQuery, lb.AnthropicAPIKey).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check token uniqueness: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("this API token is already in use")
		}
	}

	// Serialize config_nodes to JSON
	configNodesJSON, err := json.Marshal(lb.ConfigNodes)
	if err != nil {
		return fmt.Errorf("failed to marshal config nodes: %w", err)
	}

	query := `
		INSERT INTO load_balancers (
			id, name, description, user_id, strategy, config_nodes,
			enabled, anthropic_api_key, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`

	_, err = DB.Exec(query,
		lb.ID, lb.Name, lb.Description, lb.UserID, lb.Strategy, string(configNodesJSON),
		lb.Enabled, lb.AnthropicAPIKey,
	)

	if err != nil {
		return fmt.Errorf("failed to insert load balancer: %w", err)
	}

	return nil
}

// GetLoadBalancer retrieves a load balancer by ID
func GetLoadBalancer(id string) (*LoadBalancer, error) {
	query := `
		SELECT id, name, description, user_id, strategy, config_nodes,
			enabled, anthropic_api_key, created_at, updated_at,
			health_check_enabled, health_check_interval, failure_threshold,
			recovery_threshold, health_check_timeout, max_retries,
			initial_retry_delay, max_retry_delay, circuit_breaker_enabled,
			error_rate_threshold, circuit_breaker_window, circuit_breaker_timeout,
			half_open_requests, dynamic_weight_enabled, weight_update_interval,
			log_level
		FROM load_balancers WHERE id = ?
	`

	lb := &LoadBalancer{}
	var configNodesJSON string
	var healthCheckEnabled, circuitBreakerEnabled, dynamicWeightEnabled sql.NullBool
	var healthCheckInterval, failureThreshold, recoveryThreshold, healthCheckTimeout sql.NullInt64
	var maxRetries, initialRetryDelay, maxRetryDelay sql.NullInt64
	var errorRateThreshold sql.NullFloat64
	var circuitBreakerWindow, circuitBreakerTimeout, halfOpenRequests sql.NullInt64
	var weightUpdateInterval sql.NullInt64
	var logLevel sql.NullString

	err := DB.QueryRow(query, id).Scan(
		&lb.ID, &lb.Name, &lb.Description, &lb.UserID, &lb.Strategy, &configNodesJSON,
		&lb.Enabled, &lb.AnthropicAPIKey, &lb.CreatedAt, &lb.UpdatedAt,
		&healthCheckEnabled, &healthCheckInterval, &failureThreshold,
		&recoveryThreshold, &healthCheckTimeout, &maxRetries,
		&initialRetryDelay, &maxRetryDelay, &circuitBreakerEnabled,
		&errorRateThreshold, &circuitBreakerWindow, &circuitBreakerTimeout,
		&halfOpenRequests, &dynamicWeightEnabled, &weightUpdateInterval,
		&logLevel,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("load balancer not found")
		}
		return nil, fmt.Errorf("failed to query load balancer: %w", err)
	}

	// Deserialize config_nodes from JSON
	if configNodesJSON != "" {
		err = json.Unmarshal([]byte(configNodesJSON), &lb.ConfigNodes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config nodes: %w", err)
		}
	}

	return lb, nil
}

// GetLoadBalancerByAPIKey retrieves a load balancer by Anthropic API key
func GetLoadBalancerByAPIKey(apiKey string) (*LoadBalancer, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("empty API key")
	}

	query := `
		SELECT id, name, description, user_id, strategy, config_nodes,
			enabled, anthropic_api_key, created_at, updated_at,
			health_check_enabled, health_check_interval, failure_threshold,
			recovery_threshold, health_check_timeout, max_retries,
			initial_retry_delay, max_retry_delay, circuit_breaker_enabled,
			error_rate_threshold, circuit_breaker_window, circuit_breaker_timeout,
			half_open_requests, dynamic_weight_enabled, weight_update_interval,
			log_level
		FROM load_balancers 
		WHERE anthropic_api_key = ? AND enabled = 1
		LIMIT 1
	`

	lb := &LoadBalancer{}
	var configNodesJSON string
	var healthCheckEnabled, circuitBreakerEnabled, dynamicWeightEnabled sql.NullBool
	var healthCheckInterval, failureThreshold, recoveryThreshold, healthCheckTimeout sql.NullInt64
	var maxRetries, initialRetryDelay, maxRetryDelay sql.NullInt64
	var errorRateThreshold sql.NullFloat64
	var circuitBreakerWindow, circuitBreakerTimeout, halfOpenRequests sql.NullInt64
	var weightUpdateInterval sql.NullInt64
	var logLevel sql.NullString

	err := DB.QueryRow(query, apiKey).Scan(
		&lb.ID, &lb.Name, &lb.Description, &lb.UserID, &lb.Strategy, &configNodesJSON,
		&lb.Enabled, &lb.AnthropicAPIKey, &lb.CreatedAt, &lb.UpdatedAt,
		&healthCheckEnabled, &healthCheckInterval, &failureThreshold,
		&recoveryThreshold, &healthCheckTimeout, &maxRetries,
		&initialRetryDelay, &maxRetryDelay, &circuitBreakerEnabled,
		&errorRateThreshold, &circuitBreakerWindow, &circuitBreakerTimeout,
		&halfOpenRequests, &dynamicWeightEnabled, &weightUpdateInterval,
		&logLevel,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("load balancer not found for API key")
		}
		return nil, fmt.Errorf("failed to query load balancer: %w", err)
	}

	// Deserialize config_nodes from JSON
	if configNodesJSON != "" {
		err = json.Unmarshal([]byte(configNodesJSON), &lb.ConfigNodes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config nodes: %w", err)
		}
	}

	return lb, nil
}

// GetAllLoadBalancers retrieves all load balancers
func GetAllLoadBalancers() ([]*LoadBalancer, error) {
	query := `
		SELECT id, name, description, user_id, strategy, config_nodes,
			enabled, anthropic_api_key, created_at, updated_at
		FROM load_balancers ORDER BY created_at DESC
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query load balancers: %w", err)
	}
	defer rows.Close()

	var loadBalancers []*LoadBalancer
	for rows.Next() {
		lb := &LoadBalancer{}
		var configNodesJSON string
		err := rows.Scan(
			&lb.ID, &lb.Name, &lb.Description, &lb.UserID, &lb.Strategy, &configNodesJSON,
			&lb.Enabled, &lb.AnthropicAPIKey, &lb.CreatedAt, &lb.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan load balancer: %w", err)
		}

		// Deserialize config_nodes from JSON
		if configNodesJSON != "" {
			err = json.Unmarshal([]byte(configNodesJSON), &lb.ConfigNodes)
			if err != nil {
				// Ignore deserialization errors, continue processing
				lb.ConfigNodes = nil
			}
		}

		loadBalancers = append(loadBalancers, lb)
	}

	return loadBalancers, nil
}

// GetLoadBalancersByUser retrieves load balancers for a user
func GetLoadBalancersByUser(userID int64) ([]*LoadBalancer, error) {
	query := `
		SELECT id, name, description, user_id, strategy, config_nodes,
			enabled, anthropic_api_key, created_at, updated_at
		FROM load_balancers
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query load balancers: %w", err)
	}
	defer rows.Close()

	var loadBalancers []*LoadBalancer
	for rows.Next() {
		lb := &LoadBalancer{}
		var configNodesJSON string
		err := rows.Scan(
			&lb.ID, &lb.Name, &lb.Description, &lb.UserID, &lb.Strategy, &configNodesJSON,
			&lb.Enabled, &lb.AnthropicAPIKey, &lb.CreatedAt, &lb.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan load balancer: %w", err)
		}

		if configNodesJSON != "" {
			err = json.Unmarshal([]byte(configNodesJSON), &lb.ConfigNodes)
			if err != nil {
				lb.ConfigNodes = nil
			}
		}

		loadBalancers = append(loadBalancers, lb)
	}

	return loadBalancers, nil
}

// UpdateLoadBalancer updates an existing load balancer
func UpdateLoadBalancer(lb *LoadBalancer) error {
	// Serialize config_nodes to JSON
	configNodesJSON, err := json.Marshal(lb.ConfigNodes)
	if err != nil {
		return fmt.Errorf("failed to marshal config nodes: %w", err)
	}

	query := `
		UPDATE load_balancers SET
			name = ?, description = ?, strategy = ?, config_nodes = ?,
			enabled = ?, updated_at = datetime('now')
		WHERE id = ?
	`

	_, err = DB.Exec(query,
		lb.Name, lb.Description, lb.Strategy, string(configNodesJSON),
		lb.Enabled, lb.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update load balancer: %w", err)
	}

	return nil
}

// DeleteLoadBalancer deletes a load balancer
func DeleteLoadBalancer(id string) error {
	_, err := DB.Exec("DELETE FROM load_balancers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete load balancer: %w", err)
	}
	return nil
}

// RenewLoadBalancerAPIKey generates a new Anthropic API key for a load balancer
func RenewLoadBalancerAPIKey(id string, customToken string) (string, error) {
	var newAPIKey string

	if customToken != "" {
		// Validate custom token
		if len(customToken) > 100 {
			return "", fmt.Errorf("custom token must be 100 characters or less")
		}

		// Validate characters (alphanumeric + underscore only)
		for _, ch := range customToken {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '_') {
				return "", fmt.Errorf("custom token can only contain letters, numbers, and underscores")
			}
		}

		// Check uniqueness (excluding current load balancer)
		var count int
		checkQuery := `SELECT COUNT(*) FROM load_balancers WHERE anthropic_api_key = ? AND id != ?`
		err := DB.QueryRow(checkQuery, customToken, id).Scan(&count)
		if err != nil {
			return "", fmt.Errorf("failed to check token uniqueness: %w", err)
		}
		if count > 0 {
			return "", fmt.Errorf("this API token is already in use by another load balancer")
		}

		newAPIKey = customToken
	} else {
		// Generate UUID
		newAPIKey = uuid.New().String()
	}

	query := `
		UPDATE load_balancers 
		SET anthropic_api_key = ?, updated_at = datetime('now')
		WHERE id = ?
	`

	_, err := DB.Exec(query, newAPIKey, id)
	if err != nil {
		return "", fmt.Errorf("failed to renew API key: %w", err)
	}

	return newAPIKey, nil
}
