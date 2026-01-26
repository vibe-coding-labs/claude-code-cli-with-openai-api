package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// TestE2E_CompleteRequestFlow tests the complete request flow through the load balancer
func TestE2E_CompleteRequestFlow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test configs
	config1 := createTestConfigE2E(t, "config1-e2e", "http://backend1.test")
	config2 := createTestConfigE2E(t, "config2-e2e", "http://backend2.test")

	// Create load balancer
	lb := &database.LoadBalancer{
		ID:          "test-lb-e2e",
		Name:        "Test LB",
		Description: "E2E Test Load Balancer",
		Strategy:    "round_robin",
		Enabled:     true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: config1.ID, Weight: 1, Enabled: true},
			{ConfigID: config2.ID, Weight: 1, Enabled: true},
		},
		HealthCheckEnabled:    true,
		HealthCheckInterval:   10,
		FailureThreshold:      3,
		RecoveryThreshold:     2,
		MaxRetries:            3,
		CircuitBreakerEnabled: true,
		ErrorRateThreshold:    0.5,
	}
	err := database.CreateLoadBalancer(lb)
	require.NoError(t, err)

	// Create circuit breaker manager
	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbMgr := NewCircuitBreakerManager(cbConfig)

	// Create selector
	selector, err := NewEnhancedSelector(lb, cbMgr)
	require.NoError(t, err)

	// Test: Select configs in round-robin fashion
	ctx := context.Background()

	// First request should go to config1
	selectedConfig, err := selector.SelectConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, config1.ID, selectedConfig.ID)

	// Second request should go to config2
	selectedConfig, err = selector.SelectConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, config2.ID, selectedConfig.ID)

	// Third request should go back to config1
	selectedConfig, err = selector.SelectConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, config1.ID, selectedConfig.ID)

	t.Logf("✓ Round-robin selection working correctly")
}

// TestE2E_HealthCheckFlow tests the complete health check flow
func TestE2E_HealthCheckFlow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test config
	config := createTestConfigE2E(t, "config1-health", "http://backend.test")

	// Create load balancer
	lb := &database.LoadBalancer{
		ID:                  "test-lb-health",
		Name:                "Test LB Health",
		Strategy:            "round_robin",
		Enabled:             true,
		ConfigNodes:         []database.ConfigNode{{ConfigID: config.ID, Weight: 1, Enabled: true}},
		HealthCheckEnabled:  true,
		HealthCheckInterval: 1, // 1 second for faster testing
		FailureThreshold:    2,
		RecoveryThreshold:   2,
		HealthCheckTimeout:  5,
	}
	err := database.CreateLoadBalancer(lb)
	require.NoError(t, err)

	// Create mock backend that fails initially
	var failCount int
	var mu sync.Mutex
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		failCount++
		shouldFail := failCount <= 2
		mu.Unlock()

		if shouldFail {
			// Fail first 2 requests
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed after that
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()

	// Update config URL
	config.OpenAIBaseURL = backend.URL
	err = database.UpdateAPIConfig(config)
	require.NoError(t, err)

	// Create health checker
	checker := NewHealthChecker(lb.ID,
		time.Duration(lb.HealthCheckInterval)*time.Second,
		time.Duration(lb.HealthCheckTimeout)*time.Second,
		lb.FailureThreshold,
		lb.RecoveryThreshold)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start health checker
	go checker.Start(ctx)

	// Wait for initial health checks
	time.Sleep(3 * time.Second)

	// Check that node is marked as unhealthy after failures
	status, err := checker.GetHealthStatus(config.ID)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", status.Status)
	assert.GreaterOrEqual(t, status.ConsecutiveFailures, 2)

	t.Logf("✓ Node marked as unhealthy after %d failures", status.ConsecutiveFailures)

	// Wait for recovery checks
	time.Sleep(3 * time.Second)

	// Check that node recovered
	status, err = checker.GetHealthStatus(config.ID)
	require.NoError(t, err)
	assert.Equal(t, "healthy", status.Status)
	assert.GreaterOrEqual(t, status.ConsecutiveSuccesses, 2)

	t.Logf("✓ Node recovered to healthy after %d successes", status.ConsecutiveSuccesses)
}

