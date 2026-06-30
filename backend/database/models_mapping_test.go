package database

import "testing"

// TestModelMappingsRoundTrip exercises the full model_mappings wiring: the
// column is written by CreateAPIConfig/UpdateAPIConfig and read back by every
// SELECT (GetConfigByAnthropicAPIKey — the Responses hot path — GetAPIConfig,
// GetAllAPIConfigs). A failure here means a Scan/INSERT/UPDATE arg-count
// mismatch in models.go, which would surface at runtime as a SQL Scan error.
func TestModelMappingsRoundTrip(t *testing.T) {
	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()

	if err := InitEncryption(); err != nil {
		t.Fatalf("InitEncryption: %v", err)
	}

	const token = "alias_test_token" // custom tokens: alphanumeric + underscore only
	cfg := &APIConfig{
		Name:            "alias-test",
		OpenAIAPIKey:    "sk-test-upstream-key",
		OpenAIBaseURL:   "https://upstream.example.com/v2",
		BigModel:        "astron-code-latest",
		AnthropicAPIKey: token,
		Enabled:         true,
		SupportedModels: []string{"astron-code-latest"},
		ModelMappings: map[string]string{
			"glm-5.1": "astron-code-latest",
			"glm-4.6": "astron-code-latest",
		},
	}
	if err := CreateAPIConfig(cfg); err != nil {
		t.Fatalf("CreateAPIConfig: %v", err)
	}

	// Hot path: the Responses handler resolves config by client key.
	got, err := GetConfigByAnthropicAPIKey(token)
	if err != nil {
		t.Fatalf("GetConfigByAnthropicAPIKey: %v", err)
	}
	if len(got.ModelMappings) != 2 {
		t.Fatalf("expected 2 model mappings via key lookup, got %d (%v)", len(got.ModelMappings), got.ModelMappings)
	}
	if got.ModelMappings["glm-5.1"] != "astron-code-latest" {
		t.Errorf("glm-5.1 mapping = %q, want astron-code-latest", got.ModelMappings["glm-5.1"])
	}
	// End-to-end alias resolution on the loaded config.
	if r := ResolveModelAlias("glm-5.1", got.ModelMappings); r != "astron-code-latest" {
		t.Errorf("ResolveModelAlias(glm-5.1) = %q, want astron-code-latest", r)
	}
	if r := ResolveModelAlias("gpt-4o", got.ModelMappings); r != "gpt-4o" {
		t.Errorf("ResolveModelAlias(gpt-4o) = %q, want passthrough gpt-4o", r)
	}

	// By-ID lookup reads mappings too.
	byID, err := GetAPIConfig(cfg.ID)
	if err != nil {
		t.Fatalf("GetAPIConfig: %v", err)
	}
	if byID.ModelMappings["glm-4.6"] != "astron-code-latest" {
		t.Errorf("byID glm-4.6 mapping = %q, want astron-code-latest", byID.ModelMappings["glm-4.6"])
	}

	// List view reads mappings.
	all, err := GetAllAPIConfigs()
	if err != nil {
		t.Fatalf("GetAllAPIConfigs: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 config in list, got %d", len(all))
	}
	if all[0].ModelMappings["glm-5.1"] != "astron-code-latest" {
		t.Errorf("list glm-5.1 mapping = %q, want astron-code-latest", all[0].ModelMappings["glm-5.1"])
	}

	// UPDATE path: shrink mappings and verify it persists.
	cfg.ModelMappings = map[string]string{"glm-5.1": "astron-code-latest"}
	if err := UpdateAPIConfig(cfg); err != nil {
		t.Fatalf("UpdateAPIConfig: %v", err)
	}
	GetConfigCache().Invalidate(token) // bypass cached pre-update value
	got2, err := GetConfigByAnthropicAPIKey(token)
	if err != nil {
		t.Fatalf("re-GetConfigByAnthropicAPIKey: %v", err)
	}
	if len(got2.ModelMappings) != 1 {
		t.Errorf("after update expected 1 mapping, got %d (%v)", len(got2.ModelMappings), got2.ModelMappings)
	}
}
