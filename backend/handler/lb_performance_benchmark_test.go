package handler

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// setupBenchmarkTestDB initializes a test database for benchmark tests
func setupBenchmarkTestDB(t testing.TB) {
	// Clean up any existing test database
	os.Remove("test_benchmark.db")
	os.Remove("test_benchmark.db-shm")
	os.Remove("test_benchmark.db-wal")
	
	// Initialize test database
	database.InitDB("test_benchmark.db")
	
	// Initialize encryption
	if err := database.InitEncryption(); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}
	
	// Run migrations - ignore duplicate column errors
	if err := database.RunMigrations(); err != nil {
		// Check if it's a duplicate column error (which is acceptable)
		if !strings.Contains(err.Error(), "duplicate column") {
			t.Fatalf("Failed to run migrations: %v", err)
		}
		// Otherwise, log the warning but continue
		t.Logf("Warning: Migration error (ignored): %v", err)
	}
}

// createBenchmarkAPIConfigs creates test API configurations for benchmarks
func createBenchmarkAPIConfigs(t testing.TB, count int) {
	for i := 1; i <= count; i++ {
		config := &database.APIConfig{
			ID:              fmt.Sprintf("config_%d", i),
			Name:            fmt.Sprintf("Config %d", i),
			AnthropicAPIKey: fmt.Sprintf("test_key_%d", i),
			Enabled:         true,
		}
		if err := database.CreateAPIConfig(config); err != nil {
			t.Fatalf("Failed to create API config: %v", err)
		}
	}
}

// cleanupBenchmarkTestDB cleans up the test database
func cleanupBenchmarkTestDB() {
	if database.DB != nil {
		database.DB.Close()
	}
	os.Remove("test_benchmark.db")
	os.Remove("test_benchmark.db-shm")
	os.Remove("test_benchmark.db-wal")
}

// TestLoadBalancerOverhead measures the total overhead of load balancer operations
func TestLoadBalancerOverhead(t *testing.T) {
	// Setup test database
	setupBenchmarkTestDB(t)
	defer cleanupBenchmarkTestDB()

	// Create API configs
	createBenchmarkAPIConfigs(t, 3)

	// Create test load balancer in database
	lb := &database.LoadBalancer{
		ID:       "test-lb",
		Name:     "Test LB",
		Strategy: "weighted_round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config_1", Weight: 10, Enabled: true},
			{ConfigID: "config_2", Weight: 10, Enabled: true},
			{ConfigID: "config_3", Weight: 10, Enabled: true},
		},
	}
	
	// Save load balancer to database
	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Initialize components
	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbMgr := NewCircuitBreakerManager(cbConfig)
	
	selector, err := NewEnhancedSelector(lb, cbMgr)
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	cacheManager := NewCacheManager(5 * time.Minute)
	cache := cacheManager.GetHealthStatusCache()
	
	// Pre-populate cache with health statuses
	for _, config := range lb.ConfigNodes {
		cache.Set(config.ConfigID, &database.HealthStatus{
			ConfigID: config.ConfigID,
			Status:   "healthy",
		}, 5*time.Minute)
	}

	ctx := context.Background()

	// Measure overhead for multiple iterations
	iterations := 10000
	durations := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		
		// Simulate load balancer operations
		// 1. Get health status from cache
		_, _ = cache.Get(lb.ConfigNodes[i%len(lb.ConfigNodes)].ConfigID)
		
		// 2. Select config
		_, _ = selector.SelectConfig(ctx)
		
		durations[i] = time.Since(start)
	}

	// Calculate statistics
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	p50 := durations[len(durations)*50/100]
	p90 := durations[len(durations)*90/100]
	p95 := durations[len(durations)*95/100]
	p99 := durations[len(durations)*99/100]

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	avg := total / time.Duration(len(durations))

	// Print results
	t.Logf("\n=== Load Balancer Overhead Benchmark ===")
	t.Logf("Iterations: %d", iterations)
	t.Logf("Average:    %v", avg)
	t.Logf("P50:        %v", p50)
	t.Logf("P90:        %v", p90)
	t.Logf("P95:        %v", p95)
	t.Logf("P99:        %v", p99)
	t.Logf("========================================")

	// Verify P99 is under 10ms
	maxAllowedLatency := 10 * time.Millisecond
	if p99 > maxAllowedLatency {
		t.Errorf("P99 latency (%v) exceeds maximum allowed latency (%v)", p99, maxAllowedLatency)
	} else {
		t.Logf("✅ P99 latency (%v) is within acceptable range (< %v)", p99, maxAllowedLatency)
	}
}

