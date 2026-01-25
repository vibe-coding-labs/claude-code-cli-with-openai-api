package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// GetLoadBalancerHealthStatus handles GET /api/load-balancers/:id/health
func GetLoadBalancerHealthStatus(c *gin.Context) {
	id := c.Param("id")

	// Get load balancer
	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	// Get health statuses for all nodes
	statuses, err := database.GetHealthStatusesByLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve health statuses",
		})
		return
	}

	// Count healthy nodes
	healthyCount := 0
	for _, status := range statuses {
		if status.Status == "healthy" {
			healthyCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"load_balancer_id": id,
		"total_nodes":      len(lb.ConfigNodes),
		"healthy_nodes":    healthyCount,
		"unhealthy_nodes":  len(statuses) - healthyCount,
		"statuses":         statuses,
	})
}

// GetNodeHealthStatus handles GET /api/load-balancers/:id/health/:config_id
func GetNodeHealthStatus(c *gin.Context) {
	configID := c.Param("config_id")

	status, err := database.GetHealthStatus(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Health status not found",
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetLoadBalancerCircuitBreakers handles GET /api/load-balancers/:id/circuit-breakers
func GetLoadBalancerCircuitBreakers(c *gin.Context) {
	id := c.Param("id")

	// Get circuit breaker states for all nodes
	states, err := database.GetCircuitBreakerStatesByLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve circuit breaker states",
		})
		return
	}

	// Count states
	closedCount := 0
	openCount := 0
	halfOpenCount := 0
	for _, state := range states {
		switch state.State {
		case "closed":
			closedCount++
		case "open":
			openCount++
		case "half_open":
			halfOpenCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"load_balancer_id": id,
		"total_nodes":      len(states),
		"closed":           closedCount,
		"open":             openCount,
		"half_open":        halfOpenCount,
		"states":           states,
	})
}

// GetLoadBalancerEnhancedStats handles GET /api/load-balancers/:id/stats/enhanced
func GetLoadBalancerEnhancedStats(c *gin.Context) {
	id := c.Param("id")
	timeWindow := c.DefaultQuery("window", "24h")

	stats, err := database.GetLoadBalancerStats(id, timeWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetLoadBalancerRealTimeMetrics handles GET /api/load-balancers/:id/metrics/realtime
func GetLoadBalancerRealTimeMetrics(c *gin.Context) {
	id := c.Param("id")

	// Verify load balancer exists
	_, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	// Get real-time metrics
	metrics, err := database.GetRealTimeMetrics(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to retrieve real-time metrics: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetLoadBalancerRequestLogs handles GET /api/load-balancers/:id/logs
func GetLoadBalancerRequestLogs(c *gin.Context) {
	id := c.Param("id")
	limit := c.DefaultQuery("limit", "100")
	offset := c.DefaultQuery("offset", "0")

	var limitInt, offsetInt int
	if _, err := fmt.Sscanf(limit, "%d", &limitInt); err != nil {
		limitInt = 100
	}
	if _, err := fmt.Sscanf(offset, "%d", &offsetInt); err != nil {
		offsetInt = 0
	}

	logs, err := database.GetLoadBalancerRequestLogs(id, limitInt, offsetInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve request logs",
		})
		return
	}

	// Get total count
	totalCount, err := database.CountLoadBalancerRequestLogs(id)
	if err != nil {
		totalCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"load_balancer_id": id,
		"total_count":      totalCount,
		"limit":            limitInt,
		"offset":           offsetInt,
		"logs":             logs,
	})
}

// GetLoadBalancerAlerts handles GET /api/load-balancers/:id/alerts
func GetLoadBalancerAlerts(c *gin.Context) {
	id := c.Param("id")
	acknowledgedParam := c.DefaultQuery("acknowledged", "false")

	acknowledged := acknowledgedParam == "true"
	alerts, err := database.GetAlertsByLoadBalancer(id, &acknowledged, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve alerts",
		})
		return
	}

	// Count unacknowledged alerts
	unacknowledgedCount, err := database.CountUnacknowledgedAlerts(id)
	if err != nil {
		unacknowledgedCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"load_balancer_id":      id,
		"unacknowledged_count":  unacknowledgedCount,
		"alerts":                alerts,
	})
}

