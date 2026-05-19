package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestNewConnectionPool(t *testing.T) {
	config := &database.APIConfig{
		ID:             "test-config-1",
		Name:           "Test Config",
		OpenAIBaseURL:  "https://api.openai.com",
		OpenAIAPIKey:   "test-key",
		Enabled:        true,
	}

	poolConfig := DefaultConnectionPoolConfig()
	pool, err := NewConnectionPool(config, poolConfig)

	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	if pool == nil {
		t.Fatal("Connection pool is nil")
	}

	if pool.GetConfigID() != config.ID {
		t.Errorf("Expected config ID %s, got %s", config.ID, pool.GetConfigID())
	}

	if pool.GetBaseURL() != config.OpenAIBaseURL {
		t.Errorf("Expected base URL %s, got %s", config.OpenAIBaseURL, pool.GetBaseURL())
	}

	pool.Close()
}

func TestConnectionPoolManager(t *testing.T) {
	manager := NewConnectionPoolManager()

	config1 := &database.APIConfig{
		ID:             "test-config-1",
		Name:           "Test Config 1",
		OpenAIBaseURL:  "https://api.openai.com",
		OpenAIAPIKey:   "test-key-1",
		Enabled:        true,
	}

	config2 := &database.APIConfig{
		ID:             "test-config-2",
		Name:           "Test Config 2",
		OpenAIBaseURL:  "https://api.anthropic.com",
		OpenAIAPIKey:   "test-key-2",
		Enabled:        true,
	}

	// Test GetOrCreatePool
	pool1, err := manager.GetOrCreatePool(config1)
	if err != nil {
		t.Fatalf("Failed to create pool 1: %v", err)
	}

	if pool1.GetConfigID() != config1.ID {
		t.Errorf("Expected config ID %s, got %s", config1.ID, pool1.GetConfigID())
	}

	// Test getting existing pool
	pool1Again, err := manager.GetOrCreatePool(config1)
	if err != nil {
		t.Fatalf("Failed to get existing pool: %v", err)
	}

	if pool1 != pool1Again {
		t.Error("Expected to get the same pool instance")
	}

	// Test creating second pool
	pool2, err := manager.GetOrCreatePool(config2)
	if err != nil {
		t.Fatalf("Failed to create pool 2: %v", err)
	}

	if pool2.GetConfigID() != config2.ID {
		t.Errorf("Expected config ID %s, got %s", config2.ID, pool2.GetConfigID())
	}

	// Test GetPool
	retrievedPool, err := manager.GetPool(config1.ID)
	if err != nil {
		t.Fatalf("Failed to get pool: %v", err)
	}

	if retrievedPool != pool1 {
		t.Error("Expected to get the same pool instance")
	}

	// Test GetStats
	stats := manager.GetStats()
	if len(stats) != 2 {
		t.Errorf("Expected 2 pools in stats, got %d", len(stats))
	}

	// Test RemovePool
	manager.RemovePool(config1.ID)
	_, err = manager.GetPool(config1.ID)
	if err == nil {
		t.Error("Expected error when getting removed pool")
	}

	// Test CloseAll
	manager.CloseAll()
	stats = manager.GetStats()
	if len(stats) != 0 {
		t.Errorf("Expected 0 pools after CloseAll, got %d", len(stats))
	}
}

