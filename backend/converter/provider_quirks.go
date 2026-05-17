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
	ProviderGemini
	ProviderGoogle
	ProviderAnthropic
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
	case strings.Contains(lower, "generativelanguage.googleapis.com") || strings.Contains(lower, "gemini"):
		return ProviderGemini
	case strings.Contains(lower, "googleapis.com"):
		return ProviderGoogle
	case strings.Contains(lower, "anthropic.com") || strings.Contains(lower, "api.anthropic"):
		return ProviderAnthropic
	case strings.Contains(lower, "openai.com") || strings.Contains(lower, "api.openai"):
		return ProviderOpenAI
	default:
		return ProviderUnknown
	}
}

// IsGeminiProvider returns true if the base URL points to a Gemini/Google AI endpoint.
func IsGeminiProvider(baseURL string) bool {
	p := DetectProvider(baseURL)
	return p == ProviderGemini || p == ProviderGoogle
}

// IsAnthropicProvider returns true if the base URL points to Anthropic directly.
func IsAnthropicProvider(baseURL string) bool {
	return DetectProvider(baseURL) == ProviderAnthropic
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

// GetMaxTokensCap returns the maximum tokens allowed for the given provider.
// Returns 0 if no cap applies.
func GetMaxTokensCap(provider ProviderType) int {
	switch provider {
	case ProviderOpenAI, ProviderAzureOpenAI:
		return 16384
	case ProviderDeepSeek:
		return 16384
	default:
		return 0
	}
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

var geminiUnsupportedSchemaFields = []string{
	"additionalProperties",
	"default",
	"$schema",
	"deprecated",
	"examples",
}

// CleanSchemaForGemini recursively removes JSON Schema fields that Gemini doesn't support.
// 参考 claude-code-proxy 的 clean_schema 实现.
func CleanSchemaForGemini(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	cleaned := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		if isUnsupportedSchemaField(k) {
			continue
		}
		cleaned[k] = cleanSchemaValue(v)
	}
	if props, ok := cleaned["properties"].(map[string]interface{}); ok {
		for propName, propVal := range props {
			if propMap, ok := propVal.(map[string]interface{}); ok {
				props[propName] = CleanSchemaForGemini(propMap)
			}
		}
	}
	if items, ok := cleaned["items"].(map[string]interface{}); ok {
		cleaned["items"] = CleanSchemaForGemini(items)
	}
	for _, key := range []string{"anyOf", "allOf", "oneOf"} {
		if arr, ok := cleaned[key].([]interface{}); ok {
			for i, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arr[i] = CleanSchemaForGemini(itemMap)
				}
			}
		}
	}
	return cleaned
}

func isUnsupportedSchemaField(field string) bool {
	for _, f := range geminiUnsupportedSchemaFields {
		if field == f {
			return true
		}
	}
	return false
}

func cleanSchemaValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return CleanSchemaForGemini(val)
	case []interface{}:
		cleaned := make([]interface{}, len(val))
		for i, item := range val {
			cleaned[i] = cleanSchemaValue(item)
		}
		return cleaned
	default:
		return v
	}
}
