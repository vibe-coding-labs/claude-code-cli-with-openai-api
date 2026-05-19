package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// HealthChecker interface defines health checking operations
type HealthChecker interface {
	Start(ctx context.Context) error
	Stop() error
	CheckNode(ctx context.Context, configID string) (*database.HealthStatus, error)
	GetHealthStatus(configID string) (*database.HealthStatus, error)
	GetAllHealthStatuses() ([]*database.HealthStatus, error)
}

// DefaultHealthChecker implements the HealthChecker interface
type DefaultHealthChecker struct {
	loadBalancerID    string
	interval          time.Duration
	timeout           time.Duration
	failureThreshold  int
	recoveryThreshold int
	stopChan          chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
	running           bool
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(loadBalancerID string, interval, timeout time.Duration, failureThreshold, recoveryThreshold int) HealthChecker {
	return &DefaultHealthChecker{
		loadBalancerID:    loadBalancerID,
		interval:          interval,
		timeout:           timeout,
		failureThreshold:  failureThreshold,
		recoveryThreshold: recoveryThreshold,
		stopChan:          make(chan struct{}),
	}
}

// Start starts the health checker
func (hc *DefaultHealthChecker) Start(ctx context.Context) error {
	hc.mu.Lock()
	if hc.running {
		hc.mu.Unlock()
		return fmt.Errorf("health checker already running")
	}
	hc.running = true
	hc.mu.Unlock()

	// Get load balancer configuration
	lb, err := database.GetLoadBalancer(hc.loadBalancerID)
	if err != nil {
		return fmt.Errorf("failed to get load balancer: %w", err)
	}

	// Start health check goroutine
	hc.wg.Add(1)
	go hc.runHealthChecks(ctx, lb)

	log.Printf("Health checker started for load balancer %s (interval: %v)", hc.loadBalancerID, hc.interval)
	return nil
}

// Stop stops the health checker
func (hc *DefaultHealthChecker) Stop() error {
	hc.mu.Lock()
	if !hc.running {
		hc.mu.Unlock()
		return fmt.Errorf("health checker not running")
	}
	hc.running = false
	hc.mu.Unlock()

	close(hc.stopChan)
	hc.wg.Wait()

	log.Printf("Health checker stopped for load balancer %s", hc.loadBalancerID)
	return nil
}

// runHealthChecks runs periodic health checks
func (hc *DefaultHealthChecker) runHealthChecks(ctx context.Context, lb *database.LoadBalancer) {
	defer hc.wg.Done()

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	// Run initial check immediately
	hc.checkAllNodes(ctx, lb)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hc.stopChan:
			return
		case <-ticker.C:
			hc.checkAllNodes(ctx, lb)
		}
	}
}

// checkAllNodes checks health of all nodes in the load balancer
func (hc *DefaultHealthChecker) checkAllNodes(ctx context.Context, lb *database.LoadBalancer) {
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}

		// Check node health in a separate goroutine to avoid blocking
		go func(configID string) {
			_, err := hc.CheckNode(ctx, configID)
			if err != nil {
				log.Printf("Health check failed for config %s: %v", configID, err)
			}
		}(node.ConfigID)
	}
}

// CheckNode performs a health check on a single node
func (hc *DefaultHealthChecker) CheckNode(ctx context.Context, configID string) (*database.HealthStatus, error) {
	startTime := time.Now()

	// Get config
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// Create a timeout context for the health check
	checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	// Perform health check by sending a simple request
	err = hc.performHealthCheck(checkCtx, config)
	responseTime := time.Since(startTime)

	// Get current health status
	currentStatus, err := database.GetHealthStatus(configID)
	if err != nil {
		// Initialize if doesn't exist
		currentStatus = &database.HealthStatus{
			ConfigID:             configID,
			Status:               "unknown",
			LastCheckTime:        time.Now(),
			ConsecutiveSuccesses: 0,
			ConsecutiveFailures:  0,
		}
	}

	// Update health status based on check result
	if err == nil {
		// Success
		currentStatus.ConsecutiveSuccesses++
		currentStatus.ConsecutiveFailures = 0
		currentStatus.ResponseTimeMs = int(responseTime.Milliseconds())
		currentStatus.LastError = ""

		// Transition to healthy if recovery threshold reached
		if currentStatus.Status != "healthy" && currentStatus.ConsecutiveSuccesses >= hc.recoveryThreshold {
			currentStatus.Status = "healthy"
			log.Printf("Node %s transitioned to healthy (consecutive successes: %d)", configID, currentStatus.ConsecutiveSuccesses)
		} else if currentStatus.Status == "unknown" {
			currentStatus.Status = "healthy"
		}
	} else {
		// Failure
		currentStatus.ConsecutiveFailures++
		currentStatus.ConsecutiveSuccesses = 0
		currentStatus.LastError = err.Error()

		// Transition to unhealthy if failure threshold reached
		if currentStatus.Status != "unhealthy" && currentStatus.ConsecutiveFailures >= hc.failureThreshold {
			currentStatus.Status = "unhealthy"
			log.Printf("Node %s transitioned to unhealthy (consecutive failures: %d)", configID, currentStatus.ConsecutiveFailures)
		} else if currentStatus.Status == "unknown" {
			currentStatus.Status = "unhealthy"
		}
	}

	currentStatus.LastCheckTime = time.Now()

	// Save updated status
	if err := database.CreateOrUpdateHealthStatus(currentStatus); err != nil {
		return nil, fmt.Errorf("failed to update health status: %w", err)
	}

	return currentStatus, nil
}

// performHealthCheck performs the actual health check request
func (hc *DefaultHealthChecker) performHealthCheck(ctx context.Context, config *database.APIConfig) error {
	// Create a simple HTTP request to check if the endpoint is reachable
	// We'll use the base URL and send a HEAD request
	req, err := http.NewRequestWithContext(ctx, "HEAD", config.OpenAIBaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set authorization header
	req.Header.Set("Authorization", "Bearer "+config.OpenAIAPIKey)

	// Send request
	client := &http.Client{
		Timeout: hc.timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	// We consider 2xx, 3xx, 401, and 404 as "healthy" because they indicate the service is responding
	// 5xx errors indicate service issues
	if resp.StatusCode >= 500 {
		return fmt.Errorf("service error: status code %d", resp.StatusCode)
	}

	return nil
}

// GetHealthStatus retrieves the health status for a node
func (hc *DefaultHealthChecker) GetHealthStatus(configID string) (*database.HealthStatus, error) {
	return database.GetHealthStatus(configID)
}

// GetAllHealthStatuses retrieves all health statuses
func (hc *DefaultHealthChecker) GetAllHealthStatuses() ([]*database.HealthStatus, error) {
	return database.GetAllHealthStatuses()
}