// TestE2E_CircuitBreakerFlow tests the complete circuit breaker flow
func TestE2E_CircuitBreakerFlow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test config
	config := createTestConfigE2E(t, "config1-cb", "http://backend.test")

	// Create circuit breaker with low thresholds for testing
	cb := NewCircuitBreaker(config.ID,
		0.5,           // 50% error rate
		5*time.Second, // 5 second window
		2*time.Second, // 2 second timeout
		2)             // 2 test requests

	ctx := context.Background()

	// Test: Circuit breaker starts in closed state
	assert.Equal(t, "closed", cb.GetState())
	t.Logf("✓ Circuit breaker starts in closed state")

	// Simulate failures to trigger circuit breaker
	for i := 0; i < 10; i++ {
		cb.Call(ctx, func() error {
			return fmt.Errorf("simulated failure")
		})
	}

	// Circuit breaker should now be open
	assert.Equal(t, "open", cb.GetState())
	t.Logf("✓ Circuit breaker opened after failures")

	// Test: Requests should fail fast when circuit is open
	err := cb.Call(ctx, func() error {
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
	t.Logf("✓ Requests fail fast when circuit is open")

	// Wait for timeout to transition to half-open
	time.Sleep(3 * time.Second)

	// Make a successful request to trigger the transition to half-open
	err = cb.Call(ctx, func() error {
		return nil
	})
	assert.NoError(t, err)

	// Circuit breaker should now be in half-open state
	state := cb.GetState()
	assert.Equal(t, "half_open", state)
	t.Logf("✓ Circuit breaker transitioned to half-open after timeout")

	// Test: Successful requests in half-open should close the circuit
	for i := 0; i < 2; i++ {
		err = cb.Call(ctx, func() error {
			return nil
		})
		assert.NoError(t, err)
	}

	// Circuit breaker should now be closed
	assert.Equal(t, "closed", cb.GetState())
	t.Logf("✓ Circuit breaker closed after successful test requests")
}

// TestE2E_RetryFlow tests the complete retry flow
func TestE2E_RetryFlow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test configs
	config1 := createTestConfigE2E(t, "config1-retry", "http://backend1.test")
	config2 := createTestConfigE2E(t, "config2-retry", "http://backend2.test")

	// Create load balancer
	lb := &database.LoadBalancer{
		ID:       "test-lb-retry",
		Name:     "Test LB Retry",
		Strategy: "round_robin",
		Enabled:  true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: config1.ID, Weight: 1, Enabled: true},
			{ConfigID: config2.ID, Weight: 1, Enabled: true},
		},
		MaxRetries:        3,
		InitialRetryDelay: 100,
		MaxRetryDelay:     1000,
	}
	err := database.CreateLoadBalancer(lb)
	require.NoError(t, err)

	// Create circuit breaker manager
	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbManager := NewCircuitBreakerManager(cbConfig)

	// Create selector
	selector, err := NewEnhancedSelector(lb, cbManager)
	require.NoError(t, err)

	// Create retry handler
	retryHandler := NewRetryHandler(
		lb.MaxRetries,
		time.Duration(lb.InitialRetryDelay)*time.Millisecond,
		time.Duration(lb.MaxRetryDelay)*time.Millisecond,
		selector,
		cbManager,
	)

	// Test: Retry with different nodes
	ctx := context.Background()
	attempts := 0
	err = retryHandler.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
		attempts++
		if attempts <= 2 {
			return fmt.Errorf("temporary error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
	t.Logf("✓ Retry succeeded after %d attempts", attempts)
}

// TestE2E_FailoverScenario tests complete failover scenario
func TestE2E_FailoverScenario(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test configs
	config1 := createTestConfigE2E(t, "config1-failover", "http://backend1.test")
	config2 := createTestConfigE2E(t, "config2-failover", "http://backend2.test")

	// Create load balancer
	lb := &database.LoadBalancer{
		ID:       "test-lb-failover",
		Name:     "Test LB Failover",
		Strategy: "round_robin",
		Enabled:  true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: config1.ID, Weight: 1, Enabled: true},
			{ConfigID: config2.ID, Weight: 1, Enabled: true},
		},
		HealthCheckEnabled:  true,
		HealthCheckInterval: 1,
		FailureThreshold:    2,
		RecoveryThreshold:   2,
	}
	err := database.CreateLoadBalancer(lb)
	require.NoError(t, err)

	// Mark config1 as unhealthy
	err = database.CreateOrUpdateHealthStatus(&database.HealthStatus{
		ConfigID:             config1.ID,
		Status:               "unhealthy",
		LastCheckTime:        time.Now(),
		ConsecutiveFailures:  3,
		ConsecutiveSuccesses: 0,
	})
	require.NoError(t, err)

	// Mark config2 as healthy
	err = database.CreateOrUpdateHealthStatus(&database.HealthStatus{
		ConfigID:             config2.ID,
		Status:               "healthy",
		LastCheckTime:        time.Now(),
		ConsecutiveFailures:  0,
		ConsecutiveSuccesses: 3,
	})
	require.NoError(t, err)

	// Create circuit breaker manager
	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbManager := NewCircuitBreakerManager(cbConfig)

	// Create selector
	selector, err := NewEnhancedSelector(lb, cbManager)
	require.NoError(t, err)

	// Test: Selector should only return config2 (healthy node)
	for i := 0; i < 5; i++ {
		selectedConfig, err := selector.SelectConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, config2.ID, selectedConfig.ID, "Should only select healthy node")
	}

	t.Logf("✓ Failover working: only healthy node selected")
}

