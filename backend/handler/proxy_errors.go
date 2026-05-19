package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// GetProxyErrors returns recent proxy errors with optional filters
func (h *Handler) GetProxyErrors(c *gin.Context) {
	configID := c.Query("config_id")
	errorCategory := c.Query("error_category")
	model := c.Query("model")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	errors, err := database.GetProxyErrors(configID, errorCategory, model, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query proxy errors: " + err.Error()})
		return
	}
	if errors == nil {
		errors = []database.ProxyError{}
	}
	c.JSON(http.StatusOK, gin.H{"errors": errors, "count": len(errors)})
}

// GetProxyErrorStats returns error statistics grouped by category and config
func (h *Handler) GetProxyErrorStats(c *gin.Context) {
	configID := c.Query("config_id")
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 {
		hours = 24
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := database.GetProxyErrorStats(configID, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query error stats: " + err.Error()})
		return
	}
	if stats == nil {
		stats = []map[string]interface{}{}
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats, "since": since.UTC().Format(time.RFC3339)})
}

// CleanupProxyErrors removes old error records
func (h *Handler) CleanupProxyErrors(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days <= 0 {
		days = 30
	}

	deleted, err := database.CleanupOldProxyErrors(time.Duration(days) * 24 * time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup proxy errors: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted, "message": "Cleanup completed"})
}
