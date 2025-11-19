package config

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// CreateConfig creates a new API configuration
func (cm *ConfigManager) CreateConfig(req *models.APIConfigRequest) (*models.APIConfig, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Generate ID
	id := uuid.New().String()

	// Create config
	config := &models.APIConfig{
		ID:              id,
		Name:            req.Name,
		Description:     req.Description,
		OpenAIAPIKey:    req.OpenAIAPIKey,
		OpenAIBaseURL:   req.OpenAIBaseURL,
		BigModel:        req.BigModel,
		MiddleModel:     req.MiddleModel,
		SmallModel:      req.SmallModel,
		MaxTokensLimit:  req.MaxTokensLimit,
		RequestTimeout:  req.RequestTimeout,
		AnthropicAPIKey: req.AnthropicAPIKey,
		Enabled:         true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Validate
	if config.Name == "" {
		return nil, fmt.Errorf("config name is required")
	}
	if config.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	if config.OpenAIBaseURL == "" {
		return nil, fmt.Errorf("OpenAI base URL is required")
	}

	// Store
	cm.configs[id] = config

	// Set as default if it's the first config
	if len(cm.configs) == 1 {
		cm.defaultConfigID = id
	}

	// Save to file
	if err := cm.SaveConfigs(); err != nil {
		return nil, fmt.Errorf("failed to save configs: %w", err)
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

// GetAllConfigs returns all configurations
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
	if req.Name != "" {
		config.Name = req.Name
	}
	if req.Description != "" {
		config.Description = req.Description
	}
	if req.OpenAIAPIKey != "" {
		config.OpenAIAPIKey = req.OpenAIAPIKey
	}
	if req.OpenAIBaseURL != "" {
		config.OpenAIBaseURL = req.OpenAIBaseURL
	}
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
	if req.RequestTimeout > 0 {
		config.RequestTimeout = req.RequestTimeout
	}
	if req.AnthropicAPIKey != "" {
		config.AnthropicAPIKey = req.AnthropicAPIKey
	}

	config.UpdatedAt = time.Now()

	// Save to file
	if err := cm.SaveConfigs(); err != nil {
		return nil, fmt.Errorf("failed to save configs: %w", err)
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

	// Don't allow deleting the default config
	if id == cm.defaultConfigID {
		return fmt.Errorf("cannot delete default config")
	}

	delete(cm.configs, id)

	// Save to file
	if err := cm.SaveConfigs(); err != nil {
		return fmt.Errorf("failed to save configs: %w", err)
	}

	return nil
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

// GetConfigByAnthropicKey finds a config by Anthropic API key
func (cm *ConfigManager) GetConfigByAnthropicKey(anthropicKey string) (*models.APIConfig, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for _, config := range cm.configs {
		if config.AnthropicAPIKey == anthropicKey {
			return config, nil
		}
	}

	return nil, fmt.Errorf("config not found with given Anthropic API key")
}

// UpdateTestStatus updates the test status of a configuration
func (cm *ConfigManager) UpdateTestStatus(id string, status string, errorMsg string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	config, exists := cm.configs[id]
	if !exists {
		return fmt.Errorf("config not found: %s", id)
	}

	config.UpdatedAt = time.Now()

	return cm.SaveConfigs()
}
