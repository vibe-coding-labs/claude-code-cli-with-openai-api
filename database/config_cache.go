package database

import (
	"sync"
	"time"
)

// ConfigCache provides a thread-safe cache for API configurations
type ConfigCache struct {
	cache      map[string]*CachedConfig
	mutex      sync.RWMutex
	ttl        time.Duration
	maxEntries int
}

// CachedConfig holds a configuration with its expiry time
type CachedConfig struct {
	config    *APIConfig
	expiresAt time.Time
}

var (
	globalCache     *ConfigCache
	globalCacheOnce sync.Once
)

// GetConfigCache returns the global config cache instance
func GetConfigCache() *ConfigCache {
	globalCacheOnce.Do(func() {
		globalCache = &ConfigCache{
			cache:      make(map[string]*CachedConfig),
			ttl:        5 * time.Minute, // Cache configs for 5 minutes
			maxEntries: 1000,            // Maximum 1000 cached entries
		}
		// Start background cleanup goroutine
		go globalCache.cleanupExpired()
	})
	return globalCache
}

// Get retrieves a config from cache by API key
func (cc *ConfigCache) Get(apiKey string) (*APIConfig, bool) {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()

	cached, exists := cc.cache[apiKey]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(cached.expiresAt) {
		return nil, false
	}

	return cached.config, true
}

// Set stores a config in cache with API key as the key
func (cc *ConfigCache) Set(apiKey string, config *APIConfig) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// Check if we need to make room
	if len(cc.cache) >= cc.maxEntries {
		// Remove oldest entry (simple LRU approximation)
		cc.evictOldest()
	}

	cc.cache[apiKey] = &CachedConfig{
		config:    config,
		expiresAt: time.Now().Add(cc.ttl),
	}
}

// Invalidate removes a config from cache
func (cc *ConfigCache) Invalidate(apiKey string) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	delete(cc.cache, apiKey)
}

// InvalidateAll clears the entire cache
func (cc *ConfigCache) InvalidateAll() {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.cache = make(map[string]*CachedConfig)
}

// evictOldest removes the oldest entry (must be called with lock held)
func (cc *ConfigCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, cached := range cc.cache {
		if oldestKey == "" || cached.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = cached.expiresAt
		}
	}

	if oldestKey != "" {
		delete(cc.cache, oldestKey)
	}
}

// cleanupExpired periodically removes expired entries
func (cc *ConfigCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cc.mutex.Lock()
		now := time.Now()
		for key, cached := range cc.cache {
			if now.After(cached.expiresAt) {
				delete(cc.cache, key)
			}
		}
		cc.mutex.Unlock()
	}
}
