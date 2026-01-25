package handler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// AlertManager interface defines alert management operations
type AlertManager interface {
	CheckAndCreateAlerts(loadBalancerID string) error
	GetAlerts(loadBalancerID string, acknowledged bool) ([]*database.Alert, error)
	AcknowledgeAlert(alertID string) error
	Start(ctx context.Context) error
	Stop() error
}

// DefaultAlertManager implements the AlertManager interface
type DefaultAlertManager struct {
	loadBalancerID    string
	checkInterval     time.Duration
	errorRateWindow   int // minutes
	errorRateThreshold float64
	minHealthyNodes   int
	stopChan          chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
	running           bool
}

// AlertManagerConfig holds alert manager configuration
type AlertManagerConfig struct {
	CheckInterval      time.Duration
	ErrorRateWindow    int     // minutes
	ErrorRateThreshold float64 // 0.0-1.0
	MinHealthyNodes    int
}

// NewAlertManager creates a new alert manager instance
func NewAlertManager(loadBalancerID string, config AlertManagerConfig) AlertManager {
	return &DefaultAlertManager{
		loadBalancerID:     loadBalancerID,
		checkInterval:      config.CheckInterval,
		errorRateWindow:    config.ErrorRateWindow,
		errorRateThreshold: config.ErrorRateThreshold,
		minHealthyNodes:    config.MinHealthyNodes,
		stopChan:           make(chan struct{}),
	}
}

// Start starts the alert manager
func (am *DefaultAlertManager) Start(ctx context.Context) error {
	am.mu.Lock()
	if am.running {
		am.mu.Unlock()
		return fmt.Errorf("alert manager already running")
	}
	am.running = true
	am.mu.Unlock()

	// Start alert checking goroutine
	am.wg.Add(1)
	go am.runAlertChecks(ctx)

	log.Printf("Alert manager started for load balancer %s", am.loadBalancerID)
	return nil
}

// Stop stops the alert manager
func (am *DefaultAlertManager) Stop() error {
	am.mu.Lock()
	if !am.running {
		am.mu.Unlock()
		return fmt.Errorf("alert manager not running")
	}
	am.running = false
	am.mu.Unlock()

	close(am.stopChan)
	am.wg.Wait()

	log.Printf("Alert manager stopped for load balancer %s", am.loadBalancerID)
	return nil
}

// runAlertChecks runs periodic alert checks
func (am *DefaultAlertManager) runAlertChecks(ctx context.Context) {
	defer am.wg.Done()

	ticker := time.NewTicker(am.checkInterval)
	defer ticker.Stop()

	// Run initial check immediately
	am.CheckAndCreateAlerts(am.loadBalancerID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.CheckAndCreateAlerts(am.loadBalancerID)
		}
	}
}

// CheckAndCreateAlerts checks conditions and creates alerts if needed
func (am *DefaultAlertManager) CheckAndCreateAlerts(loadBalancerID string) error {
	// Check for all nodes down
	if err := database.CheckAndCreateAllNodesDownAlert(loadBalancerID); err != nil {
		log.Printf("Failed to check all nodes down alert: %v", err)
	}

	// Check for low healthy nodes
	if err := database.CheckAndCreateLowHealthyNodesAlert(loadBalancerID, am.minHealthyNodes); err != nil {
		log.Printf("Failed to check low healthy nodes alert: %v", err)
	}

	// Check for high error rate
	if err := database.CheckAndCreateHighErrorRateAlert(loadBalancerID, am.errorRateThreshold, am.errorRateWindow); err != nil {
		log.Printf("Failed to check high error rate alert: %v", err)
	}

	// Check for circuit breaker open states
	if err := am.checkCircuitBreakerAlerts(loadBalancerID); err != nil {
		log.Printf("Failed to check circuit breaker alerts: %v", err)
	}

	return nil
}

