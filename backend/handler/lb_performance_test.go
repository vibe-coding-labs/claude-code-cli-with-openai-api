package handler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// setupPerformanceTestDB initializes a test database for performance tests
func setupPerformanceTestDB(t *testing.T) {
	// Clean up any existing test database
	os.Remove("test_performance.db")
	os.Remove("test_performance.db-shm")
	os.Remove("test_performance.db-wal")
	
	// Initialize test database
	database.InitDB("test_performance.db")
	
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

// createTestAPIConfigs creates test API configurations
func createTestAPIConfigs(t *testing.T, count int) {
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

// cleanupPerformanceTestDB cleans up the test database
func cleanupPerformanceTestDB() {
	if database.DB != nil {
		database.DB.Close()
	}
	os.Remove("test_performance.db")
	os.Remove("test_performance.db-shm")
	os.Remove("test_performance.db-wal")
}

// BenchmarkSelectorPerformance benchmarks the selector performance
func BenchmarkSelectorPerformance(b *testing.B) {
	// Create test load balancer
	lb := &database.LoadBalancer{
		ID:       "bench-lb",
		Name:     "Benchmark LB",
		Strategy: "weighted_round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config_1", Weight: 10, Enabled: true},
			{ConfigID: "config_2", Weight: 10, Enabled: true},
			{ConfigID: "config_3", Weight: 10, Enabled: true},
		},
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
		b.Fatalf("Failed to create selector: %v", err)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = selector.SelectConfig(ctx)
		}
	})
}

// BenchmarkCachePerformance benchmarks the cache performance
func BenchmarkCachePerformance(b *testing.B) {
	cacheManager := NewCacheManager(5 * time.Minute)
	cache := cacheManager.GetHealthStatusCache()
	
	// Pre-populate cache
	for i := 0; i < 100; i++ {
		configID := fmt.Sprintf("config-%d", i)
		cache.Set(configID, &database.HealthStatus{
			ConfigID: configID,
			Status:   "healthy",
		}, 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			configID := fmt.Sprintf("config-%d", i%100)
			_, _ = cache.Get(configID)
			i++
		}
	})
}

// BenchmarkCircuitBreakerPerformance benchmarks the circuit breaker performance
func BenchmarkCircuitBreakerPerformance(b *testing.B) {
	cb := NewCircuitBreaker("test-config", 0.5, 30*time.Second, 30*time.Second, 3)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Call(ctx, func() error {
				return nil
			})
		}
	})
}

// BenchmarkRetryHandlerPerformance benchmarks the retry handler performance
func BenchmarkRetryHandlerPerformance(b *testing.B) {
	// Create test load balancer
	lb := &database.LoadBalancer{
		ID:       "bench-lb-retry",
		Name:     "Benchmark LB Retry",
		Strategy: "round_robin",
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "config_1", Weight: 1, Enabled: true},
			{ConfigID: "config_2", Weight: 1, Enabled: true},
		},
	}

	cbConfig := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		WindowDuration:     30 * time.Second,
		Timeout:            30 * time.Second,
		HalfOpenRequests:   3,
	}
	cbMgr := NewCircuitBreakerManager(cbConfig)
	
	selector, err := NewEnhancedSelector(lb, cbMgr)
	if err != nil {
		b.Fatalf("Failed to create selector: %v", err)
	}

	retryHandler := NewRetryHandler(
		3,
		100*time.Millisecond,
		5*time.Second,
		selector,
		cbMgr,
	)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retryHandler.ExecuteWithRetry(ctx, func(config *database.APIConfig) error {
			return nil
		})
	}
}

// TestLoadBalancerThroughput tests the throughput of the load balancer
func TestLoadBalancerThroughput(t *testing.T) {
	// Setup test database
	setupPerformanceTestDB(t)
	defer cleanupPerformanceTestDB()

	// Create API configs
	createTestAPIConfigs(t, 3)

	// Create test load balancer in database
	lb := &database.LoadBalancer{
		ID:       "test-lb-throughput",
		Name:     "Test LB Throughput",
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

	// Test throughput with different concurrency levels
	concurrencyLevels := []int{10, 50, 100, 500, 1000}
	
	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			requestsPerGoroutine := 100
			totalRequests := concurrency * requestsPerGoroutine
			
			var successCount int64
			var errorCount int64
			
			start := time.Now()
			
			var wg sync.WaitGroup
			wg.Add(concurrency)
			
			// Launch concurrent goroutines
			for i := 0; i < concurrency; i++ {
				go func() {
					defer wg.Done()
					for j := 0; j < requestsPerGoroutine; j++ {
						_, err := selector.SelectConfig(ctx)
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						} else {
							atomic.AddInt64(&successCount, 1)
						}
					}
				}()
			}
			
			wg.Wait()
			
			duration := time.Since(start)
			throughput := float64(totalRequests) / duration.Seconds()
			
			t.Logf("\n=== Throughput Test (Concurrency: %d) ===", concurrency)
			t.Logf("Total Requests:  %d", totalRequests)
			t.Logf("Success:         %d", successCount)
			t.Logf("Errors:          %d", errorCount)
			t.Logf("Duration:        %v", duration)
			t.Logf("Throughput:      %.2f req/s", throughput)
			t.Logf("==========================================")
			
			// Verify throughput meets requirements
			if concurrency <= 100 {
				minThroughput := 1000.0
				if throughput < minThroughput {
					t.Errorf("Throughput (%.2f req/s) is below minimum requirement (%.2f req/s)", throughput, minThroughput)
				} else {
					t.Logf("✅ Throughput meets requirement (>= %.2f req/s)", minThroughput)
				}
			}
		})
	}
}

