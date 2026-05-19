package handler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// StatsEvent represents a statistics event
type StatsEvent struct {
	LoadBalancerID   string
	SelectedConfigID string
	Success          bool
	DurationMs       int
	Timestamp        time.Time
}

// AsyncStatsCollector handles asynchronous statistics collection and aggregation
type AsyncStatsCollector struct {
	loadBalancerID    string
	eventChan         chan *StatsEvent
	stopChan          chan struct{}
	wg                sync.WaitGroup
	mu                sync.Mutex
	running           bool
	
	// In-memory aggregation
	currentBucket     time.Time
	bucketStats       map[string]*NodeBucketStats
	
	// Configuration
	aggregationInterval time.Duration
	channelSize         int
	
	// Statistics
	eventsProcessed   int64
	eventsDropped     int64
}

// NodeBucketStats holds statistics for a node in a time bucket
type NodeBucketStats struct {
	RequestCount    int
	SuccessCount    int
	FailedCount     int
	TotalDurationMs int64
}

// AsyncStatsCollectorConfig holds configuration for async stats collector
type AsyncStatsCollectorConfig struct {
	AggregationInterval time.Duration
	ChannelSize         int
}

// DefaultAsyncStatsCollectorConfig returns default configuration
func DefaultAsyncStatsCollectorConfig() AsyncStatsCollectorConfig {
	return AsyncStatsCollectorConfig{
		AggregationInterval: 1 * time.Minute,
		ChannelSize:         10000,
	}
}

// NewAsyncStatsCollector creates a new async stats collector
func NewAsyncStatsCollector(loadBalancerID string, config AsyncStatsCollectorConfig) *AsyncStatsCollector {
	return &AsyncStatsCollector{
		loadBalancerID:      loadBalancerID,
		eventChan:           make(chan *StatsEvent, config.ChannelSize),
		stopChan:            make(chan struct{}),
		currentBucket:       time.Now().Truncate(time.Minute),
		bucketStats:         make(map[string]*NodeBucketStats),
		aggregationInterval: config.AggregationInterval,
		channelSize:         config.ChannelSize,
	}
}

// Start starts the async stats collector
func (asc *AsyncStatsCollector) Start(ctx context.Context) error {
	asc.mu.Lock()
	if asc.running {
		asc.mu.Unlock()
		return fmt.Errorf("async stats collector already running")
	}
	asc.running = true
	asc.mu.Unlock()

	// Start event processing goroutine
	asc.wg.Add(1)
	go asc.processEvents(ctx)

	// Start aggregation goroutine
	asc.wg.Add(1)
	go asc.aggregateStats(ctx)

	logger := utils.GetLogger()
	logger.Debug("Async stats collector started for load balancer %s", asc.loadBalancerID)
	return nil
}

// Stop stops the async stats collector and flushes remaining stats
func (asc *AsyncStatsCollector) Stop() error {
	asc.mu.Lock()
	if !asc.running {
		asc.mu.Unlock()
		return fmt.Errorf("async stats collector not running")
	}
	asc.running = false
	asc.mu.Unlock()

	close(asc.stopChan)
	asc.wg.Wait()

	// Flush any remaining stats
	asc.flushStats()

	logger := utils.GetLogger()
	logger.Debug("Async stats collector stopped for load balancer %s. Processed: %d, Dropped: %d",
		asc.loadBalancerID, asc.eventsProcessed, asc.eventsDropped)
	return nil
}

// RecordEvent queues a statistics event for async processing
func (asc *AsyncStatsCollector) RecordEvent(event *StatsEvent) {
	select {
	case asc.eventChan <- event:
		// Successfully queued
	default:
		// Channel full, drop event and increment counter
		asc.mu.Lock()
		asc.eventsDropped++
		asc.mu.Unlock()
		
		logger := utils.GetLogger()
		logger.Warn("Async stats collector channel full, dropping event")
	}
}

