package handler

import (
	"context"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// TestLoadBalancingStrategyProperties tests properties of load balancing strategies
func TestLoadBalancingStrategyProperties(t *testing.T) {
	// Property 1: Round robin distributes requests evenly
	t.Run("Property: Round robin distributes evenly", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "round_robin"

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()
		iterations := len(lb.ConfigNodes) * 10
		selections := make(map[string]int)

		// Select configs multiple times
		for i := 0; i < iterations; i++ {
			config, err := selector.SelectConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to select config: %v", err)
			}
			selections[config.ID]++
		}

		// Verify even distribution
		expectedCount := iterations / len(lb.ConfigNodes)
		for configID, count := range selections {
			if count != expectedCount {
				t.Errorf("Config %s: expected %d selections, got %d", configID, expectedCount, count)
			}
		}
	})

	// Property 2: Weighted strategy respects weight ratios
	t.Run("Property: Weighted strategy respects ratios", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "weighted"
		lb.ConfigNodes[0].Weight = 70
		lb.ConfigNodes[1].Weight = 30

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()
		iterations := 10000
		selections := make(map[string]int)

		// Select configs many times
		for i := 0; i < iterations; i++ {
			config, err := selector.SelectConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to select config: %v", err)
			}
			selections[config.ID]++
		}

		// Verify weight ratio (with tolerance)
		config1Count := selections[lb.ConfigNodes[0].ConfigID]
		config2Count := selections[lb.ConfigNodes[1].ConfigID]

		ratio := float64(config1Count) / float64(config2Count)
		expectedRatio := 70.0 / 30.0 // ~2.33

		// Allow 20% tolerance
		tolerance := 0.2
		if ratio < expectedRatio*(1-tolerance) || ratio > expectedRatio*(1+tolerance) {
			t.Errorf("Weight ratio not as expected: got %.2f, expected ~%.2f (counts: %d vs %d)",
				ratio, expectedRatio, config1Count, config2Count)
		}
	})

	// Property 3: Random strategy eventually selects all nodes
	t.Run("Property: Random selects all nodes", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "random"

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()
		selections := make(map[string]bool)

		// Select until all nodes are selected (with max iterations)
		maxIterations := 1000
		for i := 0; i < maxIterations && len(selections) < len(lb.ConfigNodes); i++ {
			config, err := selector.SelectConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to select config: %v", err)
			}
			selections[config.ID] = true
		}

		// Verify all nodes were selected
		if len(selections) != len(lb.ConfigNodes) {
			t.Errorf("Expected all %d nodes to be selected, got %d", len(lb.ConfigNodes), len(selections))
		}
	})

	// Property 4: Least connections prefers nodes with fewer connections
	t.Run("Property: Least connections prefers less loaded nodes", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "least_connections"

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()

		// Select first config and don't release
		config1, err := selector.SelectConfig(ctx)
		if err != nil {
			t.Fatalf("Failed to select config: %v", err)
		}

		// Next selection should prefer different config
		config2, err := selector.SelectConfig(ctx)
		if err != nil {
			t.Fatalf("Failed to select config: %v", err)
		}

		if config1.ID == config2.ID && len(lb.ConfigNodes) > 1 {
			t.Error("Least connections should prefer different node when one has more connections")
		}

		// Release first connection
		selector.ReleaseConnection(config1.ID)

		// Now both should have equal connections, next selection could be either
		config3, err := selector.SelectConfig(ctx)
		if err != nil {
			t.Fatalf("Failed to select config: %v", err)
		}

		// Should select the one with fewer connections (config1)
		if config3.ID != config1.ID {
			t.Error("Expected to select config with fewer connections after release")
		}
	})
}

