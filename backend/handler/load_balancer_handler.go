package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// LoadBalancerRequest represents a request to create or update a load balancer
type LoadBalancerRequest struct {
	Name            string                `json:"name" binding:"required"`
	Description     string                `json:"description"`
	Strategy        string                `json:"strategy" binding:"required"`
	ConfigNodes     []database.ConfigNode `json:"config_nodes" binding:"required"`
	Enabled         bool                  `json:"enabled"`
	AnthropicAPIKey string                `json:"anthropic_api_key,omitempty"`

	// Health check configuration
	HealthCheckEnabled  bool `json:"health_check_enabled"`
	HealthCheckInterval int  `json:"health_check_interval"` // seconds
	FailureThreshold    int  `json:"failure_threshold"`
	RecoveryThreshold   int  `json:"recovery_threshold"`
	HealthCheckTimeout  int  `json:"health_check_timeout"` // seconds

	// Retry configuration
	MaxRetries        int `json:"max_retries"`
	InitialRetryDelay int `json:"initial_retry_delay"` // milliseconds
	MaxRetryDelay     int `json:"max_retry_delay"`     // milliseconds

	// Circuit breaker configuration
	CircuitBreakerEnabled bool    `json:"circuit_breaker_enabled"`
	ErrorRateThreshold    float64 `json:"error_rate_threshold"`    // 0.0-1.0
	CircuitBreakerWindow  int     `json:"circuit_breaker_window"`  // seconds
	CircuitBreakerTimeout int     `json:"circuit_breaker_timeout"` // seconds
	HalfOpenRequests      int     `json:"half_open_requests"`

	// Dynamic weight configuration
	DynamicWeightEnabled bool `json:"dynamic_weight_enabled"`
	WeightUpdateInterval int  `json:"weight_update_interval"` // seconds

	// Logging configuration
	LogLevel string `json:"log_level"` // minimal, standard, detailed
}

// GetAllLoadBalancers handles GET /api/load-balancers
func GetAllLoadBalancers(c *gin.Context) {
	userID, role := getUserContext(c)
	var (
		loadBalancers []*database.LoadBalancer
		err           error
	)
	if isAdminRole(role) {
		loadBalancers, err = database.GetAllLoadBalancers()
	} else {
		loadBalancers, err = database.GetLoadBalancersByUser(userID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve load balancers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"load_balancers": loadBalancers,
	})
}

// GetLoadBalancer handles GET /api/load-balancers/:id
func GetLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && lb.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, lb)
}

// CreateLoadBalancer handles POST /api/load-balancers
func CreateLoadBalancer(c *gin.Context) {
	var req LoadBalancerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	userID, _ := getUserContext(c)

	// Validate strategy
	validStrategies := map[string]bool{
		"round_robin":       true,
		"random":            true,
		"weighted":          true,
		"least_connections": true,
	}
	if !validStrategies[req.Strategy] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid strategy. Must be one of: round_robin, random, weighted, least_connections",
		})
		return
	}

	// Validate config nodes
	if len(req.ConfigNodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "At least one config node is required",
		})
		return
	}

	// Validate weights for weighted strategy
	if req.Strategy == "weighted" {
		for _, node := range req.ConfigNodes {
			if node.Weight < 1 || node.Weight > 100 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Weight must be between 1 and 100",
				})
				return
			}
		}
	}

	lb := &database.LoadBalancer{
		Name:            req.Name,
		Description:     req.Description,
		UserID:          userID,
		Strategy:        req.Strategy,
		ConfigNodes:     req.ConfigNodes,
		Enabled:         req.Enabled,
		AnthropicAPIKey: req.AnthropicAPIKey,

		// Health check configuration
		HealthCheckEnabled:  req.HealthCheckEnabled,
		HealthCheckInterval: req.HealthCheckInterval,
		FailureThreshold:    req.FailureThreshold,
		RecoveryThreshold:   req.RecoveryThreshold,
		HealthCheckTimeout:  req.HealthCheckTimeout,

		// Retry configuration
		MaxRetries:        req.MaxRetries,
		InitialRetryDelay: req.InitialRetryDelay,
		MaxRetryDelay:     req.MaxRetryDelay,

		// Circuit breaker configuration
		CircuitBreakerEnabled: req.CircuitBreakerEnabled,
		ErrorRateThreshold:    req.ErrorRateThreshold,
		CircuitBreakerWindow:  req.CircuitBreakerWindow,
		CircuitBreakerTimeout: req.CircuitBreakerTimeout,
		HalfOpenRequests:      req.HalfOpenRequests,

		// Dynamic weight configuration
		DynamicWeightEnabled: req.DynamicWeightEnabled,
		WeightUpdateInterval: req.WeightUpdateInterval,

		// Logging configuration
		LogLevel: req.LogLevel,
	}

	if err := database.CreateLoadBalancer(lb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, lb)
}

