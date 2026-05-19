package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

type ccrConfig struct {
	LOG       bool           `json:"LOG"`
	PORT      int            `json:"PORT"`
	Providers []ccrProvider  `json:"Providers"`
	Router    ccrRouter      `json:"Router"`
}

type ccrProvider struct {
	Name        string          `json:"name"`
	APIBaseURL  string          `json:"api_base_url"`
	APIKey      string          `json:"api_key"`
	Models      []string        `json:"models"`
	Transformer *ccrTransformer `json:"transformer"`
}

type ccrTransformer struct {
	Use []string `json:"use"`
}

type ccrRouter struct {
	Default    string `json:"default"`
	Background string `json:"background"`
	Image      string `json:"image"`
}

func importCCRConfig() (int, error) {
	logger := utils.GetLogger()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".claude-code-router", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("CCR config not found at %s: %w", configPath, err)
	}

	var ccr ccrConfig
	if err := json.Unmarshal(data, &ccr); err != nil {
		return 0, fmt.Errorf("parse CCR config: %w", err)
	}

	logger.Info("Found %d providers in CCR config (port=%d)", len(ccr.Providers), ccr.PORT)

	imported := 0
	for _, provider := range ccr.Providers {
		if provider.APIKey == "" {
			logger.Warn("Skipping provider %q: no API key", provider.Name)
			continue
		}
		if provider.APIBaseURL == "" {
			logger.Warn("Skipping provider %q: no API base URL", provider.Name)
			continue
		}

		// Check if config already exists
		existingConfigs, err := database.GetAllAPIConfigs()
		if err == nil {
			alreadyExists := false
			for _, existing := range existingConfigs {
				if existing.Name == "CCR: "+provider.Name {
					alreadyExists = true
					break
				}
			}
			if alreadyExists {
				logger.Info("Provider %q already imported, skipping", provider.Name)
				continue
			}
		}

		primaryModel := ""
		if len(provider.Models) > 0 {
			primaryModel = provider.Models[0]
		}

		providerType := detectProviderType(provider)

		// Assign to the first admin user
		adminUserID := database.GetAdminUserID()

		apiConfig := &database.APIConfig{
			Name:            "CCR: " + provider.Name,
			Description:     fmt.Sprintf("Imported from CCR (type: %s)", providerType),
			UserID:          adminUserID,
			OpenAIAPIKey:    provider.APIKey,
			OpenAIBaseURL:   normalizeBaseURL(provider.APIBaseURL, providerType),
			BigModel:        primaryModel,
			MiddleModel:     primaryModel,
			SmallModel:      primaryModel,
			SupportedModels: provider.Models,
			MaxTokensLimit:  4096,
			RequestTimeout:  300,
			RetryCount:      3,
			Enabled:         true,
		}

		if err := database.CreateAPIConfig(apiConfig); err != nil {
			logger.Error("Failed to import provider %q: %v", provider.Name, err)
			continue
		}

		maskedKey := maskKey(provider.APIKey)
		logger.Info("Imported CCR provider %q → base_url=%s model=%s key=%s",
			provider.Name, apiConfig.OpenAIBaseURL, primaryModel, maskedKey)
		imported++
	}

	return imported, nil
}

func detectProviderType(p ccrProvider) string {
	if p.Transformer != nil {
		for _, t := range p.Transformer.Use {
			switch strings.ToLower(t) {
			case "gemini":
				return "gemini"
			case "openai":
				return "openai"
			case "anthropic":
				return "anthropic"
			}
		}
	}

	url := strings.ToLower(p.APIBaseURL)
	if strings.Contains(url, "generativelanguage.googleapis.com") {
		return "gemini"
	}
	if strings.Contains(url, "api.mistral.ai") {
		return "mistral"
	}
	if strings.Contains(url, "api.openai.com") {
		return "openai"
	}
	return "openai-compatible"
}

func normalizeBaseURL(rawURL string, providerType string) string {
	switch providerType {
	case "gemini":
		if strings.Contains(rawURL, "generativelanguage.googleapis.com") {
			return "https://generativelanguage.googleapis.com/v1beta/openai/"
		}
		return rawURL
	case "mistral":
		return strings.Replace(rawURL, "/chat/completions", "", 1)
	default:
		return rawURL
	}
}

func maskKey(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:8] + "****" + key[len(key)-4:]
}
