package handler

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// Selector interface defines config selection operations
type Selector interface {
	SelectConfig(ctx context.Context) (*database.APIConfig, error)
	ReleaseConnection(configID string)
	GetAvailableNodes() ([]*database.APIConfig, error)
	RefreshNodes() error
}

// EnhancedSelector implements the Selector interface with health checking and circuit breaker support
type EnhancedSelector struct {
	loadBalancer      *database.LoadBalancer
	configs           []*database.APIConfig
	mu                sync.RWMutex
	roundRobinIndex   int
	connectionCounts  map[string]int
	dynamicWeights    map[string]int // Dynamic weights based on performance
	rng               *rand.Rand
	circuitBreakerMgr *CircuitBreakerManager
}

// NewEnhancedSelector creates a new enhanced selector
func NewEnhancedSelector(lb *database.LoadBalancer, cbMgr *CircuitBreakerManager) (*EnhancedSelector, error) {
	selector := &EnhancedSelector{
		loadBalancer:      lb,
		configs:           make([]*database.APIConfig, 0),
		roundRobinIndex:   0,
		connectionCounts:  make(map[string]int),
		dynamicWeights:    make(map[string]int),
		rng:               rand.New(rand.NewSource(time.Now().UnixNano())),
		circuitBreakerMgr: cbMgr,
	}

	// Load initial configs
	if err := selector.RefreshNodes(); err != nil {
		return nil, err
	}

	return selector, nil
}

// SelectConfig selects a healthy config based on the load balancing strategy
func (s *EnhancedSelector) SelectConfig(ctx context.Context) (*database.APIConfig, error) {
	// Get available (healthy) nodes
	availableConfigs, err := s.getHealthyConfigs()
	if err != nil {
		return nil, err
	}

	if len(availableConfigs) == 0 {
		return nil, fmt.Errorf("no healthy configs available")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.loadBalancer.Strategy {
	case "round_robin":
		return s.selectRoundRobin(availableConfigs)
	case "random":
		return s.selectRandom(availableConfigs)
	case "weighted":
		return s.selectWeighted(availableConfigs)
	case "least_connections":
		return s.selectLeastConnections(availableConfigs)
	default:
		return s.selectRoundRobin(availableConfigs)
	}
}

// getHealthyConfigs returns only healthy configs with closed circuit breakers
func (s *EnhancedSelector) getHealthyConfigs() ([]*database.APIConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var healthyConfigs []*database.APIConfig

	for _, config := range s.configs {
		// Check health status
		healthStatus, err := database.GetHealthStatus(config.ID)
		if err != nil || healthStatus.Status != "healthy" {
			continue
		}

		// Check circuit breaker state
		if s.circuitBreakerMgr != nil {
			cb := s.circuitBreakerMgr.GetCircuitBreaker(config.ID)
			if cb.GetState() == "open" {
				continue
			}
		}

		healthyConfigs = append(healthyConfigs, config)
	}

	return healthyConfigs, nil
}

// selectRoundRobin implements round robin strategy
func (s *EnhancedSelector) selectRoundRobin(configs []*database.APIConfig) (*database.APIConfig, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	// Find the next config in round-robin order
	config := configs[s.roundRobinIndex%len(configs)]
	s.roundRobinIndex++

	return config, nil
}

// selectRandom implements random strategy
func (s *EnhancedSelector) selectRandom(configs []*database.APIConfig) (*database.APIConfig, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	index := s.rng.Intn(len(configs))
	return configs[index], nil
}

// selectWeighted implements weighted strategy with dynamic weight support
func (s *EnhancedSelector) selectWeighted(configs []*database.APIConfig) (*database.APIConfig, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	// Build weight map for available configs and filter out zero-weight configs
	weights := make(map[string]int)
	totalWeight := 0
	eligibleConfigs := make([]*database.APIConfig, 0)

	for _, node := range s.loadBalancer.ConfigNodes {
		if !node.Enabled {
			continue
		}

		// Check if this config is in the healthy list
		var config *database.APIConfig
		for _, c := range configs {
			if c.ID == node.ConfigID {
				config = c
				break
			}
		}

		if config == nil {
			continue
		}

		// Get dynamic weight if enabled, otherwise use base weight
		weight := s.calculateWeight(node.ConfigID, node.Weight)

		// Skip nodes with zero weight
		if weight == 0 {
			continue
		}

		weights[node.ConfigID] = weight
		totalWeight += weight
		eligibleConfigs = append(eligibleConfigs, config)
	}

	if totalWeight == 0 || len(eligibleConfigs) == 0 {
		return nil, fmt.Errorf("no configs with non-zero weight available")
	}

	// Select based on weight
	randomWeight := s.rng.Intn(totalWeight)
	currentWeight := 0

	for _, config := range eligibleConfigs {
		weight, exists := weights[config.ID]
		if !exists {
			continue
		}

		currentWeight += weight
		if randomWeight < currentWeight {
			return config, nil
		}
	}

	// This should never happen if totalWeight > 0
	return nil, fmt.Errorf("failed to select config")
}

// selectLeastConnections implements least connections strategy
func (s *EnhancedSelector) selectLeastConnections(configs []*database.APIConfig) (*database.APIConfig, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	var selectedConfig *database.APIConfig
	minConnections := int(^uint(0) >> 1) // Max int

	for _, config := range configs {
		connections := s.connectionCounts[config.ID]
		if connections < minConnections {
			minConnections = connections
			selectedConfig = config
		}
	}

	if selectedConfig == nil {
		selectedConfig = configs[0]
	}

	s.connectionCounts[selectedConfig.ID]++
	return selectedConfig, nil
}

// ReleaseConnection decrements the connection count for a config
func (s *EnhancedSelector) ReleaseConnection(configID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count, exists := s.connectionCounts[configID]; exists && count > 0 {
		s.connectionCounts[configID]--
	}
}

// GetAvailableNodes returns all available (healthy) nodes
func (s *EnhancedSelector) GetAvailableNodes() ([]*database.APIConfig, error) {
	return s.getHealthyConfigs()
}

// RefreshNodes reloads the config nodes from the database
func (s *EnhancedSelector) RefreshNodes() error {
	// Reload load balancer
	lb, err := database.GetLoadBalancer(s.loadBalancer.ID)
	if err != nil {
		return fmt.Errorf("failed to reload load balancer: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadBalancer = lb
	s.configs = make([]*database.APIConfig, 0)

	// Load all configs for the nodes
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}

		config, err := database.GetAPIConfig(node.ConfigID)
		if err != nil {
			continue
		}

		if !config.Enabled {
			continue
		}

		s.configs = append(s.configs, config)
	}

	if len(s.configs) == 0 {
		return fmt.Errorf("no available configs in load balancer")
	}

	return nil
}

// GetLoadBalancer returns the load balancer
func (s *EnhancedSelector) GetLoadBalancer() *database.LoadBalancer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadBalancer
}

