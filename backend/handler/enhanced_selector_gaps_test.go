package handler

import (
	"context"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// TestNewEnhancedSelector_Error covers the error branch when the initial
// RefreshNodes() call fails because none of the load balancer's nodes
// resolve to a usable config.
func TestNewEnhancedSelector_Error(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := &database.LoadBalancer{
		ID:       "empty-enhanced-lb",
		Name:     "Empty Enhanced LB",
		Strategy: "round_robin",
		Enabled:  true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "does-not-exist", Weight: 10, Enabled: true},
		},
	}
	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("failed to create load balancer: %v", err)
	}

	if _, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager()); err == nil {
		t.Error("expected error when no configs are available")
	}
}

// TestSelectConfig_UnknownStrategy covers SelectConfig's default branch,
// which falls back to round robin for an unrecognized strategy string.
func TestSelectConfig_UnknownStrategy(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	lb.Strategy = "totally-unknown-strategy"
	if err := database.UpdateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to update load balancer: %v", err)
	}

	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}
	markAllHealthy(t, lb)

	config, err := selector.SelectConfig(context.Background())
	if err != nil {
		t.Fatalf("SelectConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("expected a config to be selected via round-robin fallback")
	}
}

// TestEnhancedSelector_SelectStrategies_EmptyConfigsGuards white-box tests
// the "len(configs) == 0" guards inside each private select* method. These
// are unreachable via the public SelectConfig API (it already checks
// len(availableConfigs) == 0 before dispatching), so they're exercised
// directly here.
func TestEnhancedSelector_SelectStrategies_EmptyConfigsGuards(t *testing.T) {
	s := &EnhancedSelector{
		loadBalancer:     &database.LoadBalancer{ID: "lb"},
		connectionCounts: make(map[string]int),
		dynamicWeights:   make(map[string]int),
	}

	if _, err := s.selectRoundRobin(nil); err == nil {
		t.Error("expected error from selectRoundRobin with no configs")
	}
	if _, err := s.selectRandom(nil); err == nil {
		t.Error("expected error from selectRandom with no configs")
	}
	if _, err := s.selectWeighted(nil); err == nil {
		t.Error("expected error from selectWeighted with no configs")
	}
	if _, err := s.selectLeastConnections(nil); err == nil {
		t.Error("expected error from selectLeastConnections with no configs")
	}
}

// TestSelectConfig_Weighted_SkipsDisabledAndUnhealthyNodes covers the
// "!node.Enabled -> continue" and "config == nil (not in healthy list) ->
// continue" branches inside selectWeighted's weight-building loop.
func TestSelectConfig_Weighted_SkipsDisabledAndUnhealthyNodes(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	lb.Strategy = "weighted"

	// Add a third config node that is disabled, and mark the second node's
	// config unhealthy so it's excluded from the "healthy" list passed into
	// selectWeighted while still being enabled in ConfigNodes.
	thirdConfig := &database.APIConfig{
		ID:            "weighted-third-config",
		Name:          "Weighted Third Config",
		OpenAIBaseURL: "https://api.test3.com",
		Enabled:       true,
	}
	if err := database.CreateAPIConfig(thirdConfig); err != nil {
		t.Fatalf("failed to create third config: %v", err)
	}
	lb.ConfigNodes = append(lb.ConfigNodes,
		database.ConfigNode{ConfigID: thirdConfig.ID, Weight: 1000, Enabled: false},
	)
	if err := database.UpdateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to update load balancer: %v", err)
	}

	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	// Config 1 is healthy and eligible; config 2 is unhealthy (so it never
	// appears in the "configs" slice passed to selectWeighted, hitting the
	// "config == nil" skip); config 3 is disabled at the node level (hitting
	// the "!node.Enabled" skip).
	if err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
		ConfigID: lb.ConfigNodes[0].ConfigID, Status: "healthy", LastCheckTime: time.Now(),
	}); err != nil {
		t.Fatalf("Failed to update health status: %v", err)
	}
	if err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
		ConfigID: lb.ConfigNodes[1].ConfigID, Status: "unhealthy", LastCheckTime: time.Now(),
	}); err != nil {
		t.Fatalf("Failed to update health status: %v", err)
	}

	for i := 0; i < 20; i++ {
		config, err := selector.SelectConfig(context.Background())
		if err != nil {
			t.Fatalf("SelectConfig failed: %v", err)
		}
		if config.ID != lb.ConfigNodes[0].ConfigID {
			t.Errorf("expected only the healthy, enabled config to be selected, got %s", config.ID)
		}
	}
}

// TestSelectConfig_LeastConnections_ReleaseNoop covers ReleaseConnection's
// "count doesn't exist / already zero" no-op branch.
func TestSelectConfig_LeastConnections_ReleaseNoop(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	// Releasing a config that was never selected (no entry in
	// connectionCounts) must be a safe no-op.
	selector.ReleaseConnection("unknown-config-id")
	// Releasing a config whose count is already zero must also be a no-op.
	selector.ReleaseConnection(lb.ConfigNodes[0].ConfigID)
}