// TestE2E_ConcurrentRequests tests handling of concurrent requests
func TestE2E_ConcurrentRequests(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Create test configs
	config1 := createTestConfigE2E(t, "config1-concurrent", "http://backend1.test")
	config2 := createTestConfigE2E(t, "config2-concurrent", "http://backend2.test")

	// Create load balancer
	lb := &database.LoadBalancer{
		ID:       "test-lb-concurrent",
		Name:     "Test LB Concurrent",
		Strategy: "round_robin",
		Enabled:  true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: config1.ID, Weight: 1, Enabled: true},
			{ConfigID: config2.ID, Weight: 1, Enabled: true},
		},
	}
	err := database.CreateLoadBalancer(lb)
	require.NoError(t, err)

	// Create circuit breaker manager
	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbManager := NewCircuitBreakerManager(cbConfig)

	// Create selector
	selector, err := NewEnhancedSelector(lb, cbManager)
	require.NoError(t, err)

	// Test: Concurrent requests
	const numRequests = 100
	results := make(chan string, numRequests)
	ctx := context.Background()

	for i := 0; i < numRequests; i++ {
		go func() {
			config, err := selector.SelectConfig(ctx)
			if err != nil {
				results <- ""
				return
			}
			results <- config.ID
		}()
	}

	// Collect results
	config1Count := 0
	config2Count := 0
	for i := 0; i < numRequests; i++ {
		configID := <-results
		if configID == config1.ID {
			config1Count++
		} else if configID == config2.ID {
			config2Count++
		}
	}

	// Both configs should be selected roughly equally
	assert.Greater(t, config1Count, 0)
	assert.Greater(t, config2Count, 0)
	t.Logf("✓ Concurrent requests handled: config1=%d, config2=%d", config1Count, config2Count)
}

// Helper function to create test config
func createTestConfigE2E(t *testing.T, id, baseURL string) *database.APIConfig {
	config := &database.APIConfig{
		ID:             id,
		Name:           "Test Config " + id,
		Description:    "Test configuration",
		OpenAIBaseURL:  baseURL,
		OpenAIAPIKey:   "test-key",
		BigModel:       "gpt-4",
		MiddleModel:    "gpt-3.5-turbo",
		SmallModel:     "gpt-3.5-turbo",
		MaxTokensLimit: 4096,
		RequestTimeout: 30,
		Enabled:        true,
	}
	err := database.CreateAPIConfig(config)
	require.NoError(t, err)

	// Create healthy status for the config
	err = database.CreateOrUpdateHealthStatus(&database.HealthStatus{
		ConfigID:      config.ID,
		Status:        "healthy",
		LastCheckTime: time.Now(),
	})
	require.NoError(t, err)

	return config
}
