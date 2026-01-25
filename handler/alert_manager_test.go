package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestAlertManagerStartStop(t *testing.T) {
	// Setup test database for the background goroutine
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create a test load balancer so the background goroutine doesn't fail
	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-start-stop-%d", timestamp)
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB Start Stop",
		Description: "Test load balancer for start/stop",
		Strategy:    "round_robin",
		ConfigNodes: []database.ConfigNode{},
		Enabled:     true,
	}

	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	config := AlertManagerConfig{
		CheckInterval:      1 * time.Second,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    1,
	}

	am := NewAlertManager(lbID, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start alert manager
	if err := am.Start(ctx); err != nil {
		t.Fatalf("Failed to start alert manager: %v", err)
	}

	// Try to start again (should fail)
	if err := am.Start(ctx); err == nil {
		t.Error("Expected error when starting already running alert manager")
	}

	// Stop alert manager
	if err := am.Stop(); err != nil {
		t.Fatalf("Failed to stop alert manager: %v", err)
	}

	// Try to stop again (should fail)
	if err := am.Stop(); err == nil {
		t.Error("Expected error when stopping already stopped alert manager")
	}
}

func TestAlertManagerAllNodesDown(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-all-down-%d", timestamp)
	config1ID := fmt.Sprintf("config1-%d", timestamp)
	config2ID := fmt.Sprintf("config2-%d", timestamp)

	// Create a test load balancer
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB All Down",
		Description: "Test load balancer for all nodes down alert",
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

	// Create unhealthy status for both nodes
	health1 := &database.HealthStatus{
		ConfigID:      config1ID,
		Status:        "unhealthy",
		LastCheckTime: time.Now(),
	}
	health2 := &database.HealthStatus{
		ConfigID:      config2ID,
		Status:        "unhealthy",
		LastCheckTime: time.Now(),
	}

	if err := database.CreateOrUpdateHealthStatus(health1); err != nil {
		t.Fatalf("Failed to create health status 1: %v", err)
	}
	if err := database.CreateOrUpdateHealthStatus(health2); err != nil {
		t.Fatalf("Failed to create health status 2: %v", err)
	}

	// Create alert manager
	config := AlertManagerConfig{
		CheckInterval:      1 * time.Second,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    1,
	}

	am := NewAlertManager(lbID, config)

	// Check and create alerts (don't start the background goroutine)
	if err := am.CheckAndCreateAlerts(lbID); err != nil {
		t.Fatalf("Failed to check and create alerts: %v", err)
	}

	// Get alerts
	alerts, err := am.GetAlerts(lbID, false)
	if err != nil {
		t.Fatalf("Failed to get alerts: %v", err)
	}

	// Should have at least one critical alert for all nodes down
	foundCritical := false
	for _, alert := range alerts {
		if alert.Level == "critical" && alert.Type == "all_nodes_down" {
			foundCritical = true
			break
		}
	}

	if !foundCritical {
		t.Error("Expected critical alert for all nodes down")
	}
}

func TestAlertManagerLowHealthyNodes(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-low-healthy-%d", timestamp)
	config1ID := fmt.Sprintf("config1-%d", timestamp)
	config2ID := fmt.Sprintf("config2-%d", timestamp)

	// Create a test load balancer
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB Low Healthy",
		Description: "Test load balancer for low healthy nodes alert",
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

	// Create one healthy and one unhealthy node
	health1 := &database.HealthStatus{
		ConfigID:      config1ID,
		Status:        "healthy",
		LastCheckTime: time.Now(),
	}
	health2 := &database.HealthStatus{
		ConfigID:      config2ID,
		Status:        "unhealthy",
		LastCheckTime: time.Now(),
	}

	if err := database.CreateOrUpdateHealthStatus(health1); err != nil {
		t.Fatalf("Failed to create health status 1: %v", err)
	}
	if err := database.CreateOrUpdateHealthStatus(health2); err != nil {
		t.Fatalf("Failed to create health status 2: %v", err)
	}

	// Create alert manager with minHealthyNodes = 2
	config := AlertManagerConfig{
		CheckInterval:      1 * time.Second,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    2,
	}

	am := NewAlertManager(lbID, config)

	// Check and create alerts
	if err := am.CheckAndCreateAlerts(lbID); err != nil {
		t.Fatalf("Failed to check and create alerts: %v", err)
	}

	// Get alerts
	alerts, err := am.GetAlerts(lbID, false)
	if err != nil {
		t.Fatalf("Failed to get alerts: %v", err)
	}

	// Should have a warning alert for low healthy nodes
	foundWarning := false
	for _, alert := range alerts {
		if alert.Level == "warning" && alert.Type == "low_healthy_nodes" {
			foundWarning = true
			break
		}
	}

	if !foundWarning {
		t.Error("Expected warning alert for low healthy nodes")
	}
}

