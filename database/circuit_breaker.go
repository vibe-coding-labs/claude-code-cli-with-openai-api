package database

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateOrUpdateCircuitBreakerState creates or updates a circuit breaker state record
func CreateOrUpdateCircuitBreakerState(state *CircuitBreakerState) error {
	query := `
		INSERT INTO circuit_breaker_states (
			config_id, state, failure_count, success_count,
			last_state_change, next_retry_time, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(config_id) DO UPDATE SET
			state = excluded.state,
			failure_count = excluded.failure_count,
			success_count = excluded.success_count,
			last_state_change = excluded.last_state_change,
			next_retry_time = excluded.next_retry_time,
			updated_at = datetime('now')
	`

	_, err := DB.Exec(query,
		state.ConfigID, state.State, state.FailureCount,
		state.SuccessCount, state.LastStateChange, state.NextRetryTime,
	)

	if err != nil {
		return fmt.Errorf("failed to create/update circuit breaker state: %w", err)
	}

	return nil
}

// GetCircuitBreakerState retrieves the circuit breaker state for a configuration node
func GetCircuitBreakerState(configID string) (*CircuitBreakerState, error) {
	query := `
		SELECT config_id, state, failure_count, success_count,
			last_state_change, next_retry_time, created_at, updated_at
		FROM circuit_breaker_states WHERE config_id = ?
	`

	state := &CircuitBreakerState{}
	var nextRetryTime sql.NullTime
	err := DB.QueryRow(query, configID).Scan(
		&state.ConfigID, &state.State, &state.FailureCount,
		&state.SuccessCount, &state.LastStateChange, &nextRetryTime,
		&state.CreatedAt, &state.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("circuit breaker state not found")
		}
		return nil, fmt.Errorf("failed to query circuit breaker state: %w", err)
	}

	if nextRetryTime.Valid {
		state.NextRetryTime = &nextRetryTime.Time
	}

	return state, nil
}

// GetAllCircuitBreakerStates retrieves all circuit breaker states
func GetAllCircuitBreakerStates() ([]*CircuitBreakerState, error) {
	query := `
		SELECT config_id, state, failure_count, success_count,
			last_state_change, next_retry_time, created_at, updated_at
		FROM circuit_breaker_states ORDER BY last_state_change DESC
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query circuit breaker states: %w", err)
	}
	defer rows.Close()

	var states []*CircuitBreakerState
	for rows.Next() {
		state := &CircuitBreakerState{}
		var nextRetryTime sql.NullTime
		err := rows.Scan(
			&state.ConfigID, &state.State, &state.FailureCount,
			&state.SuccessCount, &state.LastStateChange, &nextRetryTime,
			&state.CreatedAt, &state.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan circuit breaker state: %w", err)
		}

		if nextRetryTime.Valid {
			state.NextRetryTime = &nextRetryTime.Time
		}

		states = append(states, state)
	}

	return states, nil
}

// GetCircuitBreakerStatesByLoadBalancer retrieves circuit breaker states for all nodes in a load balancer
func GetCircuitBreakerStatesByLoadBalancer(loadBalancerID string) ([]*CircuitBreakerState, error) {
	// First get the load balancer to get its config nodes
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer: %w", err)
	}

	var states []*CircuitBreakerState
	for _, node := range lb.ConfigNodes {
		state, err := GetCircuitBreakerState(node.ConfigID)
		if err != nil {
			// If circuit breaker state doesn't exist, create a default one
			state = &CircuitBreakerState{
				ConfigID:        node.ConfigID,
				State:           "closed",
				FailureCount:    0,
				SuccessCount:    0,
				LastStateChange: time.Now(),
			}
		}
		states = append(states, state)
	}

	return states, nil
}

// DeleteCircuitBreakerState deletes a circuit breaker state record
func DeleteCircuitBreakerState(configID string) error {
	_, err := DB.Exec("DELETE FROM circuit_breaker_states WHERE config_id = ?", configID)
	if err != nil {
		return fmt.Errorf("failed to delete circuit breaker state: %w", err)
	}
	return nil
}

// InitializeCircuitBreakerState initializes a circuit breaker state for a config node
func InitializeCircuitBreakerState(configID string) error {
	state := &CircuitBreakerState{
		ConfigID:        configID,
		State:           "closed",
		FailureCount:    0,
		SuccessCount:    0,
		LastStateChange: time.Now(),
	}
	return CreateOrUpdateCircuitBreakerState(state)
}

// TransitionCircuitBreakerToOpen transitions a circuit breaker to open state
func TransitionCircuitBreakerToOpen(configID string, timeout int) error {
	nextRetryTime := time.Now().Add(time.Duration(timeout) * time.Second)
	state := &CircuitBreakerState{
		ConfigID:        configID,
		State:           "open",
		FailureCount:    0,
		SuccessCount:    0,
		LastStateChange: time.Now(),
		NextRetryTime:   &nextRetryTime,
	}
	return CreateOrUpdateCircuitBreakerState(state)
}

// TransitionCircuitBreakerToHalfOpen transitions a circuit breaker to half-open state
func TransitionCircuitBreakerToHalfOpen(configID string) error {
	state := &CircuitBreakerState{
		ConfigID:        configID,
		State:           "half_open",
		FailureCount:    0,
		SuccessCount:    0,
		LastStateChange: time.Now(),
	}
	return CreateOrUpdateCircuitBreakerState(state)
}

// TransitionCircuitBreakerToClosed transitions a circuit breaker to closed state
func TransitionCircuitBreakerToClosed(configID string) error {
	state := &CircuitBreakerState{
		ConfigID:        configID,
		State:           "closed",
		FailureCount:    0,
		SuccessCount:    0,
		LastStateChange: time.Now(),
	}
	return CreateOrUpdateCircuitBreakerState(state)
}

// RecordCircuitBreakerSuccess records a successful request for circuit breaker
func RecordCircuitBreakerSuccess(configID string) error {
	state, err := GetCircuitBreakerState(configID)
	if err != nil {
		// Initialize if doesn't exist
		return InitializeCircuitBreakerState(configID)
	}

	state.SuccessCount++
	state.FailureCount = 0
	return CreateOrUpdateCircuitBreakerState(state)
}

// RecordCircuitBreakerFailure records a failed request for circuit breaker
func RecordCircuitBreakerFailure(configID string) error {
	state, err := GetCircuitBreakerState(configID)
	if err != nil {
		// Initialize if doesn't exist
		state = &CircuitBreakerState{
			ConfigID:        configID,
			State:           "closed",
			FailureCount:    1,
			SuccessCount:    0,
			LastStateChange: time.Now(),
		}
		return CreateOrUpdateCircuitBreakerState(state)
	}

	state.FailureCount++
	state.SuccessCount = 0
	return CreateOrUpdateCircuitBreakerState(state)
}