// UpdateLoadBalancer handles PUT /api/load-balancers/:id
func UpdateLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	var req LoadBalancerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	// Check if load balancer exists
	existing, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && existing.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Validate strategy
	validStrategies := map[string]bool{
		"round_robin":       true,
		"random":            true,
		"weighted":          true,
		"least_connections": true,
	}
	if !validStrategies[req.Strategy] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid strategy",
		})
		return
	}

	// Update fields
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Strategy = req.Strategy
	existing.ConfigNodes = req.ConfigNodes
	existing.Enabled = req.Enabled

	// Update health check configuration
	existing.HealthCheckEnabled = req.HealthCheckEnabled
	existing.HealthCheckInterval = req.HealthCheckInterval
	existing.FailureThreshold = req.FailureThreshold
	existing.RecoveryThreshold = req.RecoveryThreshold
	existing.HealthCheckTimeout = req.HealthCheckTimeout

	// Update retry configuration
	existing.MaxRetries = req.MaxRetries
	existing.InitialRetryDelay = req.InitialRetryDelay
	existing.MaxRetryDelay = req.MaxRetryDelay

	// Update circuit breaker configuration
	existing.CircuitBreakerEnabled = req.CircuitBreakerEnabled
	existing.ErrorRateThreshold = req.ErrorRateThreshold
	existing.CircuitBreakerWindow = req.CircuitBreakerWindow
	existing.CircuitBreakerTimeout = req.CircuitBreakerTimeout
	existing.HalfOpenRequests = req.HalfOpenRequests

	// Update dynamic weight configuration
	existing.DynamicWeightEnabled = req.DynamicWeightEnabled
	existing.WeightUpdateInterval = req.WeightUpdateInterval

	// Update logging configuration
	existing.LogLevel = req.LogLevel

	if err := database.UpdateLoadBalancer(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update load balancer",
		})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// DeleteLoadBalancer handles DELETE /api/load-balancers/:id
func DeleteLoadBalancer(c *gin.Context) {
	id := c.Param("id")
	userID, role := getUserContext(c)
	existing, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}
	if !isAdminRole(role) && existing.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := database.DeleteLoadBalancer(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete load balancer",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Load balancer deleted successfully",
	})
}

// RenewLoadBalancerKey handles POST /api/load-balancers/:id/renew-key
func RenewLoadBalancerKey(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		CustomToken string `json:"custom_token"`
	}
	c.ShouldBindJSON(&req)

	userID, role := getUserContext(c)
	existing, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}
	if !isAdminRole(role) && existing.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	newKey, err := database.RenewLoadBalancerAPIKey(id, req.CustomToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"anthropic_api_key": newKey,
		"message":           "API key renewed successfully",
	})
}

// TestLoadBalancer handles POST /api/load-balancers/:id/test
func TestLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && lb.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Create selector
	selector, err := NewLoadBalancerSelector(lb)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Select a config
	config, err := selector.SelectConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":            "success",
		"message":           "Load balancer is working correctly",
		"selected_config":   config.Name,
		"config_id":         config.ID,
		"strategy":          lb.Strategy,
		"available_configs": selector.GetConfigCount(),
	})
}

// GetLoadBalancerStats handles GET /api/load-balancers/:id/stats
func GetLoadBalancerStats(c *gin.Context) {
	id := c.Param("id")

	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && lb.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Aggregate stats from all config nodes
	totalRequests := int64(0)
	successRequests := int64(0)
	errorRequests := int64(0)
	totalInputTokens := int64(0)
	totalOutputTokens := int64(0)
	totalTokens := int64(0)
	avgDurationMs := float64(0)
	configCount := 0

	for _, node := range lb.ConfigNodes {
		stats, err := database.GetConfigStats(node.ConfigID, 7) // Last 7 days
		if err != nil {
			continue
		}

		totalRequests += stats.TotalRequests
		successRequests += stats.SuccessRequests
		errorRequests += stats.ErrorRequests
		totalInputTokens += stats.TotalInputTokens
		totalOutputTokens += stats.TotalOutputTokens
		totalTokens += stats.TotalTokens
		avgDurationMs += stats.AvgDurationMs
		configCount++
	}

	if configCount > 0 {
		avgDurationMs = avgDurationMs / float64(configCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"load_balancer_id":    id,
		"total_requests":      totalRequests,
		"success_requests":    successRequests,
		"error_requests":      errorRequests,
		"total_input_tokens":  totalInputTokens,
		"total_output_tokens": totalOutputTokens,
		"total_tokens":        totalTokens,
		"avg_duration_ms":     avgDurationMs,
		"config_count":        len(lb.ConfigNodes),
	})
}