// checkCircuitBreakerAlerts checks for circuit breaker state changes
func (am *DefaultAlertManager) checkCircuitBreakerAlerts(loadBalancerID string) error {
	lb, err := database.GetLoadBalancer(loadBalancerID)
	if err != nil {
		return err
	}

	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}

		cbState, err := database.GetCircuitBreakerState(node.ConfigID)
		if err != nil {
			continue
		}

		if cbState.State == "open" {
			// Check if alert already exists
			acknowledged := false
			alerts, err := database.GetAlertsByLoadBalancer(loadBalancerID, &acknowledged, 10)
			if err == nil {
				alertExists := false
				for _, alert := range alerts {
					if alert.Type == "circuit_breaker_open" && alert.Details == node.ConfigID {
						alertExists = true
						break
					}
				}

				if !alertExists {
					// Create new alert
					config, _ := database.GetAPIConfig(node.ConfigID)
					configName := node.ConfigID
					if config != nil {
						configName = config.Name
					}

					alert := &database.Alert{
						LoadBalancerID: loadBalancerID,
						Level:          "info",
						Type:           "circuit_breaker_open",
						Message:        fmt.Sprintf("Circuit breaker opened for node: %s", configName),
						Details:        node.ConfigID,
					}
					if err := database.CreateAlert(alert); err != nil {
						log.Printf("Failed to create circuit breaker alert: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// GetAlerts retrieves alerts for a load balancer
func (am *DefaultAlertManager) GetAlerts(loadBalancerID string, acknowledged bool) ([]*database.Alert, error) {
	return database.GetAlertsByLoadBalancer(loadBalancerID, &acknowledged, 100)
}

// AcknowledgeAlert marks an alert as acknowledged
func (am *DefaultAlertManager) AcknowledgeAlert(alertID string) error {
	return database.AcknowledgeAlert(alertID)
}

// AlertManagerRegistry manages alert managers for multiple load balancers
type AlertManagerRegistry struct {
	managers map[string]AlertManager
	mu       sync.RWMutex
	config   AlertManagerConfig
}

// NewAlertManagerRegistry creates a new alert manager registry
func NewAlertManagerRegistry(config AlertManagerConfig) *AlertManagerRegistry {
	return &AlertManagerRegistry{
		managers: make(map[string]AlertManager),
		config:   config,
	}
}

// GetAlertManager gets or creates an alert manager for a load balancer
func (amr *AlertManagerRegistry) GetAlertManager(loadBalancerID string) AlertManager {
	amr.mu.RLock()
	manager, exists := amr.managers[loadBalancerID]
	amr.mu.RUnlock()

	if exists {
		return manager
	}

	amr.mu.Lock()
	defer amr.mu.Unlock()

	// Double-check after acquiring write lock
	if manager, exists := amr.managers[loadBalancerID]; exists {
		return manager
	}

	// Create new alert manager
	manager = NewAlertManager(loadBalancerID, amr.config)
	amr.managers[loadBalancerID] = manager

	return manager
}

// StartAll starts all alert managers
func (amr *AlertManagerRegistry) StartAll(ctx context.Context) error {
	amr.mu.RLock()
	defer amr.mu.RUnlock()

	for _, manager := range amr.managers {
		if err := manager.Start(ctx); err != nil {
			return err
		}
	}

	return nil
}

// StopAll stops all alert managers
func (amr *AlertManagerRegistry) StopAll() error {
	amr.mu.RLock()
	defer amr.mu.RUnlock()

	for _, manager := range amr.managers {
		if err := manager.Stop(); err != nil {
			return err
		}
	}

	return nil
}

// DefaultAlertManagerConfig returns default alert manager configuration
func DefaultAlertManagerConfig() AlertManagerConfig {
	return AlertManagerConfig{
		CheckInterval:      1 * time.Minute,
		ErrorRateWindow:    5,
		ErrorRateThreshold: 0.2,
		MinHealthyNodes:    1,
	}
}
