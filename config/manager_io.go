package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// LoadConfigs loads configurations from file
func (cm *ConfigManager) LoadConfigs() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Create config directory if it doesn't exist
	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(cm.filePath); os.IsNotExist(err) {
		// File doesn't exist, initialize with empty configs
		cm.configs = make(map[string]*models.APIConfig)
		return nil
	}

	// Read file
	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	var fileData struct {
		Configs         map[string]*models.APIConfig `json:"configs"`
		DefaultConfigID string                       `json:"default_config_id"`
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	cm.configs = fileData.Configs
	if cm.configs == nil {
		cm.configs = make(map[string]*models.APIConfig)
	}
	cm.defaultConfigID = fileData.DefaultConfigID

	return nil
}

// SaveConfigs saves configurations to file
func (cm *ConfigManager) SaveConfigs() error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Prepare data
	fileData := struct {
		Configs         map[string]*models.APIConfig `json:"configs"`
		DefaultConfigID string                       `json:"default_config_id"`
	}{
		Configs:         cm.configs,
		DefaultConfigID: cm.defaultConfigID,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(cm.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
