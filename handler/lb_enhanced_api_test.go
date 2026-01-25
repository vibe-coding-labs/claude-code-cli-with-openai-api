package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// setupTestRouter creates a test router with all enhanced API endpoints
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	// Health status endpoints
	router.GET("/api/load-balancers/:id/health", GetLoadBalancerHealthStatus)
	router.GET("/api/load-balancers/:id/health/:config_id", GetNodeHealthStatus)
	router.POST("/api/load-balancers/:id/health/check", TriggerHealthCheck)

	// Circuit breaker endpoints
	router.GET("/api/load-balancers/:id/circuit-breakers", GetLoadBalancerCircuitBreakers)
	router.POST("/api/load-balancers/:id/circuit-breakers/:config_id/reset", ResetCircuitBreaker)

	// Stats and metrics endpoints
	router.GET("/api/load-balancers/:id/stats/enhanced", GetLoadBalancerEnhancedStats)
	router.GET("/api/load-balancers/:id/metrics/realtime", GetLoadBalancerRealTimeMetrics)

	// Request logs endpoints
	router.GET("/api/load-balancers/:id/logs", GetLoadBalancerRequestLogs)

	// Alerts endpoints
	router.GET("/api/load-balancers/:id/alerts", GetLoadBalancerAlerts)
	router.GET("/api/alerts", GetAllAlerts)
	router.POST("/api/alerts/:id/acknowledge", AcknowledgeAlert)

	// Configuration endpoints
	router.GET("/api/load-balancers/:id/config", GetLoadBalancerConfig)
	router.PUT("/api/load-balancers/:id/config", UpdateLoadBalancerConfig)

	return router
}

// ptrTime returns a pointer to a time.Time
func ptrTime(t time.Time) *time.Time {
	return &t
}

// TestGetLoadBalancerHealthStatus tests the health status endpoint
func TestGetLoadBalancerHealthStatus(t *testing.T) {
	// Setup test database
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
			{ConfigID: "config-2", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Create test health statuses
	status1 := &database.HealthStatus{
		ConfigID:             "config-1",
		Status:               "healthy",
		LastCheckTime:        time.Now(),
		ConsecutiveSuccesses: 5,
		ConsecutiveFailures:  0,
		ResponseTimeMs:       100,
	}
	err = database.CreateOrUpdateHealthStatus(status1)
	assert.NoError(t, err)

	status2 := &database.HealthStatus{
		ConfigID:             "config-2",
		Status:               "unhealthy",
		LastCheckTime:        time.Now(),
		ConsecutiveSuccesses: 0,
		ConsecutiveFailures:  3,
		ResponseTimeMs:       0,
		LastError:            "connection timeout",
	}
	err = database.CreateOrUpdateHealthStatus(status2)
	assert.NoError(t, err)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lb.ID, response["load_balancer_id"])
	assert.Equal(t, float64(2), response["total_nodes"])
	assert.Equal(t, float64(1), response["healthy_nodes"])
	assert.Equal(t, float64(1), response["unhealthy_nodes"])

	statuses := response["statuses"].([]interface{})
	assert.Len(t, statuses, 2)
}

// TestGetNodeHealthStatus tests the individual node health status endpoint
func TestGetNodeHealthStatus(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test health status
	status := &database.HealthStatus{
		ConfigID:             "test-config-id",
		Status:               "healthy",
		LastCheckTime:        time.Now(),
		ConsecutiveSuccesses: 10,
		ConsecutiveFailures:  0,
		ResponseTimeMs:       50,
	}
	err := database.CreateOrUpdateHealthStatus(status)
	assert.NoError(t, err)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/test-lb/health/test-config-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response database.HealthStatus
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "test-config-id", response.ConfigID)
	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, 10, response.ConsecutiveSuccesses)
}

// TestGetLoadBalancerCircuitBreakers tests the circuit breaker status endpoint
func TestGetLoadBalancerCircuitBreakers(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
			{ConfigID: "config-2", Weight: 1, Enabled: true},
			{ConfigID: "config-3", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Create test circuit breaker states
	states := []*database.CircuitBreakerState{
		{
			ConfigID:        "config-1",
			State:           "closed",
			FailureCount:    0,
			SuccessCount:    10,
			LastStateChange: time.Now(),
		},
		{
			ConfigID:        "config-2",
			State:           "open",
			FailureCount:    5,
			SuccessCount:    0,
			LastStateChange: time.Now(),
			NextRetryTime:   ptrTime(time.Now().Add(30 * time.Second)),
		},
		{
			ConfigID:        "config-3",
			State:           "half_open",
			FailureCount:    0,
			SuccessCount:    1,
			LastStateChange: time.Now(),
		},
	}

	for _, state := range states {
		err = database.CreateOrUpdateCircuitBreakerState(state)
		assert.NoError(t, err)
	}

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/circuit-breakers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lb.ID, response["load_balancer_id"])
	assert.Equal(t, float64(3), response["total_nodes"])
	assert.Equal(t, float64(1), response["closed"])
	assert.Equal(t, float64(1), response["open"])
	assert.Equal(t, float64(1), response["half_open"])
}

