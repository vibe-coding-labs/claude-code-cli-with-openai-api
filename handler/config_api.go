package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// RenewConfigAPIKey renews the Anthropic API key for a config
func (h *Handler) RenewConfigAPIKey(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		CustomToken string `json:"custom_token"`
	}

	// Parse optional custom token
	_ = c.ShouldBindJSON(&req)

	// Check if config exists
	_, err := database.GetAPIConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found",
		})
		return
	}

	// Renew API key (with optional custom token)
	newAPIKey, err := database.RenewAnthropicAPIKey(id, req.CustomToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"new_api_key": newAPIKey,
		"message":     "API key renewed successfully",
	})
}

// GetConfigLogs retrieves logs for a config with filtering, sorting, and pagination
func (h *Handler) GetConfigLogs(c *gin.Context) {
	configID := c.Param("id")
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Parse query parameters
	params := database.LogsQueryParams{
		ConfigID:  configID,
		Status:    c.Query("status"),
		Model:     c.Query("model"),
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
		Search:    c.Query("search"),
	}
	if !isAdminRole(role) {
		params.UserID = userID
	}

	// Parse page
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	params.Page = page

	// Parse page size
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	params.PageSize = pageSize

	// Get logs
	result, err := database.GetLogsWithFilters(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// DeleteConfigLogs deletes all logs for a config
func (h *Handler) DeleteConfigLogs(c *gin.Context) {
	configID := c.Param("id")

	// Check if config exists
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Delete logs
	if err := database.DeleteConfigLogs(configID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Logs deleted successfully",
	})
}

// GetLogDetail retrieves detailed information for a single log
func (h *Handler) GetLogDetail(c *gin.Context) {
	logIDStr := c.Param("log_id")
	logID, err := strconv.ParseInt(logIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid log ID",
		})
		return
	}

	log, err := database.GetLogByID(logID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && log.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetAvailableModels returns available models for filtering
func (h *Handler) GetAvailableModels(c *gin.Context) {
	configID := c.Param("id")
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found"})
		return
	}
	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	models, err := database.GetAvailableModels(configID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get models: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
	})
}

// GetHistoricalModels returns all unique models from all configs and logs
// This is used by the frontend ModelSelector component to provide autocomplete suggestions
func (h *Handler) GetHistoricalModels(c *gin.Context) {
	models, err := database.GetAllHistoricalModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get historical models: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
	})
}