// TestEnhancedSelector_RefreshNodes_Errors covers RefreshNodes' error
// branches: reload failure, node-skip branches (disabled node, missing
// config, disabled config) and the final "no available configs" error.
func TestEnhancedSelector_RefreshNodes_Errors(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	t.Run("reload failure", func(t *testing.T) {
		if err := database.DeleteLoadBalancer(lb.ID); err != nil {
			t.Fatalf("failed to delete load balancer: %v", err)
		}
		if err := selector.RefreshNodes(); err == nil {
			t.Error("expected error when reloading a deleted load balancer")
		}
	})
}

// TestEnhancedSelector_RefreshNodes_SkipsAndFails covers the per-node skip
// branches (disabled node, missing config, disabled config) plus the final
// "no available configs in load balancer" error, using a fresh load
// balancer built from scratch so it isn't torn down by the previous test.
func TestEnhancedSelector_RefreshNodes_SkipsAndFails(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	healthyConfig := &database.APIConfig{ID: "refresh-healthy", Name: "Healthy", OpenAIBaseURL: "https://a.test", Enabled: true}
	disabledConfig := &database.APIConfig{ID: "refresh-disabled-config", Name: "Disabled", OpenAIBaseURL: "https://b.test", Enabled: false}
	if err := database.CreateAPIConfig(healthyConfig); err != nil {
		t.Fatalf("failed to create healthy config: %v", err)
	}
	if err := database.CreateAPIConfig(disabledConfig); err != nil {
		t.Fatalf("failed to create disabled config: %v", err)
	}

	lb := &database.LoadBalancer{
		ID:       "refresh-skip-lb",
		Name:     "Refresh Skip LB",
		Strategy: "round_robin",
		Enabled:  true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: healthyConfig.ID, Weight: 10, Enabled: true},
			{ConfigID: "missing-config-id", Weight: 10, Enabled: true},  // GetAPIConfig err -> skip
			{ConfigID: disabledConfig.ID, Weight: 10, Enabled: true},    // config.Enabled == false -> skip
			{ConfigID: healthyConfig.ID, Weight: 10, Enabled: false},    // node.Enabled == false -> skip
		},
	}
	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("failed to create load balancer: %v", err)
	}

	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}
	if selector.GetConfigCount() != 1 {
		t.Errorf("expected 1 usable config, got %d", selector.GetConfigCount())
	}

	// Now make every node unusable and confirm RefreshNodes reports the
	// final "no available configs" error.
	lb.ConfigNodes = []database.ConfigNode{
		{ConfigID: "still-missing", Weight: 10, Enabled: true},
	}
	if err := database.UpdateLoadBalancer(lb); err != nil {
		t.Fatalf("failed to update load balancer: %v", err)
	}
	if err := selector.RefreshNodes(); err == nil {
		t.Error("expected error when RefreshNodes finds no available configs")
	}
}

// TestEnhancedSelector_GetLoadBalancer covers the trivial GetLoadBalancer getter.
func TestEnhancedSelector_GetLoadBalancer(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}
	if got := selector.GetLoadBalancer(); got == nil || got.ID != lb.ID {
		t.Errorf("GetLoadBalancer() = %v, want load balancer with ID %s", got, lb.ID)
	}
}

// TestEnhancedSelector_CalculateWeight_DynamicOverride covers calculateWeight's
// "dynamic weight exists and is > 0" branch, which was previously untested
// (only the base-weight fallback was exercised).
func TestEnhancedSelector_CalculateWeight_DynamicOverride(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	configID := lb.ConfigNodes[0].ConfigID
	selector.dynamicWeights[configID] = 77

	if got := selector.calculateWeight(configID, 10); got != 77 {
		t.Errorf("calculateWeight() = %d, want dynamic weight 77", got)
	}

	// dynamicWeight <= 0 must fall back to the base weight.
	selector.dynamicWeights[configID] = 0
	if got := selector.calculateWeight(configID, 10); got != 10 {
		t.Errorf("calculateWeight() with zero dynamic weight = %d, want base weight 10", got)
	}
}

// TestEnhancedSelector_UpdateDynamicWeights covers UpdateDynamicWeights'
// happy path (weights get populated from calculateDynamicWeight) - the
// per-config error-skip `continue` branch is unreachable in practice since
// calculateDynamicWeight never returns a non-nil error for a config that is
// actually in s.configs (GetAPIConfig always succeeds for those IDs), so it
// isn't separately exercised here.
func TestEnhancedSelector_UpdateDynamicWeights(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	if err := selector.UpdateDynamicWeights(); err != nil {
		t.Fatalf("UpdateDynamicWeights failed: %v", err)
	}
	for _, node := range lb.ConfigNodes {
		if _, ok := selector.dynamicWeights[node.ConfigID]; !ok {
			t.Errorf("expected a dynamic weight to be computed for %s", node.ConfigID)
		}
	}
}

