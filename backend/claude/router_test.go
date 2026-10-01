package claude

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// setupRouterTestDB initializes a throwaway sqlite database for router tests
// and returns the IDs of an enabled and a disabled API config.
func setupRouterTestDB(t *testing.T) (enabledID, disabledID string) {
	t.Helper()

	if err := database.InitEncryption(); err != nil {
		t.Fatalf("InitEncryption failed: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "router_test.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	enabled := &database.APIConfig{
		ID:            "router-test-enabled",
		Name:          "enabled-config",
		OpenAIBaseURL: "https://example.invalid/v1",
		BigModel:      "gpt-4",
		MiddleModel:   "gpt-4",
		SmallModel:    "gpt-4",
		Enabled:       true,
	}
	if err := database.CreateAPIConfig(enabled); err != nil {
		t.Fatalf("CreateAPIConfig(enabled) failed: %v", err)
	}

	disabled := &database.APIConfig{
		ID:            "router-test-disabled",
		Name:          "disabled-config",
		OpenAIBaseURL: "https://example.invalid/v1",
		BigModel:      "gpt-4",
		MiddleModel:   "gpt-4",
		SmallModel:    "gpt-4",
		Enabled:       false,
	}
	if err := database.CreateAPIConfig(disabled); err != nil {
		t.Fatalf("CreateAPIConfig(disabled) failed: %v", err)
	}

	return enabled.ID, disabled.ID
}

func TestRegisterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRouterTestDB(t)

	engine := gin.New()
	cfg := &config.Config{RequestTimeout: 5}

	// Registering routes should not panic and should populate the route table
	// for both the direct /v1/* group and the per-config /proxy/:id/v1/* group.
	RegisterRoutes(engine, cfg)

	routes := engine.Routes()
	if len(routes) == 0 {
		t.Fatalf("expected routes to be registered, got none")
	}

	var sawDirectMessages, sawProxyMessages bool
	for _, r := range routes {
		if r.Method == "POST" && r.Path == "/v1/messages" {
			sawDirectMessages = true
		}
		if r.Method == "POST" && r.Path == "/proxy/:id/v1/messages" {
			sawProxyMessages = true
		}
	}
	if !sawDirectMessages {
		t.Errorf("expected /v1/messages route to be registered")
	}
	if !sawProxyMessages {
		t.Errorf("expected /proxy/:id/v1/messages route to be registered")
	}
}

func TestCreateProxyHandler_ConfigNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRouterTestDB(t)

	cfg := &config.Config{}
	handler := createProxyHandler(cfg, "messages")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "does-not-exist"}}
	c.Request = httptest.NewRequest("POST", "/proxy/does-not-exist/v1/messages", nil)

	handler(c)

	if w.Code != 404 {
		t.Errorf("expected 404 for missing config, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateProxyHandler_ConfigDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, disabledID := setupRouterTestDB(t)

	cfg := &config.Config{}
	handler := createProxyHandler(cfg, "messages")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: disabledID}}
	c.Request = httptest.NewRequest("POST", "/proxy/"+disabledID+"/v1/messages", nil)

	handler(c)

	if w.Code != 400 {
		t.Errorf("expected 400 for disabled config, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateProxyHandler_AllEndpoints drives every known "endpoint" switch
// case in createProxyHandler (plus the default/unknown branch) through an
// enabled config, to exercise the dispatch table in router.go. None of these
// underlying handlers perform outbound network calls for the request shapes
// used here (empty/invalid bodies short-circuit on validation, and the
// batch/file/skill/model/admin handlers are purely in-memory).
func TestCreateProxyHandler_AllEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enabledID, _ := setupRouterTestDB(t)

	cfg := &config.Config{RequestTimeout: 5}

	endpoints := []struct {
		name   string
		method string
	}{
		{"messages", "POST"},
		{"count_tokens", "POST"},
		{"create_batch", "POST"},
		{"get_batch", "GET"},
		{"list_batches", "GET"},
		{"get_batch_results", "GET"},
		{"cancel_batch", "POST"},
		{"delete_batch", "DELETE"},
		{"create_file", "POST"},
		{"list_files", "GET"},
		{"get_file_metadata", "GET"},
		{"get_file_content", "GET"},
		{"delete_file", "DELETE"},
		{"create_skill", "POST"},
		{"list_skills", "GET"},
		{"get_skill", "GET"},
		{"delete_skill", "DELETE"},
		{"create_skill_version", "POST"},
		{"list_skill_versions", "GET"},
		{"get_skill_version", "GET"},
		{"delete_skill_version", "DELETE"},
		{"list_models", "GET"},
		{"get_model", "GET"},
		{"get_me", "GET"},
		{"get_organization_usage", "GET"},
		{"some_unknown_endpoint", "GET"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			handler := createProxyHandler(cfg, ep.name)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: enabledID}}
			c.Request = httptest.NewRequest(ep.method, "/proxy/"+enabledID+"/v1/x", nil)

			// handler must not panic for any dispatched endpoint, including
			// the unknown one which should fall through to the default 404.
			handler(c)

			if ep.name == "some_unknown_endpoint" && w.Code != 404 {
				t.Errorf("expected 404 for unknown endpoint, got %d: %s", w.Code, w.Body.String())
			}
			if w.Code == 0 {
				t.Errorf("handler for %q did not write a response", ep.name)
			}
		})
	}
}
