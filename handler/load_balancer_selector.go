package handler

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// LoadBalancerSelector manages load balancing strategy and config selection
type LoadBalancerSelector struct {
	loadBalancer *database.LoadBalancer
	configs      []*database.APIConfig
	mu           sync.Mutex
	
	// State for round robin
	roundRobinIndex int
	
	// State for least connections
	connectionCounts map[string]int
	
	// Random generator
	rng *rand.Rand
}

// NewLoadBalancerSelector creates a new load balancer selector
func NewLoadBalancerSelector(lb *database.LoadBalancer) (*LoadBalancerSelector, error) {
	selector := &LoadBalancerSelector{
		loadBalancer:     lb,
		configs:          make([]*database.APIConfig, 0),
		roundRobinIndex:  0,
		connectionCounts: make(map[string]int),
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Load all configs for the nodes
	for _, node := range lb.ConfigNodes {
		if !node.Enabled {
			continue
		}
		
		config, err := database.GetAPIConfig(node.ConfigID)
		if err != nil {
			continue // Skip unavailable configs
		}
		
		if !config.Enabled {
			continue // Skip disabled configs
		}
		
		selector.configs = append(selector.configs, config)
	}

	if len(selector.configs) == 0 {
		return nil, fmt.Errorf("no available configs in load balancer")
	}

	return selector, nil
}

// SelectConfig selects a config based on the load balancing strategy
func (s *LoadBalancerSelector) SelectConfig() (*database.APIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	switch s.loadBalancer.Strategy {
	case "round_robin":
		return s.selectRoundRobin()
	case "random":
		return s.selectRandom()
	case "weighted":
		return s.selectWeighted()
	case "least_connections":
		return s.selectLeastConnections()
	default:
		return s.selectRoundRobin() // Default to round robin
	}
}

// selectRoundRobin implements round robin strategy
func (s *LoadBalancerSelector) selectRoundRobin() (*database.APIConfig, error) {
	if len(s.configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	config := s.configs[s.roundRobinIndex]
	s.roundRobinIndex = (s.roundRobinIndex + 1) % len(s.configs)
	
	return config, nil
}

// selectRandom implements random strategy
func (s *LoadBalancerSelector) selectRandom() (*database.APIConfig, error) {
	if len(s.configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	index := s.rng.Intn(len(s.configs))
	return s.configs[index], nil
}

// selectWeighted implements weighted strategy
func (s *LoadBalancerSelector) selectWeighted() (*database.APIConfig, error) {
	if len(s.configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	// Calculate total weight
	totalWeight := 0
	for _, node := range s.loadBalancer.ConfigNodes {
		if !node.Enabled {
			continue
		}
		totalWeight += node.Weight
	}

	if totalWeight == 0 {
		// Fallback to round robin if no weights
		return s.selectRoundRobin()
	}

	// Generate random number in range [0, totalWeight)
	randomWeight := s.rng.Intn(totalWeight)
	
	// Find the config based on weight
	currentWeight := 0
	for _, node := range s.loadBalancer.ConfigNodes {
		if !node.Enabled {
			continue
		}
		
		currentWeight += node.Weight
		if randomWeight < currentWeight {
			// Find the config in our loaded configs
			for _, config := range s.configs {
				if config.ID == node.ConfigID {
					return config, nil
				}
			}
		}
	}

	// Fallback to first config
	return s.configs[0], nil
}

// selectLeastConnections implements least connections strategy
func (s *LoadBalancerSelector) selectLeastConnections() (*database.APIConfig, error) {
	if len(s.configs) == 0 {
		return nil, fmt.Errorf("no available configs")
	}

	// Find config with least connections
	var selectedConfig *database.APIConfig
	minConnections := int(^uint(0) >> 1) // Max int

	for _, config := range s.configs {
		connections := s.connectionCounts[config.ID]
		if connections < minConnections {
			minConnections = connections
			selectedConfig = config
		}
	}

	if selectedConfig == nil {
		selectedConfig = s.configs[0]
	}

	// Increment connection count
	s.connectionCounts[selectedConfig.ID]++

	return selectedConfig, nil
}

// ReleaseConnection decrements the connection count for a config (for least connections strategy)
func (s *LoadBalancerSelector) ReleaseConnection(configID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count, exists := s.connectionCounts[configID]; exists && count > 0 {
		s.connectionCounts[configID]--
	}
}

// GetLoadBalancer returns the load balancer
func (s *LoadBalancerSelector) GetLoadBalancer() *database.LoadBalancer {
	return s.loadBalancer
}

// GetConfigCount returns the number of available configs
func (s *LoadBalancerSelector) GetConfigCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.configs)
}