func TestConnectionPoolDo(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	config := &database.APIConfig{
		ID:             "test-config-1",
		Name:           "Test Config",
		OpenAIBaseURL:  server.URL,
		OpenAIAPIKey:   "test-key",
		Enabled:        true,
	}

	poolConfig := DefaultConnectionPoolConfig()
	pool, err := NewConnectionPool(config, poolConfig)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create a request
	req, err := http.NewRequest("GET", server.URL+"/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Execute request
	ctx := context.Background()
	resp, err := pool.Do(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	// Check stats
	stats := pool.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("Expected 1 total request, got %d", stats.TotalRequests)
	}

	if stats.FailedRequests != 0 {
		t.Errorf("Expected 0 failed requests, got %d", stats.FailedRequests)
	}
}

func TestConnectionPoolDoWithError(t *testing.T) {
	config := &database.APIConfig{
		ID:             "test-config-1",
		Name:           "Test Config",
		OpenAIBaseURL:  "http://invalid-url-that-does-not-exist.local",
		OpenAIAPIKey:   "test-key",
		Enabled:        true,
	}

	poolConfig := DefaultConnectionPoolConfig()
	poolConfig.ConnectionTimeout = 1 * time.Second
	pool, err := NewConnectionPool(config, poolConfig)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create a request
	req, err := http.NewRequest("GET", config.OpenAIBaseURL+"/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Execute request (should fail)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = pool.Do(ctx, req)
	if err == nil {
		t.Error("Expected error when connecting to invalid URL")
	}

	// Check stats
	stats := pool.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("Expected 1 total request, got %d", stats.TotalRequests)
	}

	if stats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", stats.FailedRequests)
	}
}

func TestConnectionPoolConcurrency(t *testing.T) {
	// Create a test server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	config := &database.APIConfig{
		ID:             "test-config-1",
		Name:           "Test Config",
		OpenAIBaseURL:  server.URL,
		OpenAIAPIKey:   "test-key",
		Enabled:        true,
	}

	poolConfig := DefaultConnectionPoolConfig()
	pool, err := NewConnectionPool(config, poolConfig)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Execute multiple concurrent requests
	numRequests := 10
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req, err := http.NewRequest("GET", server.URL+"/test", nil)
			if err != nil {
				t.Errorf("Failed to create request: %v", err)
				done <- false
				return
			}

			ctx := context.Background()
			resp, err := pool.Do(ctx, req)
			if err != nil {
				t.Errorf("Failed to execute request: %v", err)
				done <- false
				return
			}
			resp.Body.Close()

			done <- true
		}()
	}

	// Wait for all requests to complete
	successCount := 0
	for i := 0; i < numRequests; i++ {
		if <-done {
			successCount++
		}
	}

	if successCount != numRequests {
		t.Errorf("Expected %d successful requests, got %d", numRequests, successCount)
	}

	// Check stats
	stats := pool.GetStats()
	if stats.TotalRequests != int64(numRequests) {
		t.Errorf("Expected %d total requests, got %d", numRequests, stats.TotalRequests)
	}

	if stats.FailedRequests != 0 {
		t.Errorf("Expected 0 failed requests, got %d", stats.FailedRequests)
	}
}

func TestConnectionPoolCleanup(t *testing.T) {
	config := &database.APIConfig{
		ID:             "test-config-1",
		Name:           "Test Config",
		OpenAIBaseURL:  "https://api.openai.com",
		OpenAIAPIKey:   "test-key",
		Enabled:        true,
	}

	poolConfig := DefaultConnectionPoolConfig()
	pool, err := NewConnectionPool(config, poolConfig)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	// Test cleanup
	pool.CleanupIdleConnections()

	// Test close
	pool.Close()

	// Verify pool is closed (no panic should occur)
	pool.Close() // Should be safe to call multiple times
}

func TestDefaultConnectionPoolConfig(t *testing.T) {
	config := DefaultConnectionPoolConfig()

	if config.MaxConnectionsPerHost <= 0 {
		t.Error("MaxConnectionsPerHost should be positive")
	}

	if config.MaxIdleConnections <= 0 {
		t.Error("MaxIdleConnections should be positive")
	}

	if config.MaxIdleTime <= 0 {
		t.Error("MaxIdleTime should be positive")
	}

	if config.ConnectionTimeout <= 0 {
		t.Error("ConnectionTimeout should be positive")
	}

	if config.KeepAlive <= 0 {
		t.Error("KeepAlive should be positive")
	}
}
