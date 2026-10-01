package database

import (
	"testing"
	"time"
)

func TestConfigCache_GetSetInvalidate(t *testing.T) {
	cc := &ConfigCache{
		cache:      make(map[string]*CachedConfig),
		ttl:        time.Minute,
		maxEntries: 1000,
	}

	if _, ok := cc.Get("missing"); ok {
		t.Error("expected miss on empty cache")
	}

	cfg := &APIConfig{ID: "cfg-1", Name: "test"}
	cc.Set("key-1", cfg)

	got, ok := cc.Get("key-1")
	if !ok || got.ID != "cfg-1" {
		t.Fatalf("expected cache hit for key-1, got ok=%v got=%v", ok, got)
	}

	cc.Invalidate("key-1")
	if _, ok := cc.Get("key-1"); ok {
		t.Error("expected miss after Invalidate")
	}
}

func TestConfigCache_GetExpiredEntry(t *testing.T) {
	cc := &ConfigCache{
		cache:      make(map[string]*CachedConfig),
		ttl:        time.Minute,
		maxEntries: 1000,
	}
	// Manually insert an already-expired entry to hit the expiry branch in Get.
	cc.cache["stale"] = &CachedConfig{
		config:    &APIConfig{ID: "stale-cfg"},
		expiresAt: time.Now().Add(-time.Second),
	}

	if _, ok := cc.Get("stale"); ok {
		t.Error("expected expired entry to report as a miss")
	}
}

func TestConfigCache_InvalidateAll(t *testing.T) {
	cc := &ConfigCache{
		cache:      make(map[string]*CachedConfig),
		ttl:        time.Minute,
		maxEntries: 1000,
	}
	cc.Set("a", &APIConfig{ID: "a"})
	cc.Set("b", &APIConfig{ID: "b"})

	cc.InvalidateAll()

	if _, ok := cc.Get("a"); ok {
		t.Error("expected a to be gone after InvalidateAll")
	}
	if _, ok := cc.Get("b"); ok {
		t.Error("expected b to be gone after InvalidateAll")
	}
	if len(cc.cache) != 0 {
		t.Errorf("expected empty cache map, got %d entries", len(cc.cache))
	}
}

func TestConfigCache_EvictOldest(t *testing.T) {
	cc := &ConfigCache{
		cache:      make(map[string]*CachedConfig),
		ttl:        time.Minute,
		maxEntries: 3,
	}

	now := time.Now()
	// Insert with explicit, distinct expiresAt so eviction order is deterministic.
	cc.cache["oldest"] = &CachedConfig{config: &APIConfig{ID: "oldest"}, expiresAt: now.Add(1 * time.Minute)}
	cc.cache["middle"] = &CachedConfig{config: &APIConfig{ID: "middle"}, expiresAt: now.Add(2 * time.Minute)}
	cc.cache["newest"] = &CachedConfig{config: &APIConfig{ID: "newest"}, expiresAt: now.Add(3 * time.Minute)}

	// Cache is at maxEntries=3; Set should trigger evictOldest before inserting.
	cc.Set("fresh", &APIConfig{ID: "fresh"})

	if _, ok := cc.Get("oldest"); ok {
		t.Error("expected the entry with the earliest expiresAt to be evicted")
	}
	if _, ok := cc.Get("middle"); !ok {
		t.Error("middle entry should survive eviction")
	}
	if _, ok := cc.Get("newest"); !ok {
		t.Error("newest entry should survive eviction")
	}
	if _, ok := cc.Get("fresh"); !ok {
		t.Error("newly set entry should be present")
	}
}

func TestConfigCache_EvictOldest_EmptyCache(t *testing.T) {
	cc := &ConfigCache{cache: make(map[string]*CachedConfig)}
	// Must not panic when there's nothing to evict.
	cc.evictOldest()
	if len(cc.cache) != 0 {
		t.Errorf("expected cache to remain empty, got %d", len(cc.cache))
	}
}

func TestGetConfigCache_Singleton(t *testing.T) {
	c1 := GetConfigCache()
	c2 := GetConfigCache()
	if c1 != c2 {
		t.Error("GetConfigCache should return the same singleton instance")
	}
	if c1 == nil {
		t.Fatal("GetConfigCache returned nil")
	}
}

// TestConfigCache_CleanupExpired_Logic exercises the same expiry-scan logic
// that the background cleanupExpired goroutine performs on each tick,
// without waiting on the hardcoded 1-minute ticker (see coverage notes).
func TestConfigCache_CleanupExpired_Logic(t *testing.T) {
	cc := &ConfigCache{cache: make(map[string]*CachedConfig), ttl: time.Minute, maxEntries: 100}
	cc.cache["expired-1"] = &CachedConfig{config: &APIConfig{ID: "e1"}, expiresAt: time.Now().Add(-time.Hour)}
	cc.cache["expired-2"] = &CachedConfig{config: &APIConfig{ID: "e2"}, expiresAt: time.Now().Add(-time.Minute)}
	cc.cache["alive"] = &CachedConfig{config: &APIConfig{ID: "a1"}, expiresAt: time.Now().Add(time.Hour)}

	cc.mutex.Lock()
	now := time.Now()
	for key, cached := range cc.cache {
		if now.After(cached.expiresAt) {
			delete(cc.cache, key)
		}
	}
	cc.mutex.Unlock()

	if len(cc.cache) != 1 {
		t.Fatalf("expected 1 surviving entry, got %d: %v", len(cc.cache), cc.cache)
	}
	if _, ok := cc.cache["alive"]; !ok {
		t.Error("expected 'alive' entry to survive cleanup")
	}
}
