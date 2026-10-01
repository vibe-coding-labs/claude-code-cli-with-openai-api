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

func TestDetectProviderGeminiAndGoogleAndAnthropic(t *testing.T) {
	tests := []struct {
		url      string
		expected ProviderType
	}{
		{"https://generativelanguage.googleapis.com/v1beta", ProviderGemini},
		{"https://my-gemini-proxy.example.com/v1", ProviderGemini},
		{"https://otherservice.googleapis.com/v1", ProviderGoogle},
		{"https://api.anthropic.com/v1", ProviderAnthropic},
		{"https://anthropic.com/v1", ProviderAnthropic},
	}
	for _, tt := range tests {
		if got := DetectProvider(tt.url); got != tt.expected {
			t.Errorf("DetectProvider(%q) = %v, want %v", tt.url, got, tt.expected)
		}
	}
}

func TestIsGeminiProvider(t *testing.T) {
	if !IsGeminiProvider("https://generativelanguage.googleapis.com/v1beta") {
		t.Error("expected Gemini endpoint to be detected as Gemini provider")
	}
	if !IsGeminiProvider("https://otherservice.googleapis.com/v1") {
		t.Error("expected a generic googleapis.com endpoint (ProviderGoogle) to count as Gemini provider")
	}
	if IsGeminiProvider("https://api.openai.com/v1") {
		t.Error("OpenAI endpoint should not be detected as Gemini provider")
	}
}

func TestIsAnthropicProvider(t *testing.T) {
	if !IsAnthropicProvider("https://api.anthropic.com/v1") {
		t.Error("expected api.anthropic.com to be detected as Anthropic provider")
	}
	if IsAnthropicProvider("https://api.openai.com/v1") {
		t.Error("OpenAI endpoint should not be detected as Anthropic provider")
	}
	if IsAnthropicProvider("https://generativelanguage.googleapis.com/v1beta") {
		t.Error("Gemini endpoint should not be detected as Anthropic provider")
	}
}

func TestGetMaxTokensCap(t *testing.T) {
	tests := []struct {
		provider ProviderType
		expected int
	}{
		{ProviderOpenAI, 16384},
		{ProviderAzureOpenAI, 16384},
		{ProviderDeepSeek, 16384},
		{ProviderOllama, 0},
		{ProviderOpenRouter, 0},
		{ProviderMistral, 0},
		{ProviderGemini, 0},
		{ProviderGoogle, 0},
		{ProviderAnthropic, 0},
		{ProviderUnknown, 0},
	}
	for _, tt := range tests {
		if got := GetMaxTokensCap(tt.provider); got != tt.expected {
			t.Errorf("GetMaxTokensCap(%v) = %d, want %d", tt.provider, got, tt.expected)
		}
	}
}

func TestCleanSchemaForGeminiNil(t *testing.T) {
	if got := CleanSchemaForGemini(nil); got != nil {
		t.Errorf("expected nil input to produce nil output, got %v", got)
	}
}

// TestCleanSchemaForGemini verifies the blacklist-based cleaner strips only
// known-unsupported JSON Schema keywords and never touches arbitrary
// property names — the correct counterpart to gemini.go's whitelist-based
// stripUnsupportedSchemaFields (see Bug #2 in gemini.go, where the
// whitelist approach silently deleted every property because property
// names like "city" aren't schema keywords). This function is
// blacklist-based, so property names are never matched against
// geminiUnsupportedSchemaFields in the first place.
func TestCleanSchemaForGemini(t *testing.T) {
	schema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"default":              "top-level-default",
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"deprecated":           true,
		"examples":             []interface{}{"a", "b"},
		"properties": map[string]interface{}{
			"city": map[string]interface{}{
				"type":    "string",
				"default": "Beijing",
			},
			"count": map[string]interface{}{
				"type":       "integer",
				"deprecated": true,
			},
		},
		"items": map[string]interface{}{
			"type":    "string",
			"default": "dropped-in-items",
		},
		"anyOf": []interface{}{
			map[string]interface{}{"type": "string", "default": "dropped-in-anyof"},
			map[string]interface{}{"type": "integer"},
		},
	}

	got := CleanSchemaForGemini(schema)

	for _, field := range []string{"additionalProperties", "default", "$schema", "deprecated", "examples"} {
		if _, ok := got[field]; ok {
			t.Errorf("top-level field %q should have been stripped", field)
		}
	}
	if got["type"] != "object" {
		t.Errorf("expected type to survive, got %v", got["type"])
	}

	props, ok := got["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to survive as a map")
	}
	if _, ok := props["city"]; !ok {
		t.Fatal("property name 'city' must survive — it is not a schema keyword")
	}
	if _, ok := props["count"]; !ok {
		t.Fatal("property name 'count' must survive — it is not a schema keyword")
	}
	cityProp := props["city"].(map[string]interface{})
	if _, ok := cityProp["default"]; ok {
		t.Error("nested default inside properties.city should have been stripped")
	}
	if cityProp["type"] != "string" {
		t.Errorf("expected nested type to survive, got %v", cityProp["type"])
	}
	countProp := props["count"].(map[string]interface{})
	if _, ok := countProp["deprecated"]; ok {
		t.Error("nested deprecated inside properties.count should have been stripped")
	}

	itemsSchema, ok := got["items"].(map[string]interface{})
	if !ok {
		t.Fatal("expected items to survive as a map")
	}
	if _, ok := itemsSchema["default"]; ok {
		t.Error("default inside items should have been stripped")
	}

	anyOf, ok := got["anyOf"].([]interface{})
	if !ok || len(anyOf) != 2 {
		t.Fatalf("expected anyOf array of len 2, got %v", got["anyOf"])
	}
	anyOfFirst, ok := anyOf[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected anyOf[0] to survive as a map")
	}
	if _, ok := anyOfFirst["default"]; ok {
		t.Error("default inside anyOf[0] should have been stripped")
	}
}

func TestCleanSchemaForGeminiItemsAsArray(t *testing.T) {
	// "items" as a tuple-validation array (rather than a single schema map)
	// is handled by cleanSchemaValue's generic []interface{} branch.
	schema := map[string]interface{}{
		"type": "array",
		"items": []interface{}{
			map[string]interface{}{"type": "string", "default": "x"},
			map[string]interface{}{"type": "number"},
		},
	}
	got := CleanSchemaForGemini(schema)
	items, ok := got["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("expected items array of len 2 to survive, got %v", got["items"])
	}
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected items[0] to survive as a map")
	}
	if _, ok := first["default"]; ok {
		t.Error("default inside items[0] should have been stripped")
	}
}