// TestConcurrentLoadBalancerOverhead measures overhead under concurrent load
func TestConcurrentLoadBalancerOverhead(t *testing.T) {
	// Setup test database
	setupBenchmarkTestDB(t)
	defer cleanupBenchmarkTestDB()

	// Create API configs
	createBenchmarkAPIConfigs(t, 3)

	// Create test load balancer in database
	lb := &database.LoadBalancer{
		ID:       "test-lb",
		Name:     "Test LB",
		Strategy: "weighted_round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config_1", Weight: 10, Enabled: true},
			{ConfigID: "config_2", Weight: 10, Enabled: true},
			{ConfigID: "config_3", Weight: 10, Enabled: true},
		},
	}
	
	// Save load balancer to database
	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Initialize components
	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbMgr := NewCircuitBreakerManager(cbConfig)
	
	selector, err := NewEnhancedSelector(lb, cbMgr)
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	cacheManager := NewCacheManager(5 * time.Minute)
	cache := cacheManager.GetHealthStatusCache()
	
	// Pre-populate cache with health statuses
	for _, config := range lb.ConfigNodes {
		cache.Set(config.ConfigID, &database.HealthStatus{
			ConfigID: config.ConfigID,
			Status:   "healthy",
		}, 5*time.Minute)
	}

	ctx := context.Background()

	// Measure overhead with concurrent requests
	concurrency := 100
	requestsPerGoroutine := 100
	totalRequests := concurrency * requestsPerGoroutine
	
	durations := make([]time.Duration, totalRequests)
	durationsChan := make(chan time.Duration, totalRequests)

	start := time.Now()
	
	// Launch concurrent goroutines
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			for j := 0; j < requestsPerGoroutine; j++ {
				reqStart := time.Now()
				
				// Simulate load balancer operations
				_, _ = cache.Get(lb.ConfigNodes[(id+j)%len(lb.ConfigNodes)].ConfigID)
				_, _ = selector.SelectConfig(ctx)
				
				durationsChan <- time.Since(reqStart)
			}
		}(i)
	}

	// Collect results
	for i := 0; i < totalRequests; i++ {
		durations[i] = <-durationsChan
	}
	
	totalDuration := time.Since(start)

	// Calculate statistics
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	p50 := durations[len(durations)*50/100]
	p90 := durations[len(durations)*90/100]
	p95 := durations[len(durations)*95/100]
	p99 := durations[len(durations)*99/100]

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	avg := total / time.Duration(len(durations))
	
	throughput := float64(totalRequests) / totalDuration.Seconds()

	// Print results
	t.Logf("\n=== Concurrent Load Balancer Overhead Benchmark ===")
	t.Logf("Concurrency:     %d", concurrency)
	t.Logf("Total Requests:  %d", totalRequests)
	t.Logf("Total Duration:  %v", totalDuration)
	t.Logf("Throughput:      %.2f req/s", throughput)
	t.Logf("Average Latency: %v", avg)
	t.Logf("P50:             %v", p50)
	t.Logf("P90:             %v", p90)
	t.Logf("P95:             %v", p95)
	t.Logf("P99:             %v", p99)
	t.Logf("===================================================")

	// Verify P99 is under 10ms
	maxAllowedLatency := 10 * time.Millisecond
	if p99 > maxAllowedLatency {
		t.Errorf("P99 latency (%v) exceeds maximum allowed latency (%v)", p99, maxAllowedLatency)
	} else {
		t.Logf("✅ P99 latency (%v) is within acceptable range (< %v)", p99, maxAllowedLatency)
	}

	// Verify throughput is at least 1000 req/s
	minThroughput := 1000.0
	if throughput < minThroughput {
		t.Errorf("Throughput (%.2f req/s) is below minimum requirement (%.2f req/s)", throughput, minThroughput)
	} else {
		t.Logf("✅ Throughput (%.2f req/s) meets requirement (>= %.2f req/s)", throughput, minThroughput)
	}
}
