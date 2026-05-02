package converter

import "testing"

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		url      string
		expected ProviderType
	}{
		{"https://api.openai.com/v1", ProviderOpenAI},
		{"https://myresource.openai.azure.com/openai/", ProviderAzureOpenAI},
		{"https://api.deepseek.com/v1", ProviderDeepSeek},
		{"http://localhost:11434/v1", ProviderOllama},
		{"https://openrouter.ai/api/v1", ProviderOpenRouter},
		{"https://api.mistral.ai/v1", ProviderMistral},
		{"https://unknown.example.com/v1", ProviderUnknown},
	}
	for _, tt := range tests {
		if got := DetectProvider(tt.url); got != tt.expected {
			t.Errorf("DetectProvider(%q) = %v, want %v", tt.url, got, tt.expected)
		}
	}
}

func TestSupportsFunctionCalling(t *testing.T) {
	if !SupportsFunctionCalling(ProviderOpenAI) {
		t.Error("OpenAI should support function calling")
	}
	if SupportsFunctionCalling(ProviderOllama) {
		t.Error("Ollama should not support function calling")
	}
}

func TestSupportsMaxTokens(t *testing.T) {
	if SupportsMaxTokens(ProviderOpenAI, "o1-preview") != "max_completion_tokens" {
		t.Error("o1 should use max_completion_tokens")
	}
	if SupportsMaxTokens(ProviderOpenAI, "gpt-4o") != "max_tokens" {
		t.Error("gpt-4o should use max_tokens")
	}
	if SupportsMaxTokens(ProviderDeepSeek, "deepseek-chat") != "max_tokens" {
		t.Error("DeepSeek should use max_tokens")
	}
}

func TestGetAuthHeader(t *testing.T) {
	header, format := GetAuthHeader(ProviderAzureOpenAI)
	if header != "api-key" || format != "%s" {
		t.Errorf("Azure should use api-key header, got %s/%s", header, format)
	}
	header, format = GetAuthHeader(ProviderOpenAI)
	if header != "Authorization" || format != "Bearer %s" {
		t.Errorf("OpenAI should use Authorization Bearer, got %s/%s", header, format)
	}
}

func TestSupportsStopSequences(t *testing.T) {
	if !SupportsStopSequences(ProviderOpenAI) {
		t.Error("OpenAI should support stop sequences")
	}
	if SupportsStopSequences(ProviderOllama) {
		t.Error("Ollama should not support stop sequences")
	}
}
