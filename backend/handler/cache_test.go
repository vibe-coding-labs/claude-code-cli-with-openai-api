package handler

import (
	"testing"
	"time"
)

func TestLRUCache_SetAndGet(t *testing.T) {
	cache := NewLRUCache(3)

	// Test basic set and get
	cache.Set("key1", "value1", 1*time.Hour)
	value, exists := cache.Get("key1")

	if !exists {
		t.Error("Expected key1 to exist")
	}

	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}

	// Test non-existent key
	_, exists = cache.Get("key2")
	if exists {
		t.Error("Expected key2 to not exist")
	}
}

func TestLRUCache_TTL(t *testing.T) {
	cache := NewLRUCache(10)

	// Set with short TTL
	cache.Set("key1", "value1", 100*time.Millisecond)

	// Should exist immediately
	_, exists := cache.Get("key1")
	if !exists {
		t.Error("Expected key1 to exist immediately")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not exist after expiration
	_, exists = cache.Get("key1")
	if exists {
		t.Error("Expected key1 to be expired")
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	cache := NewLRUCache(3)

	// Fill cache to capacity
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)

	// All keys should exist
	if cache.Size() != 3 {
		t.Errorf("Expected size 3, got %d", cache.Size())
	}

	// Add one more key, should evict oldest (key1)
	cache.Set("key4", "value4", 1*time.Hour)

	if cache.Size() != 3 {
		t.Errorf("Expected size 3 after eviction, got %d", cache.Size())
	}

	// key1 should be evicted
	_, exists := cache.Get("key1")
	if exists {
		t.Error("Expected key1 to be evicted")
	}

	// Other keys should still exist
	_, exists = cache.Get("key2")
	if !exists {
		t.Error("Expected key2 to exist")
	}

	_, exists = cache.Get("key3")
	if !exists {
		t.Error("Expected key3 to exist")
	}

	_, exists = cache.Get("key4")
	if !exists {
		t.Error("Expected key4 to exist")
	}
}

func TestLRUCache_LRUOrder(t *testing.T) {
	cache := NewLRUCache(3)

	// Fill cache
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add new key, should evict key2 (least recently used)
	cache.Set("key4", "value4", 1*time.Hour)

	// key2 should be evicted
	_, exists := cache.Get("key2")
	if exists {
		t.Error("Expected key2 to be evicted")
	}

	// key1 should still exist (was accessed recently)
	_, exists = cache.Get("key1")
	if !exists {
		t.Error("Expected key1 to exist")
	}
}

func TestLRUCache_Update(t *testing.T) {
	cache := NewLRUCache(10)

	// Set initial value
	cache.Set("key1", "value1", 1*time.Hour)

	// Update value
	cache.Set("key1", "value2", 1*time.Hour)

	value, exists := cache.Get("key1")
	if !exists {
		t.Error("Expected key1 to exist")
	}

	if value != "value2" {
		t.Errorf("Expected value2, got %v", value)
	}

	// Size should still be 1
	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}
}

func TestLRUCache_Delete(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)

	// Delete key1
	cache.Delete("key1")

	_, exists := cache.Get("key1")
	if exists {
		t.Error("Expected key1 to be deleted")
	}

	// key2 should still exist
	_, exists = cache.Get("key2")
	if !exists {
		t.Error("Expected key2 to exist")
	}

	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}
}

func TestLRUCache_Clear(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}

	_, exists := cache.Get("key1")
	if exists {
		t.Error("Expected key1 to not exist after clear")
	}
}

func TestLRUCache_Stats(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)

	// Generate some hits and misses
	cache.Get("key1") // hit
	cache.Get("key1") // hit
	cache.Get("key3") // miss
	cache.Get("key2") // hit

	stats := cache.Stats()

	if stats.Hits != 3 {
		t.Errorf("Expected 3 hits, got %d", stats.Hits)
	}

	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	if stats.Size != 2 {
		t.Errorf("Expected size 2, got %d", stats.Size)
	}

	expectedHitRate := 3.0 / 4.0
	if stats.HitRate != expectedHitRate {
		t.Errorf("Expected hit rate %.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestLRUCache_CleanupExpired(t *testing.T) {
	cache := NewLRUCache(10)

	// Add entries with different TTLs
	cache.Set("key1", "value1", 100*time.Millisecond)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 100*time.Millisecond)

	// Wait for some entries to expire
	time.Sleep(150 * time.Millisecond)

	// Cleanup expired entries
	removed := cache.CleanupExpired()

	if removed != 2 {
		t.Errorf("Expected 2 entries to be removed, got %d", removed)
	}

	if cache.Size() != 1 {
		t.Errorf("Expected size 1 after cleanup, got %d", cache.Size())
	}

	// key2 should still exist
	_, exists := cache.Get("key2")
	if !exists {
		t.Error("Expected key2 to exist after cleanup")
	}
}

func TestCacheManager(t *testing.T) {
	manager := NewCacheManager(1 * time.Second)

	// Test getting caches
	healthCache := manager.GetHealthStatusCache()
	if healthCache == nil {
		t.Error("Expected health status cache to exist")
	}

	cbCache := manager.GetCircuitBreakerCache()
	if cbCache == nil {
		t.Error("Expected circuit breaker cache to exist")
	}

	configCache := manager.GetConfigCache()
	if configCache == nil {
		t.Error("Expected config cache to exist")
	}

	// Test setting values
	healthCache.Set("config1", "healthy", 1*time.Hour)
	cbCache.Set("config1", "closed", 1*time.Hour)
	configCache.Set("config1", "config_data", 1*time.Hour)

	// Test getting values
	value, exists := healthCache.Get("config1")
	if !exists || value != "healthy" {
		t.Error("Expected to get health status from cache")
	}

	value, exists = cbCache.Get("config1")
	if !exists || value != "closed" {
		t.Error("Expected to get circuit breaker state from cache")
	}

	value, exists = configCache.Get("config1")
	if !exists || value != "config_data" {
		t.Error("Expected to get config from cache")
	}

	// Test stats
	stats := manager.GetAllStats()
	if len(stats) != 3 {
		t.Errorf("Expected 3 cache stats, got %d", len(stats))
	}

	// Test clear all
	manager.ClearAll()
	if healthCache.Size() != 0 {
		t.Error("Expected health cache to be empty after clear")
	}
}

func TestCacheManager_Cleanup(t *testing.T) {
	manager := NewCacheManager(100 * time.Millisecond)

	// Add entries with short TTL
	healthCache := manager.GetHealthStatusCache()
	healthCache.Set("key1", "value1", 50*time.Millisecond)
	healthCache.Set("key2", "value2", 1*time.Hour)

	// Start cleanup
	manager.StartCleanup()

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Stop cleanup
	manager.StopCleanup()

	// key1 should be cleaned up
	_, exists := healthCache.Get("key1")
	if exists {
		t.Error("Expected key1 to be cleaned up")
	}

	// key2 should still exist
	_, exists = healthCache.Get("key2")
	if !exists {
		t.Error("Expected key2 to exist")
	}
}

func TestLRUCache_Concurrency(t *testing.T) {
	cache := NewLRUCache(100)
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				cache.Set(string(rune(id*100+j)), j, 1*time.Hour)
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				cache.Get(string(rune(id*100 + j)))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should not panic and should have some entries
	if cache.Size() == 0 {
		t.Error("Expected cache to have entries after concurrent operations")
	}
}
