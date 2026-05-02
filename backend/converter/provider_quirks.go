package converter

import "strings"

// ProviderType identifies the upstream API provider.
type ProviderType int

const (
	ProviderOpenAI      ProviderType = iota
	ProviderAzureOpenAI
	ProviderDeepSeek
	ProviderOllama
	ProviderOpenRouter
	ProviderMistral
	ProviderUnknown
)

// DetectProvider determines the provider type from the base URL.
func DetectProvider(baseURL string) ProviderType {
	lower := strings.ToLower(baseURL)
	switch {
	case strings.Contains(lower, "azure.com") || strings.Contains(lower, "openai.azure"):
		return ProviderAzureOpenAI
	case strings.Contains(lower, "deepseek"):
		return ProviderDeepSeek
	case strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1"):
		return ProviderOllama
	case strings.Contains(lower, "openrouter"):
		return ProviderOpenRouter
	case strings.Contains(lower, "mistral"):
		return ProviderMistral
	case strings.Contains(lower, "openai.com") || strings.Contains(lower, "api.openai"):
		return ProviderOpenAI
	default:
		return ProviderUnknown
	}
}

// SupportsFunctionCalling returns true if the provider supports native function calling.
func SupportsFunctionCalling(provider ProviderType) bool {
	switch provider {
	case ProviderOllama:
		return false
	default:
		return true
	}
}

// SupportsStopSequences returns true if the provider supports stop sequences.
func SupportsStopSequences(provider ProviderType) bool {
	switch provider {
	case ProviderOllama:
		return false
	default:
		return true
	}
}

// SupportsMaxTokens returns the correct field name for token limit.
func SupportsMaxTokens(provider ProviderType, model string) string {
	if provider == ProviderOpenAI || provider == ProviderAzureOpenAI {
		lower := strings.ToLower(model)
		if strings.Contains(lower, "o1") || strings.Contains(lower, "o3") ||
			strings.Contains(lower, "gpt-5") || strings.Contains(lower, "gpt-4.1") {
			return "max_completion_tokens"
		}
	}
	return "max_tokens"
}

// GetAuthHeader returns the auth header name and format for the provider.
func GetAuthHeader(provider ProviderType) (headerName string, format string) {
	switch provider {
	case ProviderAzureOpenAI:
		return "api-key", "%s"
	default:
		return "Authorization", "Bearer %s"
	}
}
