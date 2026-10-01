package handler

import (
	"math/rand"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// TestLoadBalancerSelector_New covers NewLoadBalancerSelector's happy path,
// its "skip disabled/unavailable config" branches and the "no available
// configs" error.
func TestLoadBalancerSelector_New(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)

	// Add a node pointing at a config ID that doesn't exist in the DB, and a
	// disabled node, and a node whose config is disabled - all three should
	// be skipped during construction.
	missingID := "missing-config-id"
	disabledConfig := &database.APIConfig{
		ID:            "disabled-config",
		Name:          "Disabled Config",
		OpenAIBaseURL: "https://api.disabled.com",
		Enabled:       false,
	}
	if err := database.CreateAPIConfig(disabledConfig); err != nil {
		t.Fatalf("failed to create disabled config: %v", err)
	}

	lb.ConfigNodes = append(lb.ConfigNodes,
		database.ConfigNode{ConfigID: missingID, Weight: 10, Enabled: true},
		database.ConfigNode{ConfigID: disabledConfig.ID, Weight: 10, Enabled: true},
		database.ConfigNode{ConfigID: lb.ConfigNodes[0].ConfigID, Weight: 10, Enabled: false},
	)

	selector, err := NewLoadBalancerSelector(lb)
	if err != nil {
		t.Fatalf("NewLoadBalancerSelector failed: %v", err)
	}
	if selector.GetConfigCount() != 2 {
		t.Errorf("expected 2 usable configs (missing/disabled/node-disabled skipped), got %d", selector.GetConfigCount())
	}
	if selector.GetLoadBalancer() != lb {
		t.Error("GetLoadBalancer should return the same load balancer instance")
	}
}

// TestLoadBalancerSelector_New_NoAvailableConfigs covers the error path when
// every node is unusable.
func TestLoadBalancerSelector_New_NoAvailableConfigs(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := &database.LoadBalancer{
		ID:       "empty-lb",
		Name:     "Empty LB",
		Strategy: "round_robin",
		Enabled:  true,
		ConfigNodes: []database.ConfigNode{
			{ConfigID: "does-not-exist", Weight: 10, Enabled: true},
		},
	}
	if err := database.CreateLoadBalancer(lb); err != nil {
		t.Fatalf("failed to create load balancer: %v", err)
	}

	if _, err := NewLoadBalancerSelector(lb); err == nil {
		t.Error("expected error when no configs are available")
	}
}

func newPlainSelector(t *testing.T, strategy string) (*LoadBalancerSelector, *database.LoadBalancer) {
	t.Helper()
	lb := createTestLoadBalancer(t)
	lb.Strategy = strategy
	selector, err := NewLoadBalancerSelector(lb)
	if err != nil {
		t.Fatalf("NewLoadBalancerSelector failed: %v", err)
	}
	return selector, lb
}

// TestLoadBalancerSelector_SelectConfig_Strategies exercises every branch of
// SelectConfig's strategy switch, including the "unknown strategy" default.
func TestLoadBalancerSelector_SelectConfig_Strategies(t *testing.T) {
	strategies := []string{"round_robin", "random", "weighted", "least_connections", "totally_unknown"}
	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			setupTestDB(t)
			defer cleanupTestDB(t)

			selector, lb := newPlainSelector(t, strategy)
			config, err := selector.SelectConfig()
			if err != nil {
				t.Fatalf("SelectConfig() failed: %v", err)
			}
			if config == nil {
				t.Fatal("SelectConfig() returned nil config")
			}
			found := false
			for _, node := range lb.ConfigNodes {
				if node.ConfigID == config.ID {
					found = true
				}
			}
			if !found {
				t.Errorf("selected config %s not among load balancer nodes", config.ID)
			}
		})
	}
}

// TestLoadBalancerSelector_RoundRobin verifies cyclic ordering and index wraparound.
func TestLoadBalancerSelector_RoundRobin(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	selector, lb := newPlainSelector(t, "round_robin")
	n := len(lb.ConfigNodes)

	var firstRound []string
	for i := 0; i < n; i++ {
		c, err := selector.SelectConfig()
		if err != nil {
			t.Fatalf("SelectConfig failed: %v", err)
		}
		firstRound = append(firstRound, c.ID)
	}
	var secondRound []string
	for i := 0; i < n; i++ {
		c, err := selector.SelectConfig()
		if err != nil {
			t.Fatalf("SelectConfig failed: %v", err)
		}
		secondRound = append(secondRound, c.ID)
	}
	for i := range firstRound {
		if firstRound[i] != secondRound[i] {
			t.Errorf("round robin did not wrap around consistently: %v vs %v", firstRound, secondRound)
		}
	}
}

// TestLoadBalancerSelector_Weighted_ZeroWeight covers the "totalWeight == 0"
// fallback-to-round-robin branch in selectWeighted.
func TestLoadBalancerSelector_Weighted_ZeroWeight(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	lb.Strategy = "weighted"
	for i := range lb.ConfigNodes {
		lb.ConfigNodes[i].Weight = 0
	}

	selector, err := NewLoadBalancerSelector(lb)
	if err != nil {
		t.Fatalf("NewLoadBalancerSelector failed: %v", err)
	}
	if _, err := selector.SelectConfig(); err != nil {
		t.Fatalf("expected fallback round-robin selection, got error: %v", err)
	}
}

