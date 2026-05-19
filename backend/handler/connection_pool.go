package handler

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// ConnectionPool manages HTTP connections for a specific config node
type ConnectionPool struct {
	configID       string
	baseURL        string
	maxConnections int
	maxIdleTime    time.Duration
	transport      *http.Transport
	client         *http.Client
	mu             sync.RWMutex
	stats          *PoolStats
}

// PoolStats tracks connection pool statistics
type PoolStats struct {
	ActiveConnections int
	IdleConnections   int
	TotalRequests     int64
	FailedRequests    int64
	mu                sync.RWMutex
}

// ConnectionPoolManager manages connection pools for all config nodes
type ConnectionPoolManager struct {
	pools map[string]*ConnectionPool
	mu    sync.RWMutex
}

// ConnectionPoolConfig defines configuration for connection pools
type ConnectionPoolConfig struct {
	MaxConnectionsPerHost int
	MaxIdleConnections    int
	MaxIdleTime           time.Duration
	ConnectionTimeout     time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
}

// DefaultConnectionPoolConfig returns default connection pool configuration
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxConnectionsPerHost: 100,
		MaxIdleConnections:    10,
		MaxIdleTime:           90 * time.Second,
		ConnectionTimeout:     30 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
}

// NewConnectionPoolManager creates a new connection pool manager
func NewConnectionPoolManager() *ConnectionPoolManager {
	return &ConnectionPoolManager{
		pools: make(map[string]*ConnectionPool),
	}
}

// GetOrCreatePool gets an existing pool or creates a new one for the config
func (m *ConnectionPoolManager) GetOrCreatePool(config *database.APIConfig) (*ConnectionPool, error) {
	m.mu.RLock()
	pool, exists := m.pools[config.ID]
	m.mu.RUnlock()

	if exists {
		return pool, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, exists := m.pools[config.ID]; exists {
		return pool, nil
	}

	// Create new pool
	pool, err := NewConnectionPool(config, DefaultConnectionPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	m.pools[config.ID] = pool
	return pool, nil
}

// GetPool returns an existing pool for the config
func (m *ConnectionPoolManager) GetPool(configID string) (*ConnectionPool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pool, exists := m.pools[configID]
	if !exists {
		return nil, fmt.Errorf("connection pool not found for config: %s", configID)
	}

	return pool, nil
}

// RemovePool removes a connection pool
func (m *ConnectionPoolManager) RemovePool(configID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pool, exists := m.pools[configID]; exists {
		pool.Close()
		delete(m.pools, configID)
	}
}

// CloseAll closes all connection pools
func (m *ConnectionPoolManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pool := range m.pools {
		pool.Close()
	}

	m.pools = make(map[string]*ConnectionPool)
}

// GetStats returns statistics for all pools
func (m *ConnectionPoolManager) GetStats() map[string]*PoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]*PoolStats)
	for configID, pool := range m.pools {
		stats[configID] = pool.GetStats()
	}

	return stats
}

// NewConnectionPool creates a new connection pool for a config
func NewConnectionPool(config *database.APIConfig, poolConfig *ConnectionPoolConfig) (*ConnectionPool, error) {
	logger := utils.GetLogger()

	// Create custom transport with connection pooling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   poolConfig.ConnectionTimeout,
			KeepAlive: poolConfig.KeepAlive,
		}).DialContext,
		MaxIdleConns:          poolConfig.MaxIdleConnections,
		MaxIdleConnsPerHost:   poolConfig.MaxIdleConnections,
		MaxConnsPerHost:       poolConfig.MaxConnectionsPerHost,
		IdleConnTimeout:       poolConfig.MaxIdleTime,
		TLSHandshakeTimeout:   poolConfig.TLSHandshakeTimeout,
		ResponseHeaderTimeout: poolConfig.ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	// Create HTTP client with the transport
	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second, // Overall request timeout
	}

	pool := &ConnectionPool{
		configID:       config.ID,
		baseURL:        config.OpenAIBaseURL,
		maxConnections: poolConfig.MaxConnectionsPerHost,
		maxIdleTime:    poolConfig.MaxIdleTime,
		transport:      transport,
		client:         client,
		stats: &PoolStats{
			ActiveConnections: 0,
			IdleConnections:   0,
			TotalRequests:     0,
			FailedRequests:    0,
		},
	}

	logger.Debug("Created connection pool for config %s (%s) with max %d connections",
		config.ID, config.Name, poolConfig.MaxConnectionsPerHost)

	return pool, nil
}

// GetClient returns the HTTP client for this pool
func (p *ConnectionPool) GetClient() *http.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client
}

// Do executes an HTTP request using the connection pool
func (p *ConnectionPool) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	p.incrementActive()
	defer p.decrementActive()

	p.stats.mu.Lock()
	p.stats.TotalRequests++
	p.stats.mu.Unlock()

	// Execute request with context
	resp, err := p.client.Do(req.WithContext(ctx))
	if err != nil {
		p.stats.mu.Lock()
		p.stats.FailedRequests++
		p.stats.mu.Unlock()
		return nil, err
	}

	return resp, nil
}

// incrementActive increments the active connection count
func (p *ConnectionPool) incrementActive() {
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()
	p.stats.ActiveConnections++
}

// decrementActive decrements the active connection count
func (p *ConnectionPool) decrementActive() {
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()
	if p.stats.ActiveConnections > 0 {
		p.stats.ActiveConnections--
	}
}

// GetStats returns current pool statistics
func (p *ConnectionPool) GetStats() *PoolStats {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()

	return &PoolStats{
		ActiveConnections: p.stats.ActiveConnections,
		IdleConnections:   p.stats.IdleConnections,
		TotalRequests:     p.stats.TotalRequests,
		FailedRequests:    p.stats.FailedRequests,
	}
}

// Close closes the connection pool and cleans up resources
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}

	logger := utils.GetLogger()
	logger.Debug("Closed connection pool for config %s", p.configID)
}

// CleanupIdleConnections closes idle connections that have exceeded the max idle time
func (p *ConnectionPool) CleanupIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
}

// GetConfigID returns the config ID for this pool
func (p *ConnectionPool) GetConfigID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.configID
}

// GetBaseURL returns the base URL for this pool
func (p *ConnectionPool) GetBaseURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.baseURL
}
