package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestMonitorRealTimeMetrics(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Use unique IDs to avoid conflicts
	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-metrics-%d", timestamp)
	config1ID := fmt.Sprintf("config1-%d", timestamp)
	config2ID := fmt.Sprintf("config2-%d", timestamp)

	// Create a test load balancer
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB Metrics",
		Description: "Test load balancer for metrics",
		Strategy:    "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: config1ID, Weight: 10, Enabled: true},
			{ConfigID: config2ID, Weight: 10, Enabled: true},
		},
		Enabled: true,
	}

	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Create test configs
	config1 := &database.APIConfig{
		ID:                    config1ID,
		Name:                  "Config 1",
		OpenAIBaseURL:         "http://localhost:8080",
		OpenAIAPIKeyEncrypted: "encrypted-key-1", // Use pre-encrypted key
		Enabled:               true,
	}
	config2 := &database.APIConfig{
		ID:                    config2ID,
		Name:                  "Config 2",
		OpenAIBaseURL:         "http://localhost:8081",
		OpenAIAPIKeyEncrypted: "encrypted-key-2", // Use pre-encrypted key
		Enabled:               true,
	}

	// Insert directly to bypass encryption
	_, err := database.DB.Exec(`
		INSERT INTO api_configs (id, name, openai_base_url, openai_api_key_encrypted, big_model, middle_model, small_model, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, config1.ID, config1.Name, config1.OpenAIBaseURL, config1.OpenAIAPIKeyEncrypted, "claude-3-opus", "claude-3-sonnet", "claude-3-haiku", config1.Enabled)
	if err != nil {
		t.Fatalf("Failed to create config1: %v", err)
	}

	_, err = database.DB.Exec(`
		INSERT INTO api_configs (id, name, openai_base_url, openai_api_key_encrypted, big_model, middle_model, small_model, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, config2.ID, config2.Name, config2.OpenAIBaseURL, config2.OpenAIAPIKeyEncrypted, "claude-3-opus", "claude-3-sonnet", "claude-3-haiku", config2.Enabled)
	if err != nil {
		t.Fatalf("Failed to create config2: %v", err)
	}

	// Create health statuses
	health1 := &database.HealthStatus{
		ConfigID:      config1ID,
		Status:        "healthy",
		LastCheckTime: time.Now(),
	}
	health2 := &database.HealthStatus{
		ConfigID:      config2ID,
		Status:        "healthy",
		LastCheckTime: time.Now(),
	}

	if err := database.CreateOrUpdateHealthStatus(health1); err != nil {
		t.Fatalf("Failed to create health status 1: %v", err)
	}
	if err := database.CreateOrUpdateHealthStatus(health2); err != nil {
		t.Fatalf("Failed to create health status 2: %v", err)
	}

	// Create some test request logs
	now := time.Now()
	logs := []*database.LoadBalancerRequestLog{
		{
			ID:               fmt.Sprintf("log1-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: config1ID,
			RequestTime:      now.Add(-30 * time.Second),
			ResponseTime:     now.Add(-29 * time.Second),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
		},
		{
			ID:               fmt.Sprintf("log2-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: config2ID,
			RequestTime:      now.Add(-25 * time.Second),
			ResponseTime:     now.Add(-24 * time.Second),
			DurationMs:       150,
			StatusCode:       200,
			Success:          true,
		},
		{
			ID:               fmt.Sprintf("log3-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: config1ID,
			RequestTime:      now.Add(-20 * time.Second),
			ResponseTime:     now.Add(-19 * time.Second),
			DurationMs:       200,
			StatusCode:       500,
			Success:          false,
		},
		{
			ID:               fmt.Sprintf("log4-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: config2ID,
			RequestTime:      now.Add(-10 * time.Second),
			ResponseTime:     now.Add(-9 * time.Second),
			DurationMs:       120,
			StatusCode:       200,
			Success:          true,
		},
	}

	for _, log := range logs {
		if err := database.CreateLoadBalancerRequestLog(log); err != nil {
			t.Fatalf("Failed to create request log: %v", err)
		}
	}

	// Create monitor and get real-time metrics
	monitor := NewMonitor(lbID)

	metrics, err := monitor.GetRealTimeMetrics(lbID)
	if err != nil {
		t.Fatalf("Failed to get real-time metrics: %v", err)
	}

	// Verify metrics
	if metrics.LoadBalancerID != lbID {
		t.Errorf("Expected load_balancer_id to be '%s', got '%s'", lbID, metrics.LoadBalancerID)
	}

	if metrics.TotalRequests != 4 {
		t.Errorf("Expected total_requests to be 4, got %d", metrics.TotalRequests)
	}

	if metrics.SuccessRequests != 3 {
		t.Errorf("Expected success_requests to be 3, got %d", metrics.SuccessRequests)
	}

	if metrics.FailedRequests != 1 {
		t.Errorf("Expected failed_requests to be 1, got %d", metrics.FailedRequests)
	}

	expectedSuccessRate := 3.0 / 4.0
	if metrics.SuccessRate != expectedSuccessRate {
		t.Errorf("Expected success_rate to be %.2f, got %.2f", expectedSuccessRate, metrics.SuccessRate)
	}

	// Requests per second should be total_requests / 60
	expectedRPS := 4.0 / 60.0
	if metrics.RequestsPerSecond < expectedRPS-0.01 || metrics.RequestsPerSecond > expectedRPS+0.01 {
		t.Errorf("Expected requests_per_second to be around %.4f, got %.4f", expectedRPS, metrics.RequestsPerSecond)
	}

	if metrics.HealthyNodes != 2 {
		t.Errorf("Expected healthy_nodes to be 2, got %d", metrics.HealthyNodes)
	}

	if metrics.TotalNodes != 2 {
		t.Errorf("Expected total_nodes to be 2, got %d", metrics.TotalNodes)
	}

	// Verify node metrics
	if len(metrics.NodeMetrics) != 2 {
		t.Logf("Node metrics count: %d", len(metrics.NodeMetrics))
		for i, nm := range metrics.NodeMetrics {
			t.Logf("Node %d: ConfigID=%s, Name=%s", i, nm.ConfigID, nm.ConfigName)
		}
		t.Errorf("Expected 2 node metrics, got %d", len(metrics.NodeMetrics))
	}

	// Find config1 metrics
	var config1Metrics *database.NodeRealTimeMetrics
	for i := range metrics.NodeMetrics {
		if metrics.NodeMetrics[i].ConfigID == config1ID {
			config1Metrics = &metrics.NodeMetrics[i]
			break
		}
	}

	if config1Metrics == nil {
		t.Fatal("Config1 metrics not found")
	}

	if config1Metrics.HealthStatus != "healthy" {
		t.Errorf("Expected config1 health_status to be 'healthy', got '%s'", config1Metrics.HealthStatus)
	}

	// Config1 had 2 requests (1 success, 1 failure)
	expectedConfig1SuccessRate := 1.0 / 2.0
	if config1Metrics.SuccessRate != expectedConfig1SuccessRate {
		t.Errorf("Expected config1 success_rate to be %.2f, got %.2f", expectedConfig1SuccessRate, config1Metrics.SuccessRate)
	}
}

func TestMonitorStartStop(t *testing.T) {
	monitor := NewMonitor("test-lb-start-stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start monitor
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// Try to start again (should fail)
	if err := monitor.Start(ctx); err == nil {
		t.Error("Expected error when starting already running monitor")
	}

	// Stop monitor
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	// Try to stop again (should fail)
	if err := monitor.Stop(); err == nil {
		t.Error("Expected error when stopping already stopped monitor")
	}
}

func TestMonitorRecordRequest(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	monitor := NewMonitor("test-lb-record")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start monitor
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Record a request
	log := &database.LoadBalancerRequestLog{
		ID:               "test-log-1",
		LoadBalancerID:   "test-lb-record",
		SelectedConfigID: "config1",
		RequestTime:      time.Now(),
		ResponseTime:     time.Now(),
		DurationMs:       100,
		StatusCode:       200,
		Success:          true,
	}

	monitor.RecordRequest(log)

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)

	// Verify the log was recorded
	logs, err := database.GetLoadBalancerRequestLogs("test-lb-record", 10, 0)
	if err != nil {
		t.Fatalf("Failed to get request logs: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}
}

func TestMonitorCleanup(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()

	// Create old request logs (35 days old)
	oldTime := time.Now().Add(-35 * 24 * time.Hour)
	oldLog := &database.LoadBalancerRequestLog{
		ID:               fmt.Sprintf("old-log-%d", timestamp),
		LoadBalancerID:   "test-lb-cleanup",
		SelectedConfigID: "config1",
		RequestTime:      oldTime,
		ResponseTime:     oldTime,
		DurationMs:       100,
		StatusCode:       200,
		Success:          true,
	}

	if err := database.CreateLoadBalancerRequestLog(oldLog); err != nil {
		t.Fatalf("Failed to create old log: %v", err)
	}

	// Create recent request log (1 day old)
	recentTime := time.Now().Add(-24 * time.Hour)
	recentLog := &database.LoadBalancerRequestLog{
		ID:               fmt.Sprintf("recent-log-%d", timestamp),
		LoadBalancerID:   "test-lb-cleanup",
		SelectedConfigID: "config1",
		RequestTime:      recentTime,
		ResponseTime:     recentTime,
		DurationMs:       100,
		StatusCode:       200,
		Success:          true,
	}

	if err := database.CreateLoadBalancerRequestLog(recentLog); err != nil {
		t.Fatalf("Failed to create recent log: %v", err)
	}

	// Verify both logs exist
	logs, err := database.GetLoadBalancerRequestLogs("test-lb-cleanup", 100, 0)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("Expected 2 logs before cleanup, got %d", len(logs))
	}

	// Run cleanup (deletes logs older than 30 days)
	if err := CleanupOldData(); err != nil {
		t.Fatalf("Failed to cleanup old data: %v", err)
	}

	// Verify only recent log remains
	logs, err = database.GetLoadBalancerRequestLogs("test-lb-cleanup", 100, 0)
	if err != nil {
		t.Fatalf("Failed to get logs after cleanup: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("Expected 1 log after cleanup, got %d", len(logs))
	}

	if len(logs) > 0 && logs[0].ID != fmt.Sprintf("recent-log-%d", timestamp) {
		t.Errorf("Expected recent log to remain, got log with ID %s", logs[0].ID)
	}
}

func TestMonitorGetStats(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-stats-%d", timestamp)
	configID := fmt.Sprintf("config-%d", timestamp)

	// Create a test load balancer
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB Stats",
		Description: "Test load balancer for stats",
		Strategy:    "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: configID, Weight: 10, Enabled: true},
		},
		Enabled: true,
	}

	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Create test config
	_, err := database.DB.Exec(`
		INSERT INTO api_configs (id, name, openai_base_url, openai_api_key_encrypted, big_model, middle_model, small_model, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, configID, "Test Config", "http://localhost:8080", "encrypted-key", "claude-3-opus", "claude-3-sonnet", "claude-3-haiku", true)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create test request logs
	now := time.Now()
	logs := []*database.LoadBalancerRequestLog{
		{
			ID:               fmt.Sprintf("log1-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-30 * time.Minute),
			ResponseTime:     now.Add(-30*time.Minute + 100*time.Millisecond),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
		},
		{
			ID:               fmt.Sprintf("log2-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-20 * time.Minute),
			ResponseTime:     now.Add(-20*time.Minute + 150*time.Millisecond),
			DurationMs:       150,
			StatusCode:       200,
			Success:          true,
		},
		{
			ID:               fmt.Sprintf("log3-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-10 * time.Minute),
			ResponseTime:     now.Add(-10*time.Minute + 200*time.Millisecond),
			DurationMs:       200,
			StatusCode:       500,
			Success:          false,
		},
	}

	for _, log := range logs {
		if err := database.CreateLoadBalancerRequestLog(log); err != nil {
			t.Fatalf("Failed to create request log: %v", err)
		}
	}

	// Create monitor and get stats
	monitor := NewMonitor(lbID)

	// Test 1h window
	stats, err := monitor.GetStats(lbID, "1h")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.LoadBalancerID != lbID {
		t.Errorf("Expected load_balancer_id to be '%s', got '%s'", lbID, stats.LoadBalancerID)
	}

	if stats.TimeWindow != "1h" {
		t.Errorf("Expected time_window to be '1h', got '%s'", stats.TimeWindow)
	}

	if stats.TotalRequests != 3 {
		t.Errorf("Expected total_requests to be 3, got %d", stats.TotalRequests)
	}

	if stats.SuccessRequests != 2 {
		t.Errorf("Expected success_requests to be 2, got %d", stats.SuccessRequests)
	}

	if stats.FailedRequests != 1 {
		t.Errorf("Expected failed_requests to be 1, got %d", stats.FailedRequests)
	}

	expectedAvgResponseTime := (100.0 + 150.0 + 200.0) / 3.0
	if stats.AvgResponseTimeMs < expectedAvgResponseTime-1 || stats.AvgResponseTimeMs > expectedAvgResponseTime+1 {
		t.Errorf("Expected avg_response_time_ms to be around %.2f, got %.2f", expectedAvgResponseTime, stats.AvgResponseTimeMs)
	}

	expectedErrorRate := 1.0 / 3.0
	if stats.ErrorRate < expectedErrorRate-0.01 || stats.ErrorRate > expectedErrorRate+0.01 {
		t.Errorf("Expected error_rate to be around %.2f, got %.2f", expectedErrorRate, stats.ErrorRate)
	}
}

func TestMonitorAggregation(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-agg-%d", timestamp)
	configID := fmt.Sprintf("config-%d", timestamp)

	// Create a test load balancer
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB Aggregation",
		Description: "Test load balancer for aggregation",
		Strategy:    "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: configID, Weight: 10, Enabled: true},
		},
		Enabled: true,
	}

	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Create test config
	_, err := database.DB.Exec(`
		INSERT INTO api_configs (id, name, openai_base_url, openai_api_key_encrypted, big_model, middle_model, small_model, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, configID, "Test Config", "http://localhost:8080", "encrypted-key", "claude-3-opus", "claude-3-sonnet", "claude-3-haiku", true)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create test request logs for a specific time bucket
	timeBucket := time.Now().Truncate(time.Minute)
	logs := []*database.LoadBalancerRequestLog{
		{
			ID:               fmt.Sprintf("log1-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      timeBucket.Add(10 * time.Second),
			ResponseTime:     timeBucket.Add(10*time.Second + 100*time.Millisecond),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
		},
		{
			ID:               fmt.Sprintf("log2-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      timeBucket.Add(20 * time.Second),
			ResponseTime:     timeBucket.Add(20*time.Second + 150*time.Millisecond),
			DurationMs:       150,
			StatusCode:       500,
			Success:          false,
		},
	}

	for _, log := range logs {
		if err := database.CreateLoadBalancerRequestLog(log); err != nil {
			t.Fatalf("Failed to create request log: %v", err)
		}
	}

	// Run aggregation
	if err := database.AggregateStatsForTimeBucket(lbID, timeBucket); err != nil {
		t.Fatalf("Failed to aggregate stats: %v", err)
	}

	// Verify node stats were created
	var requestCount, successCount, failedCount int
	var totalDurationMs int64

	query := `
		SELECT request_count, success_count, failed_count, total_duration_ms
		FROM node_stats
		WHERE load_balancer_id = ? AND config_id = ? AND time_bucket = ?
	`

	err = database.DB.QueryRow(query, lbID, configID, timeBucket).Scan(
		&requestCount, &successCount, &failedCount, &totalDurationMs,
	)

	if err != nil {
		t.Fatalf("Failed to query node stats: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("Expected request_count to be 2, got %d", requestCount)
	}

	if successCount != 1 {
		t.Errorf("Expected success_count to be 1, got %d", successCount)
	}

	if failedCount != 1 {
		t.Errorf("Expected failed_count to be 1, got %d", failedCount)
	}

	expectedTotalDuration := int64(100 + 150)
	if totalDurationMs != expectedTotalDuration {
		t.Errorf("Expected total_duration_ms to be %d, got %d", expectedTotalDuration, totalDurationMs)
	}
}
