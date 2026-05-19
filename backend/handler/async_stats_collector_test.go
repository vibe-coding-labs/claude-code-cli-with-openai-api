package handler

import (
	"context"
	"testing"
	"time"
)

func TestAsyncStatsCollector_StartStop(t *testing.T) {
	config := DefaultAsyncStatsCollectorConfig()
	collector := NewAsyncStatsCollector("test-lb", config)

	ctx := context.Background()
	
	// Test start
	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start async stats collector: %v", err)
	}

	// Test double start
	err = collector.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running collector")
	}

	// Test stop
	err = collector.Stop()
	if err != nil {
		t.Fatalf("Failed to stop async stats collector: %v", err)
	}

	// Test double stop
	err = collector.Stop()
	if err == nil {
		t.Error("Expected error when stopping already stopped collector")
	}
}

func TestAsyncStatsCollector_RecordEvent(t *testing.T) {
	config := AsyncStatsCollectorConfig{
		AggregationInterval: 1 * time.Second,
		ChannelSize:         100,
	}
	collector := NewAsyncStatsCollector("test-lb", config)

	ctx := context.Background()
	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start async stats collector: %v", err)
	}
	defer collector.Stop()

	// Record some events
	for i := 0; i < 5; i++ {
		event := &StatsEvent{
			LoadBalancerID:   "test-lb",
			SelectedConfigID: "config-1",
			Success:          true,
			DurationMs:       100,
			Timestamp:        time.Now(),
		}
		collector.RecordEvent(event)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check stats
	stats := collector.GetStats()
	if stats.EventsProcessed != 5 {
		t.Errorf("Expected 5 events processed, got %d", stats.EventsProcessed)
	}
}

func TestAsyncStatsCollector_Aggregation(t *testing.T) {
	config := AsyncStatsCollectorConfig{
		AggregationInterval: 1 * time.Second,
		ChannelSize:         100,
	}
	collector := NewAsyncStatsCollector("test-lb", config)

	ctx := context.Background()
	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start async stats collector: %v", err)
	}
	defer collector.Stop()

	// Record events for multiple configs
	now := time.Now()
	
	// Config 1: 3 successful requests
	for i := 0; i < 3; i++ {
		event := &StatsEvent{
			LoadBalancerID:   "test-lb",
			SelectedConfigID: "config-1",
			Success:          true,
			DurationMs:       100,
			Timestamp:        now,
		}
		collector.RecordEvent(event)
	}

	// Config 2: 2 successful, 1 failed
	for i := 0; i < 2; i++ {
		event := &StatsEvent{
			LoadBalancerID:   "test-lb",
			SelectedConfigID: "config-2",
			Success:          true,
			DurationMs:       150,
			Timestamp:        now,
		}
		collector.RecordEvent(event)
	}
	
	event := &StatsEvent{
		LoadBalancerID:   "test-lb",
		SelectedConfigID: "config-2",
		Success:          false,
		DurationMs:       200,
		Timestamp:        now,
	}
	collector.RecordEvent(event)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check bucket stats
	bucketStats := collector.GetCurrentBucketStats()
	
	if len(bucketStats) != 2 {
		t.Errorf("Expected 2 configs in bucket stats, got %d", len(bucketStats))
	}

	// Check config-1 stats
	if stats, exists := bucketStats["config-1"]; exists {
		if stats.RequestCount != 3 {
			t.Errorf("Expected 3 requests for config-1, got %d", stats.RequestCount)
		}
		if stats.SuccessCount != 3 {
			t.Errorf("Expected 3 successes for config-1, got %d", stats.SuccessCount)
		}
		if stats.FailedCount != 0 {
			t.Errorf("Expected 0 failures for config-1, got %d", stats.FailedCount)
		}
		if stats.TotalDurationMs != 300 {
			t.Errorf("Expected 300ms total duration for config-1, got %d", stats.TotalDurationMs)
		}
	} else {
		t.Error("Expected config-1 in bucket stats")
	}

	// Check config-2 stats
	if stats, exists := bucketStats["config-2"]; exists {
		if stats.RequestCount != 3 {
			t.Errorf("Expected 3 requests for config-2, got %d", stats.RequestCount)
		}
		if stats.SuccessCount != 2 {
			t.Errorf("Expected 2 successes for config-2, got %d", stats.SuccessCount)
		}
		if stats.FailedCount != 1 {
			t.Errorf("Expected 1 failure for config-2, got %d", stats.FailedCount)
		}
		if stats.TotalDurationMs != 500 {
			t.Errorf("Expected 500ms total duration for config-2, got %d", stats.TotalDurationMs)
		}
	} else {
		t.Error("Expected config-2 in bucket stats")
	}
}

