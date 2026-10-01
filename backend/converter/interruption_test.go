package converter

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetInterruptionContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	SetInterruptionContext(c, "cfg-1", "my-config", "sess-1")

	if got := c.GetString(ginKeyConfigID); got != "cfg-1" {
		t.Errorf("configID = %q, want %q", got, "cfg-1")
	}
	if got := c.GetString(ginKeyConfigName); got != "my-config" {
		t.Errorf("configName = %q, want %q", got, "my-config")
	}
	if got := c.GetString(ginKeySessionID); got != "sess-1" {
		t.Errorf("sessionID = %q, want %q", got, "sess-1")
	}
}

func TestSetInterruptionContext_EmptyValuesAreNotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	// Empty strings must not overwrite/set keys — EmitInterruption relies on
	// GetString's zero-value fallback for fields the handler never resolved.
	SetInterruptionContext(c, "", "", "")

	if _, exists := c.Get(ginKeyConfigID); exists {
		t.Error("empty configID should not be set on the context")
	}
	if _, exists := c.Get(ginKeyConfigName); exists {
		t.Error("empty configName should not be set on the context")
	}
	if _, exists := c.Get(ginKeySessionID); exists {
		t.Error("empty sessionID should not be set on the context")
	}
}

func TestEmitInterruption_NilContextIsNoop(t *testing.T) {
	// Must not panic — this is the documented no-op guard for callers that
	// don't have a gin context available (e.g. background goroutines).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EmitInterruption(nil, ...) panicked: %v", r)
		}
	}()
	EmitInterruption(nil, "cause", "dimension", "stage", "detail", 42, "gpt-4o")
}

func TestEmitInterruption_WithContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	SetInterruptionContext(c, "cfg-1", "my-config", "sess-1")

	// The write is queued asynchronously (database.LogInterruptionAsync); this
	// only proves the call path doesn't panic and correctly reads back the
	// gin-context attribution set above. Actual persistence is exercised by
	// database package tests.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EmitInterruption panicked: %v", r)
		}
	}()
	EmitInterruption(c, "client_disconnect", "network", "streaming", "connection reset", 1500, "gpt-4o")
}