// processEvents processes events from the channel
func (asc *AsyncStatsCollector) processEvents(ctx context.Context) {
	defer asc.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-asc.stopChan:
			return
		case event := <-asc.eventChan:
			asc.processEvent(event)
		}
	}
}

// processEvent processes a single event
func (asc *AsyncStatsCollector) processEvent(event *StatsEvent) {
	asc.mu.Lock()
	defer asc.mu.Unlock()

	// Check if we need to start a new bucket
	eventBucket := event.Timestamp.Truncate(time.Minute)
	if eventBucket.After(asc.currentBucket) {
		// Flush current bucket before starting new one
		asc.flushStatsLocked()
		asc.currentBucket = eventBucket
	}

	// Get or create node stats for this bucket
	nodeStats, exists := asc.bucketStats[event.SelectedConfigID]
	if !exists {
		nodeStats = &NodeBucketStats{}
		asc.bucketStats[event.SelectedConfigID] = nodeStats
	}

	// Update stats
	nodeStats.RequestCount++
	if event.Success {
		nodeStats.SuccessCount++
	} else {
		nodeStats.FailedCount++
	}
	nodeStats.TotalDurationMs += int64(event.DurationMs)

	asc.eventsProcessed++
}

// aggregateStats periodically flushes aggregated stats to database
func (asc *AsyncStatsCollector) aggregateStats(ctx context.Context) {
	defer asc.wg.Done()

	ticker := time.NewTicker(asc.aggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-asc.stopChan:
			return
		case <-ticker.C:
			asc.flushStats()
		}
	}
}

// flushStats flushes aggregated stats to database (with lock)
func (asc *AsyncStatsCollector) flushStats() {
	asc.mu.Lock()
	defer asc.mu.Unlock()
	asc.flushStatsLocked()
}

// flushStatsLocked flushes aggregated stats to database (without lock)
func (asc *AsyncStatsCollector) flushStatsLocked() {
	if len(asc.bucketStats) == 0 {
		return
	}

	logger := utils.GetLogger()
	
	// Check if database is initialized
	if database.DB == nil {
		logger.Debug("Database not initialized, skipping stats flush")
		// Clear bucket stats even if we can't write to DB
		asc.bucketStats = make(map[string]*NodeBucketStats)
		return
	}
	
	// Write stats to database
	for configID, stats := range asc.bucketStats {
		err := database.CreateNodeStats(
			asc.loadBalancerID,
			configID,
			asc.currentBucket,
			stats.RequestCount,
			stats.SuccessCount,
			stats.FailedCount,
			stats.TotalDurationMs,
		)
		
		if err != nil {
			logger.Error("Failed to create node stats: %v", err)
		}
	}

	// Clear bucket stats
	asc.bucketStats = make(map[string]*NodeBucketStats)
}

// GetStats returns collector statistics
func (asc *AsyncStatsCollector) GetStats() AsyncStatsCollectorStats {
	asc.mu.Lock()
	defer asc.mu.Unlock()
	
	return AsyncStatsCollectorStats{
		EventsProcessed: asc.eventsProcessed,
		EventsDropped:   asc.eventsDropped,
		ChannelSize:     len(asc.eventChan),
		BucketSize:      len(asc.bucketStats),
	}
}

// AsyncStatsCollectorStats holds statistics for the async stats collector
type AsyncStatsCollectorStats struct {
	EventsProcessed int64
	EventsDropped   int64
	ChannelSize     int
	BucketSize      int
}

// GetCurrentBucketStats returns current in-memory bucket stats (for testing)
func (asc *AsyncStatsCollector) GetCurrentBucketStats() map[string]*NodeBucketStats {
	asc.mu.Lock()
	defer asc.mu.Unlock()
	
	// Return a copy
	statsCopy := make(map[string]*NodeBucketStats)
	for k, v := range asc.bucketStats {
		statsCopy[k] = &NodeBucketStats{
			RequestCount:    v.RequestCount,
			SuccessCount:    v.SuccessCount,
			FailedCount:     v.FailedCount,
			TotalDurationMs: v.TotalDurationMs,
		}
	}
	
	return statsCopy
}
