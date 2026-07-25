package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
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

// CleanupOldData cleans up old logs and stats based on retention settings.
// It performs four phases:
//  1. Time-based retention cleanup (delete old logs, proxy errors, body files)
//  2. Size-based quota enforcement (delete oldest logs if DB exceeds max_db_size_gb)
//  3. VACUUM INTO to reclaim disk space (if free pages > 10% of total)
//  4. Incremental migration of inline bodies to file storage
func CleanupOldData() error {
	// --- 1. Time-based retention cleanup ---

	// Get log retention setting, default 30 days
	retentionDays := 30
	if daysStr, err := database.GetSetting("log_retention_days"); err == nil {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			retentionDays = days
		}
	}

	// Delete old request logs based on retention policy
	deleted, err := database.DeleteOldRequestLogs(retentionDays)
	if err != nil {
		fmt.Printf("Warning: failed to delete old request logs: %v\n", err)
	} else if deleted > 0 {
		fmt.Printf("Auto-cleaned %d request logs older than %d days\n", deleted, retentionDays)
	}

	// Delete old proxy errors
	proxyErrorRetentionDays := 30
	if daysStr, err := database.GetSetting("proxy_error_retention_days"); err == nil {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			proxyErrorRetentionDays = days
		}
	}
	if _, err := database.CleanupOldProxyErrors(time.Duration(proxyErrorRetentionDays) * 24 * time.Hour); err != nil {
		fmt.Printf("Warning: failed to cleanup old proxy errors: %v\n", err)
	}

	// Delete load balancer request logs older than 30 days
	if err := database.DeleteOldLoadBalancerRequestLogs(30); err != nil {
		fmt.Printf("Warning: failed to delete old LB request logs: %v\n", err)
	}

	// Delete stats older than 90 days
	if err := database.DeleteOldStats(90); err != nil {
		fmt.Printf("Warning: failed to delete old stats: %v\n", err)
	}

	// Delete alerts older than 90 days
	if err := database.DeleteOldAlerts(90); err != nil {
		fmt.Printf("Warning: failed to delete old alerts: %v\n", err)
	}

	// Clean old body files
	storage := database.GetLogStorage()
	if dirsRemoved, err := storage.CleanOldDirectories(retentionDays); err == nil && dirsRemoved > 0 {
		fmt.Printf("Auto-cleaned %d old body file directories\n", dirsRemoved)
	}

	// --- 2. Size-based quota enforcement ---

	maxDBSizeGB := 10 // default 10 GB
	if sizeStr, err := database.GetSetting("max_db_size_gb"); err == nil {
		if size, err := strconv.Atoi(sizeStr); err == nil && size > 0 {
			maxDBSizeGB = size
		}
	}

	sizeInfo, err := database.GetDatabaseSizeInfo()
	if err == nil {
		maxBytes := int64(maxDBSizeGB) * 1024 * 1024 * 1024
		if sizeInfo.TotalBytes > maxBytes {
			// Delete oldest logs in batches until under quota
			overBy := sizeInfo.TotalBytes - maxBytes
			// Estimate: each log row averages ~5KB (metadata only, bodies in files)
			estimatedRows := overBy / 5120
			if estimatedRows < 100 {
				estimatedRows = 100
			}
			if estimatedRows > 10000 {
				estimatedRows = 10000 // cap batch size
			}

			deletedRows, err := database.DeleteOldestRequestLogs(int(estimatedRows))
			if err != nil {
				fmt.Printf("Warning: failed to delete oldest logs for quota: %v\n", err)
			} else if deletedRows > 0 {
				fmt.Printf("Quota enforcement: deleted %d oldest logs (DB was %.1f GB, limit %d GB)\n",
					deletedRows, float64(sizeInfo.TotalBytes)/1e9, maxDBSizeGB)
			}
		}
	}

	// --- 3. VACUUM INTO to reclaim disk space ---
	// Only run if there are significant free pages (>10% of total)
	if sizeInfo != nil && sizeInfo.TotalPages > 0 {
		freeRatio := float64(sizeInfo.FreePages) / float64(sizeInfo.TotalPages)
		if freeRatio > 0.10 {
			if sizeInfo.DBPath != "" {
				if newSize, err := database.VacuumInto(sizeInfo.DBPath); err != nil {
					fmt.Printf("Warning: VACUUM INTO failed: %v\n", err)
				} else {
					fmt.Printf("VACUUM INTO completed: new size %.1f MB\n", float64(newSize)/1e6)
				}
			}
		}
	}

	// --- 4. Migrate inline bodies to files (incremental) ---
	if migrated, err := database.MigrateInlineBodiesToFiles(100); err == nil && migrated > 0 {
		fmt.Printf("Migrated %d inline bodies to file storage\n", migrated)
	}

	return nil
}