func TestAsyncStatsCollector_BucketRollover(t *testing.T) {
	config := AsyncStatsCollectorConfig{
		AggregationInterval: 1 * time.Second,
		ChannelSize:         100,
	}
	collector := NewAsyncStatsCollector("test-lb", config)

	ctx := context.Background()
	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start async stats collector: %v", err)
	}
	defer collector.Stop()

	// Record event in current bucket
	now := time.Now()
	event1 := &StatsEvent{
		LoadBalancerID:   "test-lb",
		SelectedConfigID: "config-1",
		Success:          true,
		DurationMs:       100,
		Timestamp:        now,
	}
	collector.RecordEvent(event1)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check bucket has data
	bucketStats := collector.GetCurrentBucketStats()
	if len(bucketStats) != 1 {
		t.Errorf("Expected 1 config in bucket stats, got %d", len(bucketStats))
	}

	// Record event in next bucket (simulate time passing)
	nextBucket := now.Add(2 * time.Minute)
	event2 := &StatsEvent{
		LoadBalancerID:   "test-lb",
		SelectedConfigID: "config-1",
		Success:          true,
		DurationMs:       100,
		Timestamp:        nextBucket,
	}
	collector.RecordEvent(event2)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check bucket was reset (old data flushed)
	bucketStats = collector.GetCurrentBucketStats()
	if len(bucketStats) != 1 {
		t.Errorf("Expected 1 config in new bucket stats, got %d", len(bucketStats))
	}
	
	// New bucket should have only 1 request
	if stats, exists := bucketStats["config-1"]; exists {
		if stats.RequestCount != 1 {
			t.Errorf("Expected 1 request in new bucket, got %d", stats.RequestCount)
		}
	}
}

func TestAsyncStatsCollector_ChannelFull(t *testing.T) {
	config := AsyncStatsCollectorConfig{
		AggregationInterval: 1 * time.Second,
		ChannelSize:         5, // Small channel
	}
	collector := NewAsyncStatsCollector("test-lb", config)

	// Don't start the collector so channel fills up
	
	// Try to record more than channel size
	for i := 0; i < 10; i++ {
		event := &StatsEvent{
			LoadBalancerID:   "test-lb",
			SelectedConfigID: "config-1",
			Success:          true,
			DurationMs:       100,
			Timestamp:        time.Now(),
		}
		collector.RecordEvent(event)
	}

	// Check that some events were dropped
	stats := collector.GetStats()
	if stats.EventsDropped == 0 {
		t.Error("Expected some events to be dropped when channel is full")
	}
}

func TestAsyncStatsCollector_Stats(t *testing.T) {
	config := DefaultAsyncStatsCollectorConfig()
	collector := NewAsyncStatsCollector("test-lb", config)

	stats := collector.GetStats()
	
	if stats.EventsProcessed != 0 {
		t.Errorf("Expected 0 events processed, got %d", stats.EventsProcessed)
	}

	if stats.EventsDropped != 0 {
		t.Errorf("Expected 0 events dropped, got %d", stats.EventsDropped)
	}

	if stats.ChannelSize != 0 {
		t.Errorf("Expected 0 channel size, got %d", stats.ChannelSize)
	}

	if stats.BucketSize != 0 {
		t.Errorf("Expected 0 bucket size, got %d", stats.BucketSize)
	}
}

func TestAsyncStatsCollector_Concurrency(t *testing.T) {
	config := AsyncStatsCollectorConfig{
		AggregationInterval: 1 * time.Second,
		ChannelSize:         1000,
	}
	collector := NewAsyncStatsCollector("test-lb", config)

	ctx := context.Background()
	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start async stats collector: %v", err)
	}
	defer collector.Stop()

	// Concurrent event recording
	done := make(chan bool)
	numGoroutines := 10
	eventsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &StatsEvent{
					LoadBalancerID:   "test-lb",
					SelectedConfigID: "config-1",
					Success:          true,
					DurationMs:       100,
					Timestamp:        time.Now(),
				}
				collector.RecordEvent(event)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Check stats
	stats := collector.GetStats()
	if stats.EventsProcessed != int64(numGoroutines*eventsPerGoroutine) {
		t.Errorf("Expected %d events processed, got %d", numGoroutines*eventsPerGoroutine, stats.EventsProcessed)
	}
}

func TestDefaultAsyncStatsCollectorConfig(t *testing.T) {
	config := DefaultAsyncStatsCollectorConfig()

	if config.AggregationInterval <= 0 {
		t.Error("AggregationInterval should be positive")
	}

	if config.ChannelSize <= 0 {
		t.Error("ChannelSize should be positive")
	}
}
