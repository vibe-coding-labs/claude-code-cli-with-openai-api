package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
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

// LoadConfigs loads configurations from file
func (cm *ConfigManager) LoadConfigs() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(cm.filePath)
	if configDir != "." && configDir != "" {
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}

	// Check if file exists
	if _, err := os.Stat(cm.filePath); os.IsNotExist(err) {
		// File doesn't exist, initialize with empty config
		cm.configs = make(map[string]*models.APIConfig)
		return nil
	}

	// Read file
	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	var configList models.APIConfigList
	if err := json.Unmarshal(data, &configList); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Load configurations
	cm.configs = make(map[string]*models.APIConfig)
	for i := range configList.Configs {
		config := configList.Configs[i]
		cm.configs[config.ID] = &config
	}
	cm.defaultConfigID = configList.DefaultConfigID

	// If no default config and we have configs, set the first one as default
	if cm.defaultConfigID == "" && len(cm.configs) > 0 {
		for id := range cm.configs {
			cm.defaultConfigID = id
			break
		}
	}

	return nil
}

// SaveConfigs saves configurations to file
func (cm *ConfigManager) SaveConfigs() error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	configList := models.APIConfigList{
		Configs:        make([]models.APIConfig, 0, len(cm.configs)),
		DefaultConfigID: cm.defaultConfigID,
	}

	for _, config := range cm.configs {
		configList.Configs = append(configList.Configs, *config)
	}

	data, err := json.MarshalIndent(configList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(cm.filePath)
	if configDir != "." && configDir != "" {
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}

	if err := os.WriteFile(cm.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// CreateConfig creates a new configuration
func (cm *ConfigManager) CreateConfig(req *models.APIConfigRequest) (*models.APIConfig, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Set defaults
	if req.OpenAIBaseURL == "" {
		req.OpenAIBaseURL = "https://api.openai.com/v1"
	}
	if req.BigModel == "" {
		req.BigModel = "gpt-4o"
	}
	if req.MiddleModel == "" {
		req.MiddleModel = req.BigModel
	}
	if req.SmallModel == "" {
		req.SmallModel = "gpt-4o-mini"
	}
	if req.MaxTokensLimit == 0 {
		req.MaxTokensLimit = 4096
	}
	if req.MinTokensLimit == 0 {
		req.MinTokensLimit = 100
	}
	if req.RequestTimeout == 0 {
		req.RequestTimeout = 90
	}
	if req.CustomHeaders == nil {
		req.CustomHeaders = make(map[string]string)
	}

	// Create new config
	now := time.Now()
	config := &models.APIConfig{
		ID:             uuid.New().String(),
		Name:           req.Name,
		Description:    req.Description,
		OpenAIAPIKey:   req.OpenAIAPIKey,
		OpenAIBaseURL:  req.OpenAIBaseURL,
		AzureAPIVersion: req.AzureAPIVersion,
		AnthropicAPIKey: req.AnthropicAPIKey,
		BigModel:       req.BigModel,
		MiddleModel:    req.MiddleModel,
		SmallModel:     req.SmallModel,
		MaxTokensLimit: req.MaxTokensLimit,
		MinTokensLimit: req.MinTokensLimit,
		RequestTimeout: req.RequestTimeout,
		CustomHeaders:  req.CustomHeaders,
		Enabled:        req.Enabled,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	cm.configs[config.ID] = config

	// Set as default if it's the first config
	if cm.defaultConfigID == "" {
		cm.defaultConfigID = config.ID
	}

	if err := cm.SaveConfigs(); err != nil {
		delete(cm.configs, config.ID)
		return nil, err
	}

	return config, nil
}

// GetConfig retrieves a configuration by ID
func (cm *ConfigManager) GetConfig(id string) (*models.APIConfig, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	config, exists := cm.configs[id]
	if !exists {
		return nil, fmt.Errorf("config not found: %s", id)
	}

	return config, nil
}

// GetAllConfigs retrieves all configurations
func (cm *ConfigManager) GetAllConfigs() []*models.APIConfig {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	configs := make([]*models.APIConfig, 0, len(cm.configs))
	for _, config := range cm.configs {
		configs = append(configs, config)
	}

	return configs
}

// UpdateConfig updates an existing configuration
func (cm *ConfigManager) UpdateConfig(id string, req *models.APIConfigRequest) (*models.APIConfig, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	config, exists := cm.configs[id]
	if !exists {
		return nil, fmt.Errorf("config not found: %s", id)
	}

	// Update fields
	config.Name = req.Name
	config.Description = req.Description
	config.OpenAIAPIKey = req.OpenAIAPIKey
	if req.OpenAIBaseURL != "" {
		config.OpenAIBaseURL = req.OpenAIBaseURL
	}
	config.AzureAPIVersion = req.AzureAPIVersion
	config.AnthropicAPIKey = req.AnthropicAPIKey
	if req.BigModel != "" {
		config.BigModel = req.BigModel
	}
	if req.MiddleModel != "" {
		config.MiddleModel = req.MiddleModel
	}
	if req.SmallModel != "" {
		config.SmallModel = req.SmallModel
	}
	if req.MaxTokensLimit > 0 {
		config.MaxTokensLimit = req.MaxTokensLimit
	}
	if req.MinTokensLimit > 0 {
		config.MinTokensLimit = req.MinTokensLimit
	}
	if req.RequestTimeout > 0 {
		config.RequestTimeout = req.RequestTimeout
	}
	if req.CustomHeaders != nil {
		config.CustomHeaders = req.CustomHeaders
	}
	config.Enabled = req.Enabled
	config.UpdatedAt = time.Now()

	if err := cm.SaveConfigs(); err != nil {
		return nil, err
	}

	return config, nil
}

// DeleteConfig deletes a configuration
func (cm *ConfigManager) DeleteConfig(id string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if _, exists := cm.configs[id]; !exists {
		return fmt.Errorf("config not found: %s", id)
	}

	delete(cm.configs, id)

	// If deleted config was default, set another as default
	if cm.defaultConfigID == id {
		cm.defaultConfigID = ""
		for configID := range cm.configs {
			cm.defaultConfigID = configID
			break
		}
	}

	return cm.SaveConfigs()
}

// SetDefaultConfig sets the default configuration
func (cm *ConfigManager) SetDefaultConfig(id string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if _, exists := cm.configs[id]; !exists {
		return fmt.Errorf("config not found: %s", id)
	}

	cm.defaultConfigID = id
	return cm.SaveConfigs()
}

// GetDefaultConfig returns the default configuration
func (cm *ConfigManager) GetDefaultConfig() (*models.APIConfig, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if cm.defaultConfigID == "" {
		return nil, fmt.Errorf("no default config set")
	}

	config, exists := cm.configs[cm.defaultConfigID]
	if !exists {
		return nil, fmt.Errorf("default config not found: %s", cm.defaultConfigID)
	}

	return config, nil
}

// GetConfigByAnthropicKey returns a configuration by Anthropic API key
func (cm *ConfigManager) GetConfigByAnthropicKey(anthropicKey string) (*models.APIConfig, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for _, config := range cm.configs {
		if config.AnthropicAPIKey == anthropicKey && config.Enabled {
			return config, nil
		}
	}

	// If no match found, return default config
	if cm.defaultConfigID != "" {
		if config, exists := cm.configs[cm.defaultConfigID]; exists && config.Enabled {
			return config, nil
		}
	}

	return nil, fmt.Errorf("no matching config found")
}

// UpdateTestStatus updates the test status of a configuration
func (cm *ConfigManager) UpdateTestStatus(id string, status string, errorMsg string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	config, exists := cm.configs[id]
	if !exists {
		return fmt.Errorf("config not found: %s", id)
	}

	now := time.Now()
	config.LastTestedAt = &now
	config.LastTestStatus = status
	config.LastTestError = errorMsg

	return cm.SaveConfigs()
}

// ToConfig converts APIConfig to Config for backward compatibility
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

