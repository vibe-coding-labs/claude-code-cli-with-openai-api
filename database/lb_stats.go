package database

import (
	"fmt"
	"time"
)

// CreateLoadBalancerStats creates a new load balancer stats record
func CreateLoadBalancerStats(stats *LoadBalancerStats) error {
	// For now, we'll store basic stats in the load_balancer_stats table
	// The full LoadBalancerStats struct will be computed on-the-fly from request logs
	query := `
		INSERT INTO load_balancer_stats (
			load_balancer_id, time_bucket, total_requests, success_requests,
			failed_requests, total_duration_ms, min_duration_ms, max_duration_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	totalDuration := int64(float64(stats.TotalRequests) * stats.AvgResponseTimeMs)
	_, err := DB.Exec(query,
		stats.LoadBalancerID, time.Now().Truncate(time.Minute),
		stats.TotalRequests, stats.SuccessRequests, stats.FailedRequests,
		totalDuration, 0, 0,
	)

	if err != nil {
		return fmt.Errorf("failed to create load balancer stats: %w", err)
	}

	return nil
}

// GetLoadBalancerStats retrieves aggregated statistics for a load balancer
func GetLoadBalancerStats(loadBalancerID string, timeWindow string) (*LoadBalancerStats, error) {
	var hours int
	switch timeWindow {
	case "1h":
		hours = 1
	case "24h":
		hours = 24
	case "7d":
		hours = 24 * 7
	case "30d":
		hours = 24 * 30
	default:
		hours = 24 // default to 24 hours
	}

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Query aggregated stats from request logs
	query := `
		SELECT
			COUNT(*) as total_requests,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_requests,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_requests,
			COALESCE(AVG(duration_ms), 0) as avg_response_time_ms
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND request_time >= ?
	`

	stats := &LoadBalancerStats{
		LoadBalancerID: loadBalancerID,
		TimeWindow:     timeWindow,
	}

	err := DB.QueryRow(query, loadBalancerID, startTime).Scan(
		&stats.TotalRequests, &stats.SuccessRequests,
		&stats.FailedRequests, &stats.AvgResponseTimeMs,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer stats: %w", err)
	}

	// Calculate error rate
	if stats.TotalRequests > 0 {
		stats.ErrorRate = float64(stats.FailedRequests) / float64(stats.TotalRequests)
	}

	// Get percentile response times
	percentiles, err := getResponseTimePercentiles(loadBalancerID, startTime)
	if err == nil {
		stats.P50ResponseTimeMs = percentiles.P50
		stats.P95ResponseTimeMs = percentiles.P95
		stats.P99ResponseTimeMs = percentiles.P99
	}

	// Get node stats
	nodeStats, err := GetNodeStatsByLoadBalancer(loadBalancerID, startTime)
	if err == nil {
		stats.NodeStats = nodeStats
	}

	return stats, nil
}

type percentiles struct {
	P50 int
	P95 int
	P99 int
}

// getResponseTimePercentiles calculates response time percentiles
func getResponseTimePercentiles(loadBalancerID string, startTime time.Time) (*percentiles, error) {
	query := `
		SELECT duration_ms
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND request_time >= ?
		ORDER BY duration_ms
	`

	rows, err := DB.Query(query, loadBalancerID, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query response times: %w", err)
	}
	defer rows.Close()

	var durations []int
	for rows.Next() {
		var duration int
		if err := rows.Scan(&duration); err != nil {
			continue
		}
		durations = append(durations, duration)
	}

	if len(durations) == 0 {
		return &percentiles{}, nil
	}

	p := &percentiles{
		P50: durations[len(durations)*50/100],
		P95: durations[len(durations)*95/100],
		P99: durations[len(durations)*99/100],
	}

	return p, nil
}

// GetNodeStatsByLoadBalancer retrieves node statistics for a load balancer
func GetNodeStatsByLoadBalancer(loadBalancerID string, startTime time.Time) ([]NodeStats, error) {
	// Get the load balancer to get its config nodes
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer: %w", err)
	}

	var nodeStats []NodeStats
	for _, node := range lb.ConfigNodes {
		// Get config info
		config, err := GetAPIConfig(node.ConfigID)
		if err != nil {
			continue
		}

		// Get health status
		healthStatus, err := GetHealthStatus(node.ConfigID)
		healthStatusStr := "unknown"
		if err == nil {
			healthStatusStr = healthStatus.Status
		}

		// Get circuit breaker state
		cbState, err := GetCircuitBreakerState(node.ConfigID)
		cbStateStr := "closed"
		if err == nil {
			cbStateStr = cbState.State
		}

		// Get request stats for this node
		query := `
			SELECT
				COUNT(*) as request_count,
				SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_count,
				COALESCE(AVG(duration_ms), 0) as avg_response_time_ms
			FROM load_balancer_request_logs
			WHERE load_balancer_id = ? AND selected_config_id = ? AND request_time >= ?
		`

		var requestCount, successCount int64
		var avgResponseTime float64
		err = DB.QueryRow(query, loadBalancerID, node.ConfigID, startTime).Scan(
			&requestCount, &successCount, &avgResponseTime,
		)

		successRate := 0.0
		if requestCount > 0 {
			successRate = float64(successCount) / float64(requestCount)
		}

		nodeStats = append(nodeStats, NodeStats{
			ConfigID:            node.ConfigID,
			ConfigName:          config.Name,
			HealthStatus:        healthStatusStr,
			CircuitBreakerState: cbStateStr,
			RequestCount:        requestCount,
			SuccessRate:         successRate,
			AvgResponseTimeMs:   avgResponseTime,
			CurrentWeight:       node.Weight,
			BaseWeight:          node.Weight,
		})
	}

	return nodeStats, nil
}

// CreateNodeStats creates a new node stats record
func CreateNodeStats(loadBalancerID, configID string, timeBucket time.Time, requestCount, successCount, failedCount int, totalDurationMs int64) error {
	query := `
		INSERT INTO node_stats (
			load_balancer_id, config_id, time_bucket, request_count,
			success_count, failed_count, total_duration_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := DB.Exec(query,
		loadBalancerID, configID, timeBucket, requestCount,
		successCount, failedCount, totalDurationMs,
	)

	if err != nil {
		return fmt.Errorf("failed to create node stats: %w", err)
	}

	return nil
}

// AggregateStatsForTimeBucket aggregates request logs into stats for a time bucket
func AggregateStatsForTimeBucket(loadBalancerID string, timeBucket time.Time) error {
	// Get all config nodes for this load balancer
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return fmt.Errorf("failed to get load balancer: %w", err)
	}

	// Aggregate stats for each node
	for _, node := range lb.ConfigNodes {
		query := `
			SELECT
				COUNT(*) as request_count,
				SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_count,
				SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_count,
				COALESCE(SUM(duration_ms), 0) as total_duration_ms
			FROM load_balancer_request_logs
			WHERE load_balancer_id = ? AND selected_config_id = ?
				AND request_time >= ? AND request_time < ?
		`

		nextBucket := timeBucket.Add(time.Minute)
		var requestCount, successCount, failedCount int
		var totalDurationMs int64

		err := DB.QueryRow(query, loadBalancerID, node.ConfigID, timeBucket, nextBucket).Scan(
			&requestCount, &successCount, &failedCount, &totalDurationMs,
		)

		if err != nil {
			continue
		}

		if requestCount > 0 {
			err = CreateNodeStats(loadBalancerID, node.ConfigID, timeBucket, requestCount, successCount, failedCount, totalDurationMs)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// DeleteOldStats deletes stats older than the specified days
func DeleteOldStats(days int) error {
	queries := []string{
		`DELETE FROM load_balancer_stats WHERE created_at < datetime('now', '-' || ? || ' days')`,
		`DELETE FROM node_stats WHERE created_at < datetime('now', '-' || ? || ' days')`,
	}

	for _, query := range queries {
		_, err := DB.Exec(query, days)
		if err != nil {
			return fmt.Errorf("failed to delete old stats: %w", err)
		}
	}

	return nil
}

// GetNodeStatsForTimeWindow retrieves statistics for a specific node in a time window
func GetNodeStatsForTimeWindow(loadBalancerID, configID, timeWindow string) (*NodeStats, error) {
	var hours int
	switch timeWindow {
	case "1h":
		hours = 1
	case "24h":
		hours = 24
	case "7d":
		hours = 24 * 7
	case "30d":
		hours = 24 * 30
	default:
		hours = 24 // default to 24 hours
	}

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Get config info
	config, err := GetAPIConfig(configID)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// Get health status
	healthStatus, err := GetHealthStatus(configID)
	healthStatusStr := "unknown"
	if err == nil {
		healthStatusStr = healthStatus.Status
	}

	// Get circuit breaker state
	cbState, err := GetCircuitBreakerState(configID)
	cbStateStr := "closed"
	if err == nil {
		cbStateStr = cbState.State
	}

	// Get request stats for this node
	query := `
		SELECT
			COUNT(*) as request_count,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_count,
			COALESCE(AVG(duration_ms), 0) as avg_response_time_ms
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND selected_config_id = ? AND request_time >= ?
	`

	var requestCount, successCount int64
	var avgResponseTime float64
	err = DB.QueryRow(query, loadBalancerID, configID, startTime).Scan(
		&requestCount, &successCount, &avgResponseTime,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get node stats: %w", err)
	}

	successRate := 0.0
	if requestCount > 0 {
		successRate = float64(successCount) / float64(requestCount)
	}

	// Get base weight from load balancer config
	lb, err := GetLoadBalancer(loadBalancerID)
	baseWeight := 10 // default
	if err == nil {
		for _, node := range lb.ConfigNodes {
			if node.ConfigID == configID {
				baseWeight = node.Weight
				break
			}
		}
	}

	return &NodeStats{
		ConfigID:            configID,
		ConfigName:          config.Name,
		HealthStatus:        healthStatusStr,
		CircuitBreakerState: cbStateStr,
		RequestCount:        requestCount,
		SuccessRate:         successRate,
		AvgResponseTimeMs:   avgResponseTime,
		CurrentWeight:       baseWeight,
		BaseWeight:          baseWeight,
	}, nil
}

// GetRealTimeMetrics retrieves real-time metrics for a load balancer
func GetRealTimeMetrics(loadBalancerID string) (*RealTimeMetrics, error) {
	// Get load balancer info
	lb, err := GetLoadBalancer(loadBalancerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer: %w", err)
	}

	// Calculate metrics for the last 60 seconds
	startTime := time.Now().Add(-60 * time.Second)
	
	// Get request stats for the last 60 seconds
	query := `
		SELECT
			COUNT(*) as total_requests,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_requests,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_requests,
			COALESCE(AVG(duration_ms), 0) as avg_response_time_ms
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND request_time >= ?
	`

	var totalRequests, successRequests, failedRequests int64
	var avgResponseTime float64
	
	err = DB.QueryRow(query, loadBalancerID, startTime).Scan(
		&totalRequests, &successRequests, &failedRequests, &avgResponseTime,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query request stats: %w", err)
	}

	// Calculate success rate
	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(successRequests) / float64(totalRequests)
	}

	// Calculate requests per second
	requestsPerSecond := float64(totalRequests) / 60.0

	// Count healthy nodes
	healthyNodes := 0
	totalNodes := len(lb.ConfigNodes)
	
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}
		
		healthStatus, err := GetHealthStatus(node.ConfigID)
		if err == nil && healthStatus.Status == "healthy" {
			healthyNodes++
		}
	}

	// Get active connections (requests in the last 5 seconds)
	activeConnectionsQuery := `
		SELECT COUNT(*)
		FROM load_balancer_request_logs
		WHERE load_balancer_id = ? AND request_time >= ?
	`
	
	var activeConnections int
	err = DB.QueryRow(activeConnectionsQuery, loadBalancerID, time.Now().Add(-5*time.Second)).Scan(&activeConnections)
	if err != nil {
		activeConnections = 0
	}

	// Get node metrics
	nodeMetrics := make([]NodeRealTimeMetrics, 0, len(lb.ConfigNodes))
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}

		// Get config info
		config, err := GetAPIConfig(node.ConfigID)
		if err != nil {
			continue
		}

		// Get health status
		healthStatus, err := GetHealthStatus(node.ConfigID)
		healthStatusStr := "unknown"
		if err == nil {
			healthStatusStr = healthStatus.Status
		}

		// Get circuit breaker state
		cbState, err := GetCircuitBreakerState(node.ConfigID)
		cbStateStr := "closed"
		if err == nil {
			cbStateStr = cbState.State
		}

		// Get node request stats for the last 60 seconds
		nodeQuery := `
			SELECT
				COUNT(*) as request_count,
				SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_count,
				COALESCE(AVG(duration_ms), 0) as avg_response_time_ms,
				MAX(request_time) as last_request_time
			FROM load_balancer_request_logs
			WHERE load_balancer_id = ? AND selected_config_id = ? AND request_time >= ?
		`

		var requestCount, successCount int64
		var nodeAvgResponseTime float64
		var lastRequestTime *time.Time
		
		err = DB.QueryRow(nodeQuery, loadBalancerID, node.ConfigID, startTime).Scan(
			&requestCount, &successCount, &nodeAvgResponseTime, &lastRequestTime,
		)
		if err != nil {
			// Node has no requests in the last 60 seconds
			requestCount = 0
			successCount = 0
			nodeAvgResponseTime = 0
		}

		nodeSuccessRate := 0.0
		if requestCount > 0 {
			nodeSuccessRate = float64(successCount) / float64(requestCount)
		}

		nodeRequestsPerSecond := float64(requestCount) / 60.0

		nodeMetrics = append(nodeMetrics, NodeRealTimeMetrics{
			ConfigID:            node.ConfigID,
			ConfigName:          config.Name,
			HealthStatus:        healthStatusStr,
			CircuitBreakerState: cbStateStr,
			RequestsPerSecond:   nodeRequestsPerSecond,
			SuccessRate:         nodeSuccessRate,
			AvgResponseTimeMs:   nodeAvgResponseTime,
			LastRequestTime:     lastRequestTime,
		})
	}

	return &RealTimeMetrics{
		LoadBalancerID:    loadBalancerID,
		Timestamp:         time.Now(),
		RequestsPerSecond: requestsPerSecond,
		SuccessRate:       successRate,
		AvgResponseTimeMs: avgResponseTime,
		ActiveConnections: activeConnections,
		HealthyNodes:      healthyNodes,
		TotalNodes:        totalNodes,
		TotalRequests:     totalRequests,
		SuccessRequests:   successRequests,
		FailedRequests:    failedRequests,
		NodeMetrics:       nodeMetrics,
	}, nil
}
