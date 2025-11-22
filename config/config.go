package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAIAPIKey    string
	OpenAIBaseURL   string
	BigModel        string
	MiddleModel     string
	SmallModel      string
	SupportedModels []string
	MaxTokensLimit  int
	RequestTimeout  int
	RetryCount      int
	AnthropicAPIKey string
	AzureAPIVersion string
	Host            string
	Port            int
	LogLevel        string
	MinTokensLimit  int
	CustomHeaders   map[string]string
}

var GlobalConfig *Config

func LoadConfig() (*Config, error) {
	// Try to load .env file (optional)
	_ = godotenv.Load()

	// OPENAI_API_KEY is optional since configs are managed through UI
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")

	config := &Config{
		OpenAIAPIKey:    openAIAPIKey,
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIBaseURL:   getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		AzureAPIVersion: os.Getenv("AZURE_API_VERSION"),
		Host:            getEnvOrDefault("HOST", "0.0.0.0"),
		// ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
		Port:           getEnvAsInt("PORT", 54988),
		LogLevel:       getEnvOrDefault("LOG_LEVEL", "INFO"),
		MaxTokensLimit: getEnvAsInt("MAX_TOKENS_LIMIT", 4096),
		MinTokensLimit: getEnvAsInt("MIN_TOKENS_LIMIT", 100),
		RequestTimeout: getEnvAsInt("REQUEST_TIMEOUT", 300),
		RetryCount:     getEnvAsInt("RETRY_COUNT", 10),
		BigModel:       getEnvOrDefault("BIG_MODEL", "gpt-4o"),
		SmallModel:     getEnvOrDefault("SMALL_MODEL", "gpt-4o-mini"),
		CustomHeaders:  make(map[string]string),
	}

	// Set middle model to big model if not specified
	middleModel := os.Getenv("MIDDLE_MODEL")
	if middleModel == "" {
		config.MiddleModel = config.BigModel
	} else {
		config.MiddleModel = middleModel
	}

	// Load custom headers
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CUSTOM_HEADER_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				headerName := strings.TrimPrefix(parts[0], "CUSTOM_HEADER_")
				headerName = strings.ReplaceAll(headerName, "_", "-")
				config.CustomHeaders[headerName] = parts[1]
			}
		}
	}

	if config.AnthropicAPIKey == "" {
		fmt.Println("Warning: ANTHROPIC_API_KEY not set. Client API key validation will be disabled.")
	}

	GlobalConfig = config
	return config, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func (c *Config) ValidateAPIKey() bool {
	if c.OpenAIAPIKey == "" {
		return false
	}
	// Basic format check for OpenAI API keys
	if !strings.HasPrefix(c.OpenAIAPIKey, "sk-") {
		return false
	}
	return true
}

func (c *Config) ValidateClientAPIKey(clientAPIKey string) bool {
	// If no ANTHROPIC_API_KEY is set in environment, skip validation
	if c.AnthropicAPIKey == "" {
		return true
	}
	// Check if the client's API key matches the expected value
	return clientAPIKey == c.AnthropicAPIKey
}
