package handler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// LoadBalancerManager manages all components for a load balancer
type LoadBalancerManager struct {
	loadBalancerID    string
	healthChecker     HealthChecker
	selector          Selector
	retryHandler      RetryHandler
	monitor           Monitor
	alertManager      AlertManager
	circuitBreakerMgr *CircuitBreakerManager
	connectionPoolMgr *ConnectionPoolManager
	cacheManager      *CacheManager
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.RWMutex
	running           bool
}

// LoadBalancerManagerConfig holds configuration for the load balancer manager
type LoadBalancerManagerConfig struct {
	// Health check configuration
	HealthCheckEnabled  bool
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	FailureThreshold    int
	RecoveryThreshold   int

	// Retry configuration
	MaxRetries        int
	InitialRetryDelay time.Duration
	MaxRetryDelay     time.Duration

	// Circuit breaker configuration
	CircuitBreakerEnabled bool
	ErrorRateThreshold    float64
	CircuitBreakerWindow  time.Duration
	CircuitBreakerTimeout time.Duration
	HalfOpenRequests      int

	// Alert configuration
	AlertCheckInterval time.Duration
	ErrorRateWindow    int
	MinHealthyNodes    int
}

// NewLoadBalancerManager creates a new load balancer manager
func NewLoadBalancerManager(loadBalancerID string, config LoadBalancerManagerConfig) (*LoadBalancerManager, error) {
	// Get load balancer
	lb, err := database.GetLoadBalancer(loadBalancerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create circuit breaker manager
	cbMgr := NewCircuitBreakerManager(CircuitBreakerConfig{
		ErrorRateThreshold: config.ErrorRateThreshold,
		WindowDuration:     config.CircuitBreakerWindow,
		Timeout:            config.CircuitBreakerTimeout,
		HalfOpenRequests:   config.HalfOpenRequests,
	})

	// Create selector
	selector, err := NewEnhancedSelector(lb, cbMgr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create selector: %w", err)
	}

	// Create retry handler
	retryHandler := NewRetryHandler(
		config.MaxRetries,
		config.InitialRetryDelay,
		config.MaxRetryDelay,
		selector,
		cbMgr,
	)

	// Create health checker
	healthChecker := NewHealthChecker(
		loadBalancerID,
		config.HealthCheckInterval,
		config.HealthCheckTimeout,
		config.FailureThreshold,
		config.RecoveryThreshold,
	)

	// Create monitor
	monitor := NewMonitor(loadBalancerID)

	// Create alert manager
	alertManager := NewAlertManager(loadBalancerID, AlertManagerConfig{
		CheckInterval:      config.AlertCheckInterval,
		ErrorRateWindow:    config.ErrorRateWindow,
		ErrorRateThreshold: config.ErrorRateThreshold,
		MinHealthyNodes:    config.MinHealthyNodes,
	})

	// Create connection pool manager
	connectionPoolMgr := NewConnectionPoolManager()

	// Create cache manager
	cacheManager := NewCacheManager(5 * time.Minute) // Cleanup every 5 minutes

	return &LoadBalancerManager{
		loadBalancerID:    loadBalancerID,
		healthChecker:     healthChecker,
		selector:          selector,
		retryHandler:      retryHandler,
		monitor:           monitor,
		alertManager:      alertManager,
		circuitBreakerMgr: cbMgr,
		connectionPoolMgr: connectionPoolMgr,
		cacheManager:      cacheManager,
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// Start starts all components
func (lbm *LoadBalancerManager) Start() error {
	lbm.mu.Lock()
	if lbm.running {
		lbm.mu.Unlock()
		return fmt.Errorf("load balancer manager already running")
	}
	lbm.running = true
	lbm.mu.Unlock()

	// Start cache cleanup
	if lbm.cacheManager != nil {
		lbm.cacheManager.StartCleanup()
	}

	// Start health checker
	if err := lbm.healthChecker.Start(lbm.ctx); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	// Start monitor
	if err := lbm.monitor.Start(lbm.ctx); err != nil {
		return fmt.Errorf("failed to start monitor: %w", err)
	}

	// Start alert manager
	if err := lbm.alertManager.Start(lbm.ctx); err != nil {
		return fmt.Errorf("failed to start alert manager: %w", err)
	}

	log.Printf("Load balancer manager started for %s", lbm.loadBalancerID)
	return nil
}

// Stop stops all components
func (lbm *LoadBalancerManager) Stop() error {
	lbm.mu.Lock()
	if !lbm.running {
		lbm.mu.Unlock()
		return fmt.Errorf("load balancer manager not running")
	}
	lbm.running = false
	lbm.mu.Unlock()

	// Cancel context
	lbm.cancel()

	// Stop all components
	if err := lbm.healthChecker.Stop(); err != nil {
		log.Printf("Error stopping health checker: %v", err)
	}

	if err := lbm.monitor.Stop(); err != nil {
		log.Printf("Error stopping monitor: %v", err)
	}

	if err := lbm.alertManager.Stop(); err != nil {
		log.Printf("Error stopping alert manager: %v", err)
	}

	// Close all connection pools
	if lbm.connectionPoolMgr != nil {
		lbm.connectionPoolMgr.CloseAll()
	}

	// Stop cache cleanup
	if lbm.cacheManager != nil {
		lbm.cacheManager.StopCleanup()
	}

	log.Printf("Load balancer manager stopped for %s", lbm.loadBalancerID)
	return nil
}

// ExecuteRequest executes a request with retry and monitoring
func (lbm *LoadBalancerManager) ExecuteRequest(fn func(config *database.APIConfig) error) error {
	startTime := time.Now()
	var selectedConfigID string
	var retryCount int
	var lastErr error

	// Execute with retry
	err := lbm.retryHandler.ExecuteWithRetry(lbm.ctx, func(config *database.APIConfig) error {
		selectedConfigID = config.ID
		retryCount++
		return fn(config)
	})

	// Record request
	requestLog := &database.LoadBalancerRequestLog{
		LoadBalancerID:   lbm.loadBalancerID,
		SelectedConfigID: selectedConfigID,
		RequestTime:      startTime,
		ResponseTime:     time.Now(),
		DurationMs:       int(time.Since(startTime).Milliseconds()),
		Success:          err == nil,
		RetryCount:       retryCount - 1,
	}

	if err != nil {
		requestLog.ErrorMessage = err.Error()
		lastErr = err
	}

	lbm.monitor.RecordRequest(requestLog)

	return lastErr
}

// GetSelector returns the selector
func (lbm *LoadBalancerManager) GetSelector() Selector {
	return lbm.selector
}

// GetHealthChecker returns the health checker
func (lbm *LoadBalancerManager) GetHealthChecker() HealthChecker {
	return lbm.healthChecker
}

// GetMonitor returns the monitor
func (lbm *LoadBalancerManager) GetMonitor() Monitor {
	return lbm.monitor
}

// GetAlertManager returns the alert manager
func (lbm *LoadBalancerManager) GetAlertManager() AlertManager {
	return lbm.alertManager
}

// GetConnectionPool returns the connection pool for a config
func (lbm *LoadBalancerManager) GetConnectionPool(config *database.APIConfig) (*ConnectionPool, error) {
	if lbm.connectionPoolMgr == nil {
		return nil, fmt.Errorf("connection pool manager not initialized")
	}
	return lbm.connectionPoolMgr.GetOrCreatePool(config)
}

// GetConnectionPoolManager returns the connection pool manager
func (lbm *LoadBalancerManager) GetConnectionPoolManager() *ConnectionPoolManager {
	return lbm.connectionPoolMgr
}

// GetCacheManager returns the cache manager
func (lbm *LoadBalancerManager) GetCacheManager() *CacheManager {
	return lbm.cacheManager
}

// LoadBalancerManagerRegistry manages load balancer managers
type LoadBalancerManagerRegistry struct {
	managers map[string]*LoadBalancerManager
	mu       sync.RWMutex
	config   LoadBalancerManagerConfig
}

// NewLoadBalancerManagerRegistry creates a new registry
func NewLoadBalancerManagerRegistry(config LoadBalancerManagerConfig) *LoadBalancerManagerRegistry {
	return &LoadBalancerManagerRegistry{
		managers: make(map[string]*LoadBalancerManager),
		config:   config,
	}
}

// GetManager gets or creates a manager for a load balancer
func (lbmr *LoadBalancerManagerRegistry) GetManager(loadBalancerID string) (*LoadBalancerManager, error) {
	lbmr.mu.RLock()
	manager, exists := lbmr.managers[loadBalancerID]
	lbmr.mu.RUnlock()

	if exists {
		return manager, nil
	}

	lbmr.mu.Lock()
	defer lbmr.mu.Unlock()

	// Double-check after acquiring write lock
	if manager, exists := lbmr.managers[loadBalancerID]; exists {
		return manager, nil
	}

	// Create new manager
	manager, err := NewLoadBalancerManager(loadBalancerID, lbmr.config)
	if err != nil {
		return nil, err
	}

	// Start the manager
	if err := manager.Start(); err != nil {
		return nil, err
	}

	lbmr.managers[loadBalancerID] = manager
	return manager, nil
}

// StopAll stops all managers
func (lbmr *LoadBalancerManagerRegistry) StopAll() error {
	lbmr.mu.Lock()
	defer lbmr.mu.Unlock()

	for _, manager := range lbmr.managers {
		if err := manager.Stop(); err != nil {
			log.Printf("Error stopping manager: %v", err)
		}
	}

	return nil
}

// DefaultLoadBalancerManagerConfig returns default configuration
func DefaultLoadBalancerManagerConfig() LoadBalancerManagerConfig {
	return LoadBalancerManagerConfig{
		HealthCheckEnabled:    true,
		HealthCheckInterval:   30 * time.Second,
		HealthCheckTimeout:    5 * time.Second,
		FailureThreshold:      3,
		RecoveryThreshold:     2,
		MaxRetries:            20,                // 最多重试 20 次
		InitialRetryDelay:     1 * time.Second,   // 基础退避 1 秒
		MaxRetryDelay:         60 * time.Second,  // 最大退避 1 分钟
		CircuitBreakerEnabled: true,
		ErrorRateThreshold:    0.5,
		CircuitBreakerWindow:  60 * time.Second,
		CircuitBreakerTimeout: 30 * time.Second,
		HalfOpenRequests:      3,
		AlertCheckInterval:    1 * time.Minute,
		ErrorRateWindow:       5,
		MinHealthyNodes:       1,
	}
}