// TestCalculateDynamicWeight_NoStats covers the "stats unavailable -> return
// base weight" fallback branch, triggered when the underlying config can't
// be found (GetNodeStatsForTimeWindow -> GetAPIConfig fails).
func TestCalculateDynamicWeight_NoStats(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}
	// Give the (nonexistent) config a known base weight via ConfigNodes so
	// we can assert the exact fallback value.
	selector.loadBalancer.ConfigNodes = append(selector.loadBalancer.ConfigNodes,
		database.ConfigNode{ConfigID: "no-such-config", Weight: 42, Enabled: true})

	weight, err := selector.calculateDynamicWeight("no-such-config")
	if err != nil {
		t.Fatalf("calculateDynamicWeight returned unexpected error: %v", err)
	}
	if weight != 42 {
		t.Errorf("calculateDynamicWeight() = %d, want base weight 42 (no stats available)", weight)
	}
}

// TestCalculateDynamicWeight_ResponseTimeBuckets drives calculateDynamicWeight
// through every response-time-factor bucket (<500ms, <1000ms, <2000ms,
// >=2000ms) and both success-rate clamp directions, by seeding real
// load_balancer_request_logs rows that GetNodeStatsForTimeWindow aggregates.
func TestCalculateDynamicWeight_ResponseTimeBuckets(t *testing.T) {
	cases := []struct {
		name       string
		durationMs int
		success    bool
	}{
		{"fast_success", 100, true},    // <500ms -> factor 1.5; success rate 100% -> factor 1.5 (clamped)
		{"medium_success", 800, true},  // <1000ms -> interpolated factor
		{"slow_success", 1500, true},   // <2000ms -> interpolated factor
		{"very_slow_fail", 3000, false}, // >=2000ms -> factor 0.5; 0% success -> factor 0.5 (clamped)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			defer cleanupTestDB(t)

			lb := createTestLoadBalancer(t)
			selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
			if err != nil {
				t.Fatalf("Failed to create selector: %v", err)
			}
			configID := lb.ConfigNodes[0].ConfigID

			logEntry := &database.LoadBalancerRequestLog{
				LoadBalancerID:   lb.ID,
				SelectedConfigID: configID,
				RequestTime:      time.Now(),
				ResponseTime:     time.Now(),
				DurationMs:       tc.durationMs,
				StatusCode:       200,
				Success:          tc.success,
			}
			if err := database.CreateLoadBalancerRequestLog(logEntry); err != nil {
				t.Fatalf("failed to create request log: %v", err)
			}

			weight, err := selector.calculateDynamicWeight(configID)
			if err != nil {
				t.Fatalf("calculateDynamicWeight failed: %v", err)
			}
			if weight < 1 || weight > 100 {
				t.Errorf("calculateDynamicWeight() = %d, want value clamped to [1, 100]", weight)
			}
		})
	}
}

// TestGetDynamicWeight covers all three branches of GetDynamicWeight: an
// explicitly-set dynamic weight, falling back to the node's configured base
// weight, and falling back to the hardcoded default of 10 when the config
// isn't in the load balancer at all.
func TestGetDynamicWeight(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	lb.ConfigNodes[0].Weight = 33
	if err := database.UpdateLoadBalancer(lb); err != nil {
		t.Fatalf("Failed to update load balancer: %v", err)
	}
	selector, err := NewEnhancedSelector(lb, createTestCircuitBreakerManager())
	if err != nil {
		t.Fatalf("Failed to create selector: %v", err)
	}

	// Branch 1: explicit dynamic weight set.
	selector.dynamicWeights[lb.ConfigNodes[0].ConfigID] = 99
	if got := selector.GetDynamicWeight(lb.ConfigNodes[0].ConfigID); got != 99 {
		t.Errorf("GetDynamicWeight() = %d, want explicit dynamic weight 99", got)
	}

	// Branch 2: no dynamic weight, falls back to the node's base weight.
	if got := selector.GetDynamicWeight(lb.ConfigNodes[1].ConfigID); got != lb.ConfigNodes[1].Weight {
		t.Errorf("GetDynamicWeight() = %d, want base weight %d", got, lb.ConfigNodes[1].Weight)
	}

	// Branch 3: config not present in the load balancer at all -> hardcoded default.
	if got := selector.GetDynamicWeight("totally-unknown-config"); got != 10 {
		t.Errorf("GetDynamicWeight() for unknown config = %d, want default 10", got)
	}
}

// markAllHealthy marks every config node in lb as healthy.
func markAllHealthy(t *testing.T, lb *database.LoadBalancer) {
	t.Helper()
	for _, node := range lb.ConfigNodes {
		if err := database.CreateOrUpdateHealthStatus(&database.HealthStatus{
			ConfigID:      node.ConfigID,
			Status:        "healthy",
			LastCheckTime: time.Now(),
		}); err != nil {
			t.Fatalf("Failed to update health status: %v", err)
		}
	}
}