// GetConfigCount returns the number of loaded configs
func (s *EnhancedSelector) GetConfigCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.configs)
}

// calculateWeight calculates the effective weight for a config
// If dynamic weight is enabled, it uses performance-based weight, otherwise uses base weight
func (s *EnhancedSelector) calculateWeight(configID string, baseWeight int) int {
	// Check if dynamic weight is enabled (would need to be added to LoadBalancer struct)
	// For now, check if we have a dynamic weight calculated
	if dynamicWeight, exists := s.dynamicWeights[configID]; exists && dynamicWeight > 0 {
		return dynamicWeight
	}

	return baseWeight
}

// UpdateDynamicWeights updates dynamic weights based on performance metrics
// This should be called periodically by the load balancer manager
func (s *EnhancedSelector) UpdateDynamicWeights() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get performance metrics for each config
	for _, config := range s.configs {
		weight, err := s.calculateDynamicWeight(config.ID)
		if err != nil {
			// Log error but continue with other configs
			continue
		}

		s.dynamicWeights[config.ID] = weight
	}

	return nil
}

// calculateDynamicWeight calculates dynamic weight based on performance metrics
// Weight calculation formula:
// - Base weight from configuration
// - Adjusted by success rate (higher success rate = higher weight)
// - Adjusted by response time (lower response time = higher weight)
// - Minimum weight is 1, maximum is 100
func (s *EnhancedSelector) calculateDynamicWeight(configID string) (int, error) {
	// Get base weight from config nodes
	baseWeight := 10 // Default weight
	for _, node := range s.loadBalancer.ConfigNodes {
		if node.ConfigID == configID {
			baseWeight = node.Weight
			break
		}
	}

	// Get node stats for the last hour
	stats, err := database.GetNodeStatsForTimeWindow(s.loadBalancer.ID, configID, "1h")
	if err != nil || stats == nil {
		// If no stats available, return base weight
		return baseWeight, nil
	}

	// Calculate success rate factor (0.5 to 1.5)
	// 100% success rate = 1.5x, 50% success rate = 1.0x, 0% success rate = 0.5x
	successRateFactor := 0.5 + (stats.SuccessRate * 1.0)
	if successRateFactor < 0.5 {
		successRateFactor = 0.5
	}
	if successRateFactor > 1.5 {
		successRateFactor = 1.5
	}

	// Calculate response time factor (0.5 to 1.5)
	// Lower response time = higher factor
	// Assume baseline response time is 1000ms
	// < 500ms = 1.5x, 1000ms = 1.0x, > 2000ms = 0.5x
	responseTimeFactor := 1.0
	if stats.AvgResponseTimeMs > 0 {
		if stats.AvgResponseTimeMs < 500 {
			responseTimeFactor = 1.5
		} else if stats.AvgResponseTimeMs < 1000 {
			responseTimeFactor = 1.0 + (1000-stats.AvgResponseTimeMs)/1000.0
		} else if stats.AvgResponseTimeMs < 2000 {
			responseTimeFactor = 1.0 - (stats.AvgResponseTimeMs-1000)/2000.0
		} else {
			responseTimeFactor = 0.5
		}
	}

	// Calculate final weight
	dynamicWeight := float64(baseWeight) * successRateFactor * responseTimeFactor

	// Ensure weight is within bounds [1, 100]
	finalWeight := int(dynamicWeight)
	if finalWeight < 1 {
		finalWeight = 1
	}
	if finalWeight > 100 {
		finalWeight = 100
	}

	return finalWeight, nil
}

// GetDynamicWeight returns the current dynamic weight for a config
func (s *EnhancedSelector) GetDynamicWeight(configID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if weight, exists := s.dynamicWeights[configID]; exists {
		return weight
	}

	// Return base weight if no dynamic weight
	for _, node := range s.loadBalancer.ConfigNodes {
		if node.ConfigID == configID {
			return node.Weight
		}
	}

	return 10 // Default weight
}