func TestAlertManagerHighErrorRate(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-high-error-%d", timestamp)
	configID := fmt.Sprintf("config-%d", timestamp)

	// Create a test load balancer
	lb := &database.LoadBalancer{
		ID:          lbID,
		Name:        "Test LB High Error",
		Description: "Test load balancer for high error rate alert",
		Strategy:    "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: configID, Weight: 10, Enabled: true},
		},
		Enabled: true,
	}

	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Create request logs with high error rate (4 failures out of 5 requests = 80%)
	now := time.Now()
	logs := []*database.LoadBalancerRequestLog{
		{
			ID:               fmt.Sprintf("log1-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-4 * time.Minute),
			ResponseTime:     now.Add(-4 * time.Minute),
			DurationMs:       100,
			StatusCode:       500,
			Success:          false,
		},
		{
			ID:               fmt.Sprintf("log2-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-3 * time.Minute),
			ResponseTime:     now.Add(-3 * time.Minute),
			DurationMs:       100,
			StatusCode:       500,
			Success:          false,
		},
		{
			ID:               fmt.Sprintf("log3-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-2 * time.Minute),
			ResponseTime:     now.Add(-2 * time.Minute),
			DurationMs:       100,
			StatusCode:       500,
			Success:          false,
		},
		{
			ID:               fmt.Sprintf("log4-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now.Add(-1 * time.Minute),
			ResponseTime:     now.Add(-1 * time.Minute),
			DurationMs:       100,
			StatusCode:       500,
			Success:          false,
		},
		{
			ID:               fmt.Sprintf("log5-%d", timestamp),
			LoadBalancerID:   lbID,
			SelectedConfigID: configID,
			RequestTime:      now,
			ResponseTime:     now,
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
		},
	}

	for _, log := range logs {
		if err := database.CreateLoadBalancerRequestLog(log); err != nil {
			t.Fatalf("Failed to create request log: %v", err)
		}
	}

	// Create alert manager with errorRateThreshold = 0.2 (20%)
	config := AlertManagerConfig{
		CheckInterval:      1 * time.Second,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    1,
	}

	am := NewAlertManager(lbID, config)

	// Check and create alerts
	if err := am.CheckAndCreateAlerts(lbID); err != nil {
		t.Fatalf("Failed to check and create alerts: %v", err)
	}

	// Get alerts
	alerts, err := am.GetAlerts(lbID, false)
	if err != nil {
		t.Fatalf("Failed to get alerts: %v", err)
	}

	// Should have a warning alert for high error rate
	foundWarning := false
	for _, alert := range alerts {
		if alert.Level == "warning" && alert.Type == "high_error_rate" {
			foundWarning = true
			break
		}
	}

	if !foundWarning {
		t.Error("Expected warning alert for high error rate")
	}
}

func TestAlertManagerAcknowledge(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-ack-%d", timestamp)

	// Create a test alert
	alert := &database.Alert{
		ID:             fmt.Sprintf("alert-%d", timestamp),
		LoadBalancerID: lbID,
		Level:          "warning",
		Type:           "test_alert",
		Message:        "Test alert message",
		Acknowledged:   false,
	}

	if err := database.CreateAlert(alert); err != nil {
		t.Fatalf("Failed to create alert: %v", err)
	}

	// Create alert manager
	config := AlertManagerConfig{
		CheckInterval:      1 * time.Second,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    1,
	}

	am := NewAlertManager(lbID, config)

	// Acknowledge the alert
	if err := am.AcknowledgeAlert(alert.ID); err != nil {
		t.Fatalf("Failed to acknowledge alert: %v", err)
	}

	// Verify alert is acknowledged
	acknowledgedAlert, err := database.GetAlert(alert.ID)
	if err != nil {
		t.Fatalf("Failed to get alert: %v", err)
	}

	if !acknowledgedAlert.Acknowledged {
		t.Error("Expected alert to be acknowledged")
	}

	if acknowledgedAlert.AcknowledgedAt == nil {
		t.Error("Expected acknowledged_at to be set")
	}
}

func TestAlertManagerGetAlerts(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	timestamp := time.Now().UnixNano()
	lbID := fmt.Sprintf("test-lb-get-alerts-%d", timestamp)

	// Create test alerts
	alert1 := &database.Alert{
		ID:             fmt.Sprintf("alert1-%d", timestamp),
		LoadBalancerID: lbID,
		Level:          "warning",
		Type:           "test_alert_1",
		Message:        "Test alert 1",
		Acknowledged:   false,
	}

	alert2 := &database.Alert{
		ID:             fmt.Sprintf("alert2-%d", timestamp),
		LoadBalancerID: lbID,
		Level:          "critical",
		Type:           "test_alert_2",
		Message:        "Test alert 2",
		Acknowledged:   true,
	}

	if err := database.CreateAlert(alert1); err != nil {
		t.Fatalf("Failed to create alert 1: %v", err)
	}
	if err := database.CreateAlert(alert2); err != nil {
		t.Fatalf("Failed to create alert 2: %v", err)
	}

	// Create alert manager
	config := AlertManagerConfig{
		CheckInterval:      1 * time.Second,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    1,
	}

	am := NewAlertManager(lbID, config)

	// Get unacknowledged alerts
	unacknowledgedAlerts, err := am.GetAlerts(lbID, false)
	if err != nil {
		t.Fatalf("Failed to get unacknowledged alerts: %v", err)
	}

	if len(unacknowledgedAlerts) != 1 {
		t.Errorf("Expected 1 unacknowledged alert, got %d", len(unacknowledgedAlerts))
	}

	if len(unacknowledgedAlerts) > 0 && unacknowledgedAlerts[0].ID != alert1.ID {
		t.Errorf("Expected alert1, got %s", unacknowledgedAlerts[0].ID)
	}

	// Get acknowledged alerts
	acknowledgedAlerts, err := am.GetAlerts(lbID, true)
	if err != nil {
		t.Fatalf("Failed to get acknowledged alerts: %v", err)
	}

	if len(acknowledgedAlerts) != 1 {
		t.Errorf("Expected 1 acknowledged alert, got %d", len(acknowledgedAlerts))
	}

	if len(acknowledgedAlerts) > 0 && acknowledgedAlerts[0].ID != alert2.ID {
		t.Errorf("Expected alert2, got %s", acknowledgedAlerts[0].ID)
	}
}
