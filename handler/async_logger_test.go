package handler

import (
	"context"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestAsyncLogger_StartStop(t *testing.T) {
	config := DefaultAsyncLoggerConfig()
	logger := NewAsyncLogger(config)

	ctx := context.Background()
	
	// Test start
	err := logger.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start async logger: %v", err)
	}

	// Test double start
	err = logger.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running logger")
	}

	// Test stop
	err = logger.Stop()
	if err != nil {
		t.Fatalf("Failed to stop async logger: %v", err)
	}

	// Test double stop
	err = logger.Stop()
	if err == nil {
		t.Error("Expected error when stopping already stopped logger")
	}
}

func TestAsyncLogger_Log(t *testing.T) {
	// Skip this test as it requires database
	t.Skip("Skipping test that requires database")
}

func TestAsyncLogger_BufferFlush(t *testing.T) {
	config := AsyncLoggerConfig{
		BufferSize:    5,
		FlushInterval: 1 * time.Second,
		ChannelSize:   100,
	}
	logger := NewAsyncLogger(config)

	// Don't start logger to test buffer accumulation
	
	// Add logs to buffer manually
	for i := 0; i < 3; i++ {
		log := &database.LoadBalancerRequestLog{
			LoadBalancerID:   "test-lb",
			SelectedConfigID: "test-config",
			RequestTime:      time.Now(),
			ResponseTime:     time.Now(),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
		}
		logger.addToBuffer(log)
	}

	// Check buffer size
	stats := logger.GetStats()
	if stats.BufferSize != 3 {
		t.Errorf("Expected buffer size 3, got %d", stats.BufferSize)
	}
}

func TestAsyncLogger_PeriodicFlush(t *testing.T) {
	// Skip this test as it requires database
	t.Skip("Skipping test that requires database")
}

func TestAsyncLogger_ChannelFull(t *testing.T) {
	config := AsyncLoggerConfig{
		BufferSize:    10,
		FlushInterval: 1 * time.Second,
		ChannelSize:   5, // Small channel
	}
	logger := NewAsyncLogger(config)

	// Don't start the logger so channel fills up
	
	// Try to log more than channel size
	for i := 0; i < 10; i++ {
		log := &database.LoadBalancerRequestLog{
			LoadBalancerID:   "test-lb",
			SelectedConfigID: "test-config",
			RequestTime:      time.Now(),
			ResponseTime:     time.Now(),
			DurationMs:       100,
			StatusCode:       200,
			Success:          true,
		}
		logger.Log(log)
	}

	// Check that some logs were dropped
	stats := logger.GetStats()
	if stats.DroppedLogs == 0 {
		t.Error("Expected some logs to be dropped when channel is full")
	}
}

func TestAsyncLogger_Stats(t *testing.T) {
	config := DefaultAsyncLoggerConfig()
	logger := NewAsyncLogger(config)

	stats := logger.GetStats()
	
	if stats.ProcessedLogs != 0 {
		t.Errorf("Expected 0 processed logs, got %d", stats.ProcessedLogs)
	}

	if stats.DroppedLogs != 0 {
		t.Errorf("Expected 0 dropped logs, got %d", stats.DroppedLogs)
	}

	if stats.BufferSize != 0 {
		t.Errorf("Expected 0 buffer size, got %d", stats.BufferSize)
	}

	if stats.ChannelSize != 0 {
		t.Errorf("Expected 0 channel size, got %d", stats.ChannelSize)
	}
}

func TestAsyncLogger_Concurrency(t *testing.T) {
	// Skip this test as it requires database
	t.Skip("Skipping test that requires database")
}

func TestDefaultAsyncLoggerConfig(t *testing.T) {
	config := DefaultAsyncLoggerConfig()

	if config.BufferSize <= 0 {
		t.Error("BufferSize should be positive")
	}

	if config.FlushInterval <= 0 {
		t.Error("FlushInterval should be positive")
	}

	if config.ChannelSize <= 0 {
		t.Error("ChannelSize should be positive")
	}
}
