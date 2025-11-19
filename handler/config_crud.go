package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ListConfigs returns all configurations
func (h *ConfigHandler) ListConfigs(c *gin.Context) {
	configs := h.manager.GetAllConfigs()

	// Remove sensitive information for list view
	safeConfigs := make([]models.APIConfig, 0, len(configs))
	for _, cfg := range configs {
		safeCfg := *cfg
		// Mask API keys
		if len(safeCfg.OpenAIAPIKey) > 8 {
			safeCfg.OpenAIAPIKey = safeCfg.OpenAIAPIKey[:4] + "..." + safeCfg.OpenAIAPIKey[len(safeCfg.OpenAIAPIKey)-4:]
		}
		if len(safeCfg.AnthropicAPIKey) > 8 {
			safeCfg.AnthropicAPIKey = safeCfg.AnthropicAPIKey[:4] + "..." + safeCfg.AnthropicAPIKey[len(safeCfg.AnthropicAPIKey)-4:]
		}
		safeConfigs = append(safeConfigs, safeCfg)
	}

	defaultID, _ := h.manager.GetDefaultConfig()
	defaultIDStr := ""
	if defaultID != nil {
		defaultIDStr = defaultID.ID
	}

	c.JSON(http.StatusOK, gin.H{
		"configs":        safeConfigs,
		"default_config": defaultIDStr,
		"total":          len(safeConfigs),
	})
}

// GetConfig returns a specific configuration
func (h *ConfigHandler) GetConfig(c *gin.Context) {
	id := c.Param("id")

	cfg, err := h.manager.GetConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// CreateConfig creates a new configuration
func (h *ConfigHandler) CreateConfig(c *gin.Context) {
	var req models.APIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	cfg, err := h.manager.CreateConfig(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, cfg)
}

// UpdateConfig updates an existing configuration
func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	id := c.Param("id")

	var req models.APIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	cfg, err := h.manager.UpdateConfig(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// DeleteConfig deletes a configuration
func (h *ConfigHandler) DeleteConfig(c *gin.Context) {
	id := c.Param("id")

	err := h.manager.DeleteConfig(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Config deleted successfully",
	})
}

// SetDefaultConfig sets a configuration as default
func (h *ConfigHandler) SetDefaultConfig(c *gin.Context) {
	id := c.Param("id")

	err := h.manager.SetDefaultConfig(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to set default config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Default config set successfully",
	})
}