// TestLoadBalancerSelector_Weighted_Distribution drives selectWeighted enough
// times to exercise the weighted-pick loop and confirm the heavier node wins
// more often.
func TestLoadBalancerSelector_Weighted_Distribution(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	lb.Strategy = "weighted"
	lb.ConfigNodes[0].Weight = 90
	lb.ConfigNodes[1].Weight = 10

	selector, err := NewLoadBalancerSelector(lb)
	if err != nil {
		t.Fatalf("NewLoadBalancerSelector failed: %v", err)
	}

	counts := map[string]int{}
	for i := 0; i < 500; i++ {
		c, err := selector.SelectConfig()
		if err != nil {
			t.Fatalf("SelectConfig failed: %v", err)
		}
		counts[c.ID]++
	}
	if counts[lb.ConfigNodes[0].ConfigID] <= counts[lb.ConfigNodes[1].ConfigID] {
		t.Errorf("expected heavier-weighted node to win more often, got %v", counts)
	}
}

// TestLoadBalancerSelector_LeastConnections_AndRelease covers
// selectLeastConnections and ReleaseConnection, including the "count already
// zero" no-op branch.
func TestLoadBalancerSelector_LeastConnections_AndRelease(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	selector, lb := newPlainSelector(t, "least_connections")

	first, err := selector.SelectConfig()
	if err != nil {
		t.Fatalf("SelectConfig failed: %v", err)
	}
	second, err := selector.SelectConfig()
	if err != nil {
		t.Fatalf("SelectConfig failed: %v", err)
	}
	if first.ID == second.ID && len(lb.ConfigNodes) > 1 {
		t.Error("expected least-connections to prefer the untouched node")
	}

	// Releasing a config with a positive count should decrement it.
	selector.ReleaseConnection(first.ID)
	// Releasing an unknown / already-zero config should be a no-op, not panic.
	selector.ReleaseConnection("unknown-config-id")
	selector.ReleaseConnection(first.ID) // already back to zero: hits the no-op branch
}

// TestLoadBalancerSelector_EmptyConfigsDefensiveChecks white-box tests the
// "len(s.configs) == 0" guards inside SelectConfig and each private select*
// method. These are unreachable via the public API (NewLoadBalancerSelector
// refuses to construct a selector with zero configs, and SelectConfig itself
// already checks before dispatching), so they're exercised directly here by
// constructing a selector with an empty configs slice.
func TestLoadBalancerSelector_EmptyConfigsDefensiveChecks(t *testing.T) {
	lb := &database.LoadBalancer{ID: "lb", Strategy: "round_robin"}
	s := &LoadBalancerSelector{
		loadBalancer:     lb,
		configs:          nil,
		connectionCounts: make(map[string]int),
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if _, err := s.SelectConfig(); err == nil {
		t.Error("expected error from SelectConfig with no configs")
	}
	if _, err := s.selectRoundRobin(); err == nil {
		t.Error("expected error from selectRoundRobin with no configs")
	}
	if _, err := s.selectRandom(); err == nil {
		t.Error("expected error from selectRandom with no configs")
	}
	if _, err := s.selectWeighted(); err == nil {
		t.Error("expected error from selectWeighted with no configs")
	}
	if _, err := s.selectLeastConnections(); err == nil {
		t.Error("expected error from selectLeastConnections with no configs")
	}
}

// TestLoadBalancerSelector_Weighted_SkipsDisabledNodes covers the
// "!node.Enabled -> continue" branches in both loops of selectWeighted by
// including a disabled node alongside enabled ones.
func TestLoadBalancerSelector_Weighted_SkipsDisabledNodes(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	lb.Strategy = "weighted"
	lb.ConfigNodes[0].Weight = 100
	// A disabled node with a large weight, placed ahead of the enabled ones,
	// must be skipped by both the total-weight loop and the weight-picking
	// loop (continue is hit mid-iteration rather than trailing after a match
	// has already returned).
	disabledFirst := database.ConfigNode{ConfigID: lb.ConfigNodes[1].ConfigID, Weight: 1000, Enabled: false}
	lb.ConfigNodes = append([]database.ConfigNode{disabledFirst}, lb.ConfigNodes...)

	selector, err := NewLoadBalancerSelector(lb)
	if err != nil {
		t.Fatalf("NewLoadBalancerSelector failed: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := selector.SelectConfig(); err != nil {
			t.Fatalf("SelectConfig failed: %v", err)
		}
	}
}

// TestLoadBalancerSelector_Weighted_FallbackPaths white-box tests the
// "no matching loaded config for the winning node" (falls through to the
// final `return s.configs[0], nil`) and selectLeastConnections'
// "selectedConfig stays nil" fallback — both require configs/ConfigNodes to
// disagree, which can't happen via the normal constructor.
func TestLoadBalancerSelector_Weighted_FallbackPaths(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	lb := createTestLoadBalancer(t)
	loaded := &database.APIConfig{ID: lb.ConfigNodes[0].ConfigID, Enabled: true}

	s := &LoadBalancerSelector{
		loadBalancer: lb,
		configs:      []*database.APIConfig{loaded},
		rng:          rand.New(rand.NewSource(1)),
	}
	// ConfigNodes references an ID that isn't in s.configs, so the inner
	// "find the config in our loaded configs" loop never matches and the
	// weight loop falls through to the end without returning, hitting the
	// final `return s.configs[0], nil`.
	s.loadBalancer.ConfigNodes = []database.ConfigNode{
		{ConfigID: "not-loaded", Weight: 10, Enabled: true},
	}
	config, err := s.selectWeighted()
	if err != nil {
		t.Fatalf("selectWeighted failed: %v", err)
	}
	if config.ID != loaded.ID {
		t.Errorf("expected fallback to configs[0] (%s), got %s", loaded.ID, config.ID)
	}
}
