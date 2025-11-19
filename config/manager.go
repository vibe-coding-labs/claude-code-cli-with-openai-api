package config

import (
	"sync"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

var (
	configManager     *ConfigManager
	configManagerOnce sync.Once
	configFilePath    = "configs.json"
)

// ConfigManager manages multiple API configurations
type ConfigManager struct {
	configs         map[string]*models.APIConfig
	defaultConfigID string
	mutex           sync.RWMutex
	filePath        string
}

// GetConfigManager returns the singleton ConfigManager instance
func GetConfigManager(filePath ...string) *ConfigManager {
	configManagerOnce.Do(func() {
		path := configFilePath
		if len(filePath) > 0 && filePath[0] != "" {
			path = filePath[0]
		}
		configManager = &ConfigManager{
			configs:  make(map[string]*models.APIConfig),
			filePath: path,
		}
		configManager.LoadConfigs()
	})
	return configManager
}

// ToConfigFromAPIConfig converts APIConfig to Config for backward compatibility
func ToConfigFromAPIConfig(ac *models.APIConfig) *Config {
	return &Config{
		OpenAIAPIKey:    ac.OpenAIAPIKey,
		AnthropicAPIKey: ac.AnthropicAPIKey,
		OpenAIBaseURL:   ac.OpenAIBaseURL,
		AzureAPIVersion: ac.AzureAPIVersion,
		Host:            "0.0.0.0",
		Port:            10086,
		LogLevel:        "INFO",
		MaxTokensLimit:  ac.MaxTokensLimit,
		MinTokensLimit:  ac.MinTokensLimit,
		RequestTimeout:  ac.RequestTimeout,
		BigModel:        ac.BigModel,
		MiddleModel:     ac.MiddleModel,
		SmallModel:      ac.SmallModel,
		CustomHeaders:   ac.CustomHeaders,
	}
}