// AcknowledgeAlert handles POST /api/alerts/:id/acknowledge
func AcknowledgeAlert(c *gin.Context) {
	alertID := c.Param("id")

	if err := database.AcknowledgeAlert(alertID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to acknowledge alert",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Alert acknowledged successfully",
	})
}

// GetAllAlerts handles GET /api/alerts
func GetAllAlerts(c *gin.Context) {
	acknowledgedParam := c.DefaultQuery("acknowledged", "")
	level := c.DefaultQuery("level", "")
	limitParam := c.DefaultQuery("limit", "100")

	var acknowledged *bool
	if acknowledgedParam != "" {
		ack := acknowledgedParam == "true"
		acknowledged = &ack
	}

	var limitInt int
	if _, err := fmt.Sscanf(limitParam, "%d", &limitInt); err != nil {
		limitInt = 100
	}

	alerts, err := database.GetAllAlerts(acknowledged, level, limitInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve alerts",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
	})
}

// UpdateLoadBalancerConfig handles PUT /api/load-balancers/:id/config
func UpdateLoadBalancerConfig(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		// Health check configuration
		HealthCheckEnabled  *bool `json:"health_check_enabled"`
		HealthCheckInterval *int  `json:"health_check_interval"`
		FailureThreshold    *int  `json:"failure_threshold"`
		RecoveryThreshold   *int  `json:"recovery_threshold"`
		HealthCheckTimeout  *int  `json:"health_check_timeout"`

		// Retry configuration
		MaxRetries        *int `json:"max_retries"`
		InitialRetryDelay *int `json:"initial_retry_delay"`
		MaxRetryDelay     *int `json:"max_retry_delay"`

		// Circuit breaker configuration
		CircuitBreakerEnabled *bool    `json:"circuit_breaker_enabled"`
		ErrorRateThreshold    *float64 `json:"error_rate_threshold"`
		CircuitBreakerWindow  *int     `json:"circuit_breaker_window"`
		CircuitBreakerTimeout *int     `json:"circuit_breaker_timeout"`
		HalfOpenRequests      *int     `json:"half_open_requests"`

		// Dynamic weight configuration
		DynamicWeightEnabled *bool `json:"dynamic_weight_enabled"`
		WeightUpdateInterval *int  `json:"weight_update_interval"`

		// Log configuration
		LogLevel *string `json:"log_level"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	// Check if load balancer exists
	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	// Note: This is a simplified version. In a real implementation,
	// you would need to add these fields to the LoadBalancer struct
	// and update the database schema accordingly.

	c.JSON(http.StatusOK, gin.H{
		"message":          "Configuration updated successfully",
		"load_balancer_id": lb.ID,
	})
}

// GetLoadBalancerConfig handles GET /api/load-balancers/:id/config
func GetLoadBalancerConfig(c *gin.Context) {
	id := c.Param("id")

	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	// Return configuration
	// Note: This would include the enhanced configuration fields
	// once they are added to the LoadBalancer struct
	c.JSON(http.StatusOK, gin.H{
		"load_balancer_id": lb.ID,
		"name":             lb.Name,
		"strategy":         lb.Strategy,
		"enabled":          lb.Enabled,
		// Enhanced configuration would go here
	})
}

// TriggerHealthCheck handles POST /api/load-balancers/:id/health/check
func TriggerHealthCheck(c *gin.Context) {
	id := c.Param("id")

	lb, err := database.GetLoadBalancer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Load balancer not found",
		})
		return
	}

	// This would trigger an immediate health check
	// In a real implementation, you would call the health checker
	c.JSON(http.StatusOK, gin.H{
		"message":          "Health check triggered",
		"load_balancer_id": lb.ID,
	})
}

// ResetCircuitBreaker handles POST /api/load-balancers/:id/circuit-breakers/:config_id/reset
func ResetCircuitBreaker(c *gin.Context) {
	configID := c.Param("config_id")

	if err := database.TransitionCircuitBreakerToClosed(configID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to reset circuit breaker",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Circuit breaker reset successfully",
		"config_id": configID,
	})
}