// TestGetLoadBalancerEnhancedStats tests the enhanced stats endpoint
func TestGetLoadBalancerEnhancedStats(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Create test stats data
	now := time.Now()
	timeBucket := now.Truncate(time.Minute)

	// Create a basic stats record
	stats := &database.LoadBalancerStats{
		LoadBalancerID:  lb.ID,
		TimeWindow:      "1h",
		TotalRequests:   100,
		SuccessRequests: 95,
		FailedRequests:  5,
	}
	err = database.CreateLoadBalancerStats(stats)
	assert.NoError(t, err)

	// Also create some request logs for more realistic stats
	for i := 0; i < 10; i++ {
		log := &database.LoadBalancerRequestLog{
			LoadBalancerID:   lb.ID,
			SelectedConfigID: "config-1",
			RequestTime:      timeBucket.Add(-time.Duration(i) * time.Minute),
			ResponseTime:     timeBucket.Add(-time.Duration(i)*time.Minute + 100*time.Millisecond),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
			RetryCount:       0,
		}
		err = database.CreateLoadBalancerRequestLog(log)
		assert.NoError(t, err)
	}

	// Test request with different time windows
	testCases := []struct {
		window string
	}{
		{"1h"},
		{"24h"},
		{"7d"},
		{"30d"},
	}

	router := setupTestRouter()

	for _, tc := range testCases {
		t.Run("window_"+tc.window, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/stats/enhanced?window="+tc.window, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response database.LoadBalancerStats
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			assert.Equal(t, lb.ID, response.LoadBalancerID)
			assert.Equal(t, tc.window, response.TimeWindow)
		})
	}
}

// TestGetLoadBalancerRealTimeMetrics tests the real-time metrics endpoint
func TestGetLoadBalancerRealTimeMetrics(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Create some test request logs for metrics
	for i := 0; i < 5; i++ {
		log := &database.LoadBalancerRequestLog{
			LoadBalancerID:   lb.ID,
			SelectedConfigID: "config-1",
			RequestTime:      time.Now().Add(-time.Duration(i) * time.Second),
			ResponseTime:     time.Now().Add(-time.Duration(i)*time.Second + 100*time.Millisecond),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
			RetryCount:       0,
		}
		err = database.CreateLoadBalancerRequestLog(log)
		assert.NoError(t, err)
	}

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/metrics/realtime", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response database.RealTimeMetrics
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lb.ID, response.LoadBalancerID)
	assert.NotZero(t, response.Timestamp)
}

// TestGetLoadBalancerRequestLogs tests the request logs endpoint
func TestGetLoadBalancerRequestLogs(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Create test request logs
	for i := 0; i < 5; i++ {
		log := &database.LoadBalancerRequestLog{
			LoadBalancerID:   lb.ID,
			SelectedConfigID: "config-1",
			RequestTime:      time.Now().Add(-time.Duration(i) * time.Minute),
			ResponseTime:     time.Now().Add(-time.Duration(i)*time.Minute + 100*time.Millisecond),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
			RetryCount:       0,
		}
		err = database.CreateLoadBalancerRequestLog(log)
		assert.NoError(t, err)
	}

	// Test request with pagination
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/logs?limit=3&offset=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lb.ID, response["load_balancer_id"])
	assert.Equal(t, float64(5), response["total_count"])
	assert.Equal(t, float64(3), response["limit"])
	assert.Equal(t, float64(0), response["offset"])

	logs := response["logs"].([]interface{})
	assert.Len(t, logs, 3)
}

// TestGetLoadBalancerAlerts tests the alerts endpoint
func TestGetLoadBalancerAlerts(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Create test alerts
	alerts := []*database.Alert{
		{
			LoadBalancerID: lb.ID,
			Level:          "critical",
			Type:           "all_nodes_down",
			Message:        "All nodes are unhealthy",
			Acknowledged:   false,
		},
		{
			LoadBalancerID: lb.ID,
			Level:          "warning",
			Type:           "high_error_rate",
			Message:        "Error rate exceeds threshold",
			Acknowledged:   true,
		},
	}

	for _, alert := range alerts {
		err = database.CreateAlert(alert)
		assert.NoError(t, err)
	}

	// Test request for unacknowledged alerts
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/alerts?acknowledged=false", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lb.ID, response["load_balancer_id"])
	assert.Equal(t, float64(1), response["unacknowledged_count"])

	alertsList := response["alerts"].([]interface{})
	assert.Len(t, alertsList, 1)
}

