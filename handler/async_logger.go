package handler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// AsyncLogger handles asynchronous batch logging
type AsyncLogger struct {
	buffer          []*database.LoadBalancerRequestLog
	bufferSize      int
	flushInterval   time.Duration
	logChan         chan *database.LoadBalancerRequestLog
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	running         bool
	droppedLogs     int64
	processedLogs   int64
}

// AsyncLoggerConfig holds configuration for async logger
type AsyncLoggerConfig struct {
	BufferSize    int
	FlushInterval time.Duration
	ChannelSize   int
}

// DefaultAsyncLoggerConfig returns default configuration
func DefaultAsyncLoggerConfig() AsyncLoggerConfig {
	return AsyncLoggerConfig{
		BufferSize:    100,              // Batch size
		FlushInterval: 5 * time.Second,  // Flush every 5 seconds
		ChannelSize:   10000,            // Channel buffer size
	}
}

// NewAsyncLogger creates a new async logger
func NewAsyncLogger(config AsyncLoggerConfig) *AsyncLogger {
	return &AsyncLogger{
		buffer:        make([]*database.LoadBalancerRequestLog, 0, config.BufferSize),
		bufferSize:    config.BufferSize,
		flushInterval: config.FlushInterval,
		logChan:       make(chan *database.LoadBalancerRequestLog, config.ChannelSize),
		stopChan:      make(chan struct{}),
	}
}

// Start starts the async logger
func (al *AsyncLogger) Start(ctx context.Context) error {
	al.mu.Lock()
	if al.running {
		al.mu.Unlock()
		return fmt.Errorf("async logger already running")
	}
	al.running = true
	al.mu.Unlock()

	// Start log processing goroutine
	al.wg.Add(1)
	go al.processLogs(ctx)

	logger := utils.GetLogger()
	logger.Debug("Async logger started")
	return nil
}

// Stop stops the async logger and flushes remaining logs
func (al *AsyncLogger) Stop() error {
	al.mu.Lock()
	if !al.running {
		al.mu.Unlock()
		return fmt.Errorf("async logger not running")
	}
	al.running = false
	al.mu.Unlock()

	close(al.stopChan)
	al.wg.Wait()

	// Flush any remaining logs
	al.flush()

	logger := utils.GetLogger()
	logger.Debug("Async logger stopped. Processed: %d, Dropped: %d", al.processedLogs, al.droppedLogs)
	return nil
}

// Log queues a log entry for async processing
func (al *AsyncLogger) Log(log *database.LoadBalancerRequestLog) {
	select {
	case al.logChan <- log:
		// Successfully queued
	default:
		// Channel full, drop log and increment counter
		al.mu.Lock()
		al.droppedLogs++
		al.mu.Unlock()
		
		logger := utils.GetLogger()
		logger.Warn("Async logger channel full, dropping log entry")
	}
}

// processLogs processes logs from the channel
func (al *AsyncLogger) processLogs(ctx context.Context) {
	defer al.wg.Done()

	ticker := time.NewTicker(al.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-al.stopChan:
			return
		case log := <-al.logChan:
			al.addToBuffer(log)
			
			// Flush if buffer is full
			if len(al.buffer) >= al.bufferSize {
				al.flush()
			}
		case <-ticker.C:
			// Periodic flush
			al.flush()
		}
	}
}

// addToBuffer adds a log entry to the buffer
func (al *AsyncLogger) addToBuffer(log *database.LoadBalancerRequestLog) {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	al.buffer = append(al.buffer, log)
}

// flush writes all buffered logs to the database
func (al *AsyncLogger) flush() {
	al.mu.Lock()
	if len(al.buffer) == 0 {
		al.mu.Unlock()
		return
	}
	
	// Copy buffer and reset
	logs := make([]*database.LoadBalancerRequestLog, len(al.buffer))
	copy(logs, al.buffer)
	al.buffer = al.buffer[:0]
	al.mu.Unlock()

	// Batch insert logs
	if err := database.BatchCreateLoadBalancerRequestLogs(logs); err != nil {
		logger := utils.GetLogger()
		logger.Error("Failed to batch insert logs: %v", err)
		
		// Fallback: try inserting one by one
		for _, log := range logs {
			if err := database.CreateLoadBalancerRequestLog(log); err != nil {
				logger.Error("Failed to insert log: %v", err)
			} else {
				al.mu.Lock()
				al.processedLogs++
				al.mu.Unlock()
			}
		}
	} else {
		al.mu.Lock()
		al.processedLogs += int64(len(logs))
		al.mu.Unlock()
	}
}

// GetStats returns logger statistics
func (al *AsyncLogger) GetStats() AsyncLoggerStats {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	return AsyncLoggerStats{
		ProcessedLogs: al.processedLogs,
		DroppedLogs:   al.droppedLogs,
		BufferSize:    len(al.buffer),
		ChannelSize:   len(al.logChan),
	}
}

// AsyncLoggerStats holds statistics for the async logger
type AsyncLoggerStats struct {
	ProcessedLogs int64
	DroppedLogs   int64
	BufferSize    int
	ChannelSize   int
}
