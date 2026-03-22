package handler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// Monitor interface defines monitoring operations
type Monitor interface {
	RecordRequest(log *database.LoadBalancerRequestLog)
	GetStats(loadBalancerID string, timeWindow string) (*database.LoadBalancerStats, error)
	GetRealTimeMetrics(loadBalancerID string) (*database.RealTimeMetrics, error)
	Start(ctx context.Context) error
	Stop() error
}

// DefaultMonitor implements the Monitor interface
type DefaultMonitor struct {
	loadBalancerID string
	logChan        chan *database.LoadBalancerRequestLog
	stopChan       chan struct{}
	wg             sync.WaitGroup
	mu             sync.RWMutex
	running        bool
}

// NewMonitor creates a new monitor instance
func NewMonitor(loadBalancerID string) Monitor {
	return &DefaultMonitor{
		loadBalancerID: loadBalancerID,
		logChan:        make(chan *database.LoadBalancerRequestLog, 1000),
		stopChan:       make(chan struct{}),
	}
}

// Start starts the monitor
func (m *DefaultMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("monitor already running")
	}
	m.running = true
	m.mu.Unlock()

	// Start log processing goroutine
	m.wg.Add(1)
	go m.processLogs(ctx)

	// Start stats aggregation goroutine
	m.wg.Add(1)
	go m.aggregateStats(ctx)

	// Start cleanup goroutine
	m.wg.Add(1)
	go m.cleanupOldData(ctx)

	log.Printf("Monitor started for load balancer %s", m.loadBalancerID)
	return nil
}

// Stop stops the monitor
func (m *DefaultMonitor) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return fmt.Errorf("monitor not running")
	}
	m.running = false
	m.mu.Unlock()

	close(m.stopChan)
	m.wg.Wait()

	log.Printf("Monitor stopped for load balancer %s", m.loadBalancerID)
	return nil
}

// RecordRequest records a request log asynchronously
func (m *DefaultMonitor) RecordRequest(log *database.LoadBalancerRequestLog) {
	// 如果请求日志记录被禁用，直接返回
	if config.GlobalConfig != nil && !config.GlobalConfig.EnableRequestLogging {
		return
	}

	select {
	case m.logChan <- log:
		// Successfully queued
	default:
		// Channel full, log warning
		fmt.Printf("Warning: log channel full, dropping request log\n")
	}
}

// processLogs processes request logs from the channel
func (m *DefaultMonitor) processLogs(ctx context.Context) {
	defer m.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case log := <-m.logChan:
			if err := database.CreateLoadBalancerRequestLog(log); err != nil {
				fmt.Printf("Failed to save request log: %v\n", err)
			}
		}
	}
}

// aggregateStats periodically aggregates statistics
func (m *DefaultMonitor) aggregateStats(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.performAggregation()
		}
	}
}

// performAggregation performs stats aggregation for the current time bucket
func (m *DefaultMonitor) performAggregation() {
	timeBucket := time.Now().Truncate(time.Minute)

	if err := database.AggregateStatsForTimeBucket(m.loadBalancerID, timeBucket); err != nil {
		log.Printf("Failed to aggregate stats: %v", err)
	}
}

// cleanupOldData periodically cleans up old logs and stats
func (m *DefaultMonitor) cleanupOldData(ctx context.Context) {
	defer m.wg.Done()

	// Run cleanup once per day at 2 AM
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Calculate time until next 2 AM
	now := time.Now()
	next2AM := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	if now.After(next2AM) {
		next2AM = next2AM.Add(24 * time.Hour)
	}
	initialDelay := time.Until(next2AM)

	// Wait until 2 AM for first cleanup
	select {
	case <-ctx.Done():
		return
	case <-m.stopChan:
		return
	case <-time.After(initialDelay):
		m.performCleanup()
	}

	// Then run cleanup every 24 hours
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.performCleanup()
		}
	}
}

// performCleanup performs the actual cleanup
func (m *DefaultMonitor) performCleanup() {
	log.Printf("Starting cleanup of old data for load balancer %s", m.loadBalancerID)
	
	if err := CleanupOldData(); err != nil {
		log.Printf("Failed to cleanup old data: %v", err)
	} else {
		log.Printf("Successfully cleaned up old data for load balancer %s", m.loadBalancerID)
	}
}

// GetStats retrieves statistics for a load balancer
func (m *DefaultMonitor) GetStats(loadBalancerID string, timeWindow string) (*database.LoadBalancerStats, error) {
	return database.GetLoadBalancerStats(loadBalancerID, timeWindow)
}

// GetRealTimeMetrics retrieves real-time metrics for a load balancer
func (m *DefaultMonitor) GetRealTimeMetrics(loadBalancerID string) (*database.RealTimeMetrics, error) {
	return database.GetRealTimeMetrics(loadBalancerID)
}

// MonitorManager manages monitors for multiple load balancers
type MonitorManager struct {
	monitors map[string]Monitor
	mu       sync.RWMutex
}

// NewMonitorManager creates a new monitor manager
func NewMonitorManager() *MonitorManager {
	return &MonitorManager{
		monitors: make(map[string]Monitor),
	}
}

// GetMonitor gets or creates a monitor for a load balancer
func (mm *MonitorManager) GetMonitor(loadBalancerID string) Monitor {
	mm.mu.RLock()
	monitor, exists := mm.monitors[loadBalancerID]
	mm.mu.RUnlock()

	if exists {
		return monitor
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Double-check after acquiring write lock
	if monitor, exists := mm.monitors[loadBalancerID]; exists {
		return monitor
	}

	// Create new monitor
	monitor = NewMonitor(loadBalancerID)
	mm.monitors[loadBalancerID] = monitor

	return monitor
}

// StartAll starts all monitors
func (mm *MonitorManager) StartAll(ctx context.Context) error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	for _, monitor := range mm.monitors {
		if err := monitor.Start(ctx); err != nil {
			return err
		}
	}

	return nil
}

// StopAll stops all monitors
func (mm *MonitorManager) StopAll() error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	for _, monitor := range mm.monitors {
		if err := monitor.Stop(); err != nil {
			return err
		}
	}

	return nil
}

// CleanupOldData cleans up old logs and stats
func CleanupOldData() error {
	// Delete request logs older than 30 days
	if err := database.DeleteOldLoadBalancerRequestLogs(30); err != nil {
		return fmt.Errorf("failed to delete old request logs: %w", err)
	}

	// Delete stats older than 90 days
	if err := database.DeleteOldStats(90); err != nil {
		return fmt.Errorf("failed to delete old stats: %w", err)
	}

	// Delete alerts older than 90 days
	if err := database.DeleteOldAlerts(90); err != nil {
		return fmt.Errorf("failed to delete old alerts: %w", err)
	}

	return nil
}
