package handler

import (
	"container/list"
	"sync"
	"time"
)

// Cache interface defines cache operations
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
	Size() int
	Stats() CacheStats
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	Size        int
	MaxSize     int
	HitRate     float64
}

// cacheEntry represents a cache entry with TTL
type cacheEntry struct {
	key        string
	value      interface{}
	expiration time.Time
	element    *list.Element
}

// LRUCache implements an LRU cache with TTL support
type LRUCache struct {
	maxSize    int
	items      map[string]*cacheEntry
	lruList    *list.List
	mu         sync.RWMutex
	hits       int64
	misses     int64
	evictions  int64
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(maxSize int) *LRUCache {
	return &LRUCache{
		maxSize: maxSize,
		items:   make(map[string]*cacheEntry),
		lruList: list.New(),
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiration) {
		c.removeEntry(entry)
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(entry.element)
	c.hits++
	return entry.value, true
}

// Set adds or updates a value in the cache
func (c *LRUCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if entry, exists := c.items[key]; exists {
		// Update existing entry
		entry.value = value
		entry.expiration = time.Now().Add(ttl)
		c.lruList.MoveToFront(entry.element)
		return
	}

	// Create new entry
	entry := &cacheEntry{
		key:        key,
		value:      value,
		expiration: time.Now().Add(ttl),
	}

	// Add to front of list
	entry.element = c.lruList.PushFront(entry)
	c.items[key] = entry

	// Evict if necessary
	if c.lruList.Len() > c.maxSize {
		c.evictOldest()
	}
}

// Delete removes a key from the cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[key]; exists {
		c.removeEntry(entry)
	}
}

// Clear removes all entries from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheEntry)
	c.lruList.Init()
}

// Size returns the current number of items in the cache
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns cache statistics
func (c *LRUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		Size:      len(c.items),
		MaxSize:   c.maxSize,
		HitRate:   hitRate,
	}
}

// evictOldest removes the least recently used entry
func (c *LRUCache) evictOldest() {
	element := c.lruList.Back()
	if element != nil {
		entry := element.Value.(*cacheEntry)
		c.removeEntry(entry)
		c.evictions++
	}
}

// removeEntry removes an entry from the cache
func (c *LRUCache) removeEntry(entry *cacheEntry) {
	c.lruList.Remove(entry.element)
	delete(c.items, entry.key)
}

// CleanupExpired removes all expired entries
func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	// Iterate through all entries and remove expired ones
	for key, entry := range c.items {
		if now.After(entry.expiration) {
			c.lruList.Remove(entry.element)
			delete(c.items, key)
			removed++
		}
	}

	return removed
}

// CacheManager manages multiple caches
type CacheManager struct {
	healthStatusCache    Cache
	circuitBreakerCache  Cache
	configCache          Cache
	cleanupInterval      time.Duration
	stopCleanup          chan struct{}
	mu                   sync.RWMutex
}

// NewCacheManager creates a new cache manager
func NewCacheManager(cleanupInterval time.Duration) *CacheManager {
	return &CacheManager{
		healthStatusCache:   NewLRUCache(1000),   // Cache up to 1000 health statuses
		circuitBreakerCache: NewLRUCache(1000),   // Cache up to 1000 circuit breaker states
		configCache:         NewLRUCache(500),    // Cache up to 500 configs
		cleanupInterval:     cleanupInterval,
		stopCleanup:         make(chan struct{}),
	}
}

// StartCleanup starts the periodic cleanup of expired entries
func (cm *CacheManager) StartCleanup() {
	ticker := time.NewTicker(cm.cleanupInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				cm.cleanupExpired()
			case <-cm.stopCleanup:
				ticker.Stop()
				return
			}
		}
	}()
}

// StopCleanup stops the periodic cleanup
func (cm *CacheManager) StopCleanup() {
	close(cm.stopCleanup)
}

// cleanupExpired removes expired entries from all caches
func (cm *CacheManager) cleanupExpired() {
	if lruCache, ok := cm.healthStatusCache.(*LRUCache); ok {
		lruCache.CleanupExpired()
	}
	if lruCache, ok := cm.circuitBreakerCache.(*LRUCache); ok {
		lruCache.CleanupExpired()
	}
	if lruCache, ok := cm.configCache.(*LRUCache); ok {
		lruCache.CleanupExpired()
	}
}

// GetHealthStatusCache returns the health status cache
func (cm *CacheManager) GetHealthStatusCache() Cache {
	return cm.healthStatusCache
}

// GetCircuitBreakerCache returns the circuit breaker cache
func (cm *CacheManager) GetCircuitBreakerCache() Cache {
	return cm.circuitBreakerCache
}

// GetConfigCache returns the config cache
func (cm *CacheManager) GetConfigCache() Cache {
	return cm.configCache
}

// GetAllStats returns statistics for all caches
func (cm *CacheManager) GetAllStats() map[string]CacheStats {
	return map[string]CacheStats{
		"health_status":    cm.healthStatusCache.Stats(),
		"circuit_breaker":  cm.circuitBreakerCache.Stats(),
		"config":           cm.configCache.Stats(),
	}
}

// ClearAll clears all caches
func (cm *CacheManager) ClearAll() {
	cm.healthStatusCache.Clear()
	cm.circuitBreakerCache.Clear()
	cm.configCache.Clear()
}