// TestLoadBalancerLatency tests the latency of the load balancer
func TestLoadBalancerLatency(t *testing.T) {
	// Setup test database
	setupPerformanceTestDB(t)
	defer cleanupPerformanceTestDB()

	// Create API configs
	createTestAPIConfigs(t, 3)

	// Create test load balancer in database
	lb := &database.LoadBalancer{
		ID:       "test-lb-latency",
		Name:     "Test LB Latency",
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

	// Measure latency for multiple iterations
	iterations := 10000
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, _ = selector.SelectConfig(ctx)
		latencies[i] = time.Since(start)
	}

	// Calculate percentiles
	p50, p90, p95, p99, avg := calculatePercentiles(latencies)

	t.Logf("\n=== Latency Test ===")
	t.Logf("Iterations: %d", iterations)
	t.Logf("Average:    %v", avg)
	t.Logf("P50:        %v", p50)
	t.Logf("P90:        %v", p90)
	t.Logf("P95:        %v", p95)
	t.Logf("P99:        %v", p99)
	t.Logf("====================")

	// Verify P99 is under 10ms
	maxAllowedLatency := 10 * time.Millisecond
	if p99 > maxAllowedLatency {
		t.Errorf("P99 latency (%v) exceeds maximum allowed latency (%v)", p99, maxAllowedLatency)
	} else {
		t.Logf("✅ P99 latency (%v) is within acceptable range (< %v)", p99, maxAllowedLatency)
	}
}

// TestConcurrentLoadBalancerLatency tests latency under concurrent load
func TestConcurrentLoadBalancerLatency(t *testing.T) {
	// Setup test database
	setupPerformanceTestDB(t)
	defer cleanupPerformanceTestDB()

	// Create API configs
	createTestAPIConfigs(t, 3)

	// Create test load balancer in database
	lb := &database.LoadBalancer{
		ID:       "test-lb-concurrent-latency",
		Name:     "Test LB Concurrent Latency",
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

	// Measure latency with concurrent requests
	concurrency := 100
	requestsPerGoroutine := 100
	totalRequests := concurrency * requestsPerGoroutine
	
	latencies := make([]time.Duration, totalRequests)
	latenciesChan := make(chan time.Duration, totalRequests)

	start := time.Now()
	
	var wg sync.WaitGroup
	wg.Add(concurrency)
	
	// Launch concurrent goroutines
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				reqStart := time.Now()
				_, _ = selector.SelectConfig(ctx)
				latenciesChan <- time.Since(reqStart)
			}
		}()
	}
	
	wg.Wait()
	close(latenciesChan)
	
	// Collect results
	i := 0
	for latency := range latenciesChan {
		latencies[i] = latency
		i++
	}
	
	totalDuration := time.Since(start)

	// Calculate percentiles
	p50, p90, p95, p99, avg := calculatePercentiles(latencies)
	
	throughput := float64(totalRequests) / totalDuration.Seconds()

	t.Logf("\n=== Concurrent Latency Test ===")
	t.Logf("Concurrency:     %d", concurrency)
	t.Logf("Total Requests:  %d", totalRequests)
	t.Logf("Total Duration:  %v", totalDuration)
	t.Logf("Throughput:      %.2f req/s", throughput)
	t.Logf("Average Latency: %v", avg)
	t.Logf("P50:             %v", p50)
	t.Logf("P90:             %v", p90)
	t.Logf("P95:             %v", p95)
	t.Logf("P99:             %v", p99)
	t.Logf("================================")

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

// Helper function to calculate percentiles
func calculatePercentiles(durations []time.Duration) (p50, p90, p95, p99, avg time.Duration) {
	if len(durations) == 0 {
		return 0, 0, 0, 0, 0
	}

	// Sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	
	// Simple bubble sort for small arrays, or use sort.Slice for larger ones
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 = sorted[len(sorted)*50/100]
	p90 = sorted[len(sorted)*90/100]
	p95 = sorted[len(sorted)*95/100]
	p99 = sorted[len(sorted)*99/100]

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	avg = total / time.Duration(len(durations))

	return p50, p90, p95, p99, avg
}