// TestAcknowledgeAlert tests the alert acknowledgment endpoint
func TestAcknowledgeAlert(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test alert
	alert := &database.Alert{
		LoadBalancerID: "test-lb",
		Level:          "warning",
		Type:           "high_error_rate",
		Message:        "Error rate exceeds threshold",
		Acknowledged:   false,
	}
	err := database.CreateAlert(alert)
	assert.NoError(t, err)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/api/alerts/"+alert.ID+"/acknowledge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Alert acknowledged successfully", response["message"])

	// Verify alert was acknowledged
	updatedAlert, err := database.GetAlert(alert.ID)
	assert.NoError(t, err)
	assert.True(t, updatedAlert.Acknowledged)
	assert.NotNil(t, updatedAlert.AcknowledgedAt)
}

// TestGetAllAlerts tests the global alerts endpoint
func TestGetAllAlerts(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Clean up existing alerts first
	database.DB.Exec("DELETE FROM alerts")

	// Create test alerts for different load balancers
	alerts := []*database.Alert{
		{
			LoadBalancerID: "lb-1",
			Level:          "critical",
			Type:           "all_nodes_down",
			Message:        "All nodes down",
			Acknowledged:   false,
		},
		{
			LoadBalancerID: "lb-2",
			Level:          "warning",
			Type:           "high_error_rate",
			Message:        "High error rate",
			Acknowledged:   false,
		},
		{
			LoadBalancerID: "lb-1",
			Level:          "info",
			Type:           "circuit_breaker_open",
			Message:        "Circuit breaker opened",
			Acknowledged:   true,
		},
	}

	for _, alert := range alerts {
		err := database.CreateAlert(alert)
		assert.NoError(t, err)
	}

	// Test request with filters
	testCases := []struct {
		name          string
		query         string
		expectedCount int
	}{
		{"all_alerts", "", 3},
		{"unacknowledged", "?acknowledged=false", 2},
		{"acknowledged", "?acknowledged=true", 1},
		{"critical_level", "?level=critical", 1},
		{"warning_level", "?level=warning", 1},
	}

	router := setupTestRouter()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/alerts"+tc.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			alertsList := response["alerts"].([]interface{})
			assert.GreaterOrEqual(t, len(alertsList), tc.expectedCount, "Should have at least the expected number of alerts")
		})
	}
}

// TestUpdateLoadBalancerConfig tests the configuration update endpoint
func TestUpdateLoadBalancerConfig(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Prepare update request
	updateReq := map[string]interface{}{
		"health_check_enabled":   true,
		"health_check_interval":  60,
		"failure_threshold":      5,
		"recovery_threshold":     3,
		"max_retries":            5,
		"circuit_breaker_enabled": true,
		"error_rate_threshold":   0.3,
		"log_level":              "detailed",
	}

	body, _ := json.Marshal(updateReq)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("PUT", "/api/load-balancers/"+lb.ID+"/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Configuration updated successfully", response["message"])
	assert.Equal(t, lb.ID, response["load_balancer_id"])
}

// TestGetLoadBalancerConfig tests the configuration retrieval endpoint
func TestGetLoadBalancerConfig(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "weighted",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 2, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/api/load-balancers/"+lb.ID+"/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lb.ID, response["load_balancer_id"])
	assert.Equal(t, "Test LB", response["name"])
	assert.Equal(t, "weighted", response["strategy"])
	assert.Equal(t, true, response["enabled"])
}

// TestTriggerHealthCheck tests the manual health check trigger endpoint
func TestTriggerHealthCheck(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test load balancer
	lb := &database.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config-1", Weight: 1, Enabled: true},
		},
		Enabled: true,
	}
	err := database.CreateLoadBalancer(lb)
	assert.NoError(t, err)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/api/load-balancers/"+lb.ID+"/health/check", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Health check triggered", response["message"])
	assert.Equal(t, lb.ID, response["load_balancer_id"])
}

// TestResetCircuitBreaker tests the circuit breaker reset endpoint
func TestResetCircuitBreaker(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test circuit breaker state
	state := &database.CircuitBreakerState{
		ConfigID:        "test-config-id",
		State:           "open",
		FailureCount:    10,
		SuccessCount:    0,
		LastStateChange: time.Now(),
	}
	err := database.CreateOrUpdateCircuitBreakerState(state)
	assert.NoError(t, err)

	// Test request
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/api/load-balancers/test-lb/circuit-breakers/test-config-id/reset", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Circuit breaker reset successfully", response["message"])
	assert.Equal(t, "test-config-id", response["config_id"])

	// Verify circuit breaker was reset
	updatedState, err := database.GetCircuitBreakerState("test-config-id")
	assert.NoError(t, err)
	assert.Equal(t, "closed", updatedState.State)
}

// TestAPIEndpointsNotFound tests 404 responses
func TestAPIEndpointsNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	router := setupTestRouter()

	testCases := []struct {
		name   string
		method string
		path   string
	}{
		{"health_status_not_found", "GET", "/api/load-balancers/nonexistent/health"},
		{"node_health_not_found", "GET", "/api/load-balancers/test/health/nonexistent"},
		{"config_not_found", "GET", "/api/load-balancers/nonexistent/config"},
		{"metrics_not_found", "GET", "/api/load-balancers/nonexistent/metrics/realtime"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}