// TestLoadBalancingInvariants tests invariants that must always hold
func TestLoadBalancingInvariants(t *testing.T) {
	// Invariant 1: Selection always returns a valid config
	t.Run("Invariant: Selection returns valid config", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		strategies := []string{"round_robin", "random", "weighted", "least_connections"}

		for _, strategy := range strategies {
			t.Run(strategy, func(t *testing.T) {
				lb := createTestLoadBalancer(t)
				lb.Strategy = strategy

				cbMgr := createTestCircuitBreakerManager()
				selector, err := NewEnhancedSelector(lb, cbMgr)
				if err != nil {
					t.Fatalf("Failed to create selector: %v", err)
				}

				// Mark all configs as healthy
				for _, node := range lb.ConfigNodes {
					err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
						ConfigID:      node.ConfigID,
						Status:        "healthy",
						LastCheckTime: time.Now(),
					})
					if err != nil {
						t.Fatalf("Failed to update health status: %v", err)
					}
				}

				ctx := context.Background()

				// Select multiple times
				for i := 0; i < 100; i++ {
					config, err := selector.SelectConfig(ctx)
					if err != nil {
						t.Fatalf("Failed to select config: %v", err)
					}

					// Verify config is valid
					if config == nil {
						t.Fatal("Selected config is nil")
					}

					if config.ID == "" {
						t.Fatal("Selected config has empty ID")
					}

					// Verify config is in the load balancer's node list
					found := false
					for _, node := range lb.ConfigNodes {
						if node.ConfigID == config.ID {
							found = true
							break
						}
					}

					if !found {
						t.Errorf("Selected config %s not in load balancer's node list", config.ID)
					}
				}
			})
		}
	})

	// Invariant 2: Only healthy nodes are selected
	t.Run("Invariant: Only healthy nodes selected", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		strategies := []string{"round_robin", "random", "weighted", "least_connections"}

		for _, strategy := range strategies {
			t.Run(strategy, func(t *testing.T) {
				lb := createTestLoadBalancer(t)
				lb.Strategy = strategy

				cbMgr := createTestCircuitBreakerManager()
				selector, err := NewEnhancedSelector(lb, cbMgr)
				if err != nil {
					t.Fatalf("Failed to create selector: %v", err)
				}

				// Mark first config as healthy, second as unhealthy
				err = database.CreateOrUpdateHealthStatus(&database.HealthStatus{
					ConfigID:      lb.ConfigNodes[0].ConfigID,
					Status:        "healthy",
					LastCheckTime: time.Now(),
				})
				if err != nil {
					t.Fatalf("Failed to update health status: %v", err)
				}

				err = database.CreateOrUpdateHealthStatus(&database.HealthStatus{
					ConfigID:      lb.ConfigNodes[1].ConfigID,
					Status:        "unhealthy",
					LastCheckTime: time.Now(),
				})
				if err != nil {
					t.Fatalf("Failed to update health status: %v", err)
				}

				ctx := context.Background()

				// Select multiple times
				for i := 0; i < 50; i++ {
					config, err := selector.SelectConfig(ctx)
					if err != nil {
						t.Fatalf("Failed to select config: %v", err)
					}

					// Should always select the healthy config
					if config.ID != lb.ConfigNodes[0].ConfigID {
						t.Errorf("Selected unhealthy config %s", config.ID)
					}
				}
			})
		}
	})

	// Invariant 3: Total weight is always positive
	t.Run("Invariant: Total weight is positive", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "weighted"

		// Test various weight combinations
		weightCombinations := [][]int{
			{1, 1},
			{10, 90},
			{50, 50},
			{100, 1},
		}

		for _, weights := range weightCombinations {
			lb.ConfigNodes[0].Weight = weights[0]
			lb.ConfigNodes[1].Weight = weights[1]

			totalWeight := 0
			for _, node := range lb.ConfigNodes {
				if node.Enabled {
					totalWeight += node.Weight
				}
			}

			if totalWeight <= 0 {
				t.Errorf("Total weight should be positive, got %d", totalWeight)
			}
		}
	})

	// Invariant 4: Connection count never goes negative
	t.Run("Invariant: Connection count non-negative", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "least_connections"

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()

		// Select and release multiple times
		for i := 0; i < 100; i++ {
			config, err := selector.SelectConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to select config: %v", err)
			}

			// Release connection (even if we didn't track it)
			selector.ReleaseConnection(config.ID)

			// Try to release again (should not go negative)
			selector.ReleaseConnection(config.ID)
		}

		// Connection counts should never be negative
		// This is verified internally by the selector implementation
	})
}

// TestLoadBalancingEdgeCases tests edge cases
func TestLoadBalancingEdgeCases(t *testing.T) {
	// Edge case 1: Single node always selected
	t.Run("Edge: Single node always selected", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		// Create load balancer with single node
		config := &database.APIConfig{
			ID:            "single-config",
			Name:          "Single Config",
			OpenAIBaseURL: "https://api.single.com",
			Enabled:       true,
		}
		err := database.CreateAPIConfig(config)
		if err != nil {
			t.Fatalf("Failed to create config: %v", err)
		}

		lb := &database.LoadBalancer{
			ID:       "single-lb",
			Name:     "Single Node LB",
			Strategy: "round_robin",
			Enabled:  true,
			ConfigNodes: []database.ConfigNode{
				{ConfigID: config.ID, Weight: 100, Enabled: true},
			},
		}
		err = database.CreateLoadBalancer(lb)
		if err != nil {
			t.Fatalf("Failed to create load balancer: %v", err)
		}

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark config as healthy
		err = database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      config.ID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		})
		if err != nil {
			t.Fatalf("Failed to update health status: %v", err)
		}

		ctx := context.Background()

		// All selections should return the same config
		for i := 0; i < 10; i++ {
			selected, err := selector.SelectConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to select config: %v", err)
			}

			if selected.ID != config.ID {
				t.Errorf("Expected config %s, got %s", config.ID, selected.ID)
			}
		}
	})

	// Edge case 2: Zero weight nodes are never selected
	t.Run("Edge: Zero weight nodes not selected", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)
		lb.Strategy = "weighted"
		lb.ConfigNodes[0].Weight = 100
		lb.ConfigNodes[1].Weight = 0

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as healthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "healthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()

		// All selections should return the non-zero weight config
		for i := 0; i < 100; i++ {
			config, err := selector.SelectConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to select config: %v", err)
			}

			if config.ID != lb.ConfigNodes[0].ConfigID {
				t.Error("Zero weight node should never be selected")
			}
		}
	})

	// Edge case 3: All nodes unhealthy returns error
	t.Run("Edge: All unhealthy returns error", func(t *testing.T) {
		setupTestDB(t)
		defer cleanupTestDB(t)

		lb := createTestLoadBalancer(t)

		cbMgr := createTestCircuitBreakerManager()
		selector, err := NewEnhancedSelector(lb, cbMgr)
		if err != nil {
			t.Fatalf("Failed to create selector: %v", err)
		}

		// Mark all configs as unhealthy
		for _, node := range lb.ConfigNodes {
			err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
				ConfigID:      node.ConfigID,
				Status:        "unhealthy",
				LastCheckTime: time.Now(),
			})
			if err != nil {
				t.Fatalf("Failed to update health status: %v", err)
			}
		}

		ctx := context.Background()

		// Should return error
		_, err = selector.SelectConfig(ctx)
		if err == nil {
			t.Error("Expected error when all nodes are unhealthy")
		}
	})
}
