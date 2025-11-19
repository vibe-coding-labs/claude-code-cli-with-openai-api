package handler

import (
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

// ConfigHandler handles configuration management API endpoints
type ConfigHandler struct {
	manager *config.ConfigManager
}

// NewConfigHandler creates a new ConfigHandler
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{
		manager: config.GetConfigManager(),
	}
}
