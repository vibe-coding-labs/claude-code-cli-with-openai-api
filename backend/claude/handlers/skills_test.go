package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func createTestSkill(t *testing.T, handler *SkillsHandler, name string) models.SkillResponse {
	t.Helper()
	body, _ := json.Marshal(models.CreateSkillRequest{
		Name:         name,
		Description:  "a test skill",
		Instructions: "do the thing",
	})
	req := httptest.NewRequest("POST", "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateSkill(c)

	var resp models.SkillResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse skill response: %v", err)
	}
	return resp
}

func TestSkillsHandler_CreateSkill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	t.Run("valid request", func(t *testing.T) {
		resp := createTestSkill(t, handler, "my-skill")
		if resp.ID == "" {
			t.Error("expected non-empty skill ID")
		}
		if resp.Name != "my-skill" {
			t.Errorf("expected name 'my-skill', got '%s'", resp.Name)
		}
		if resp.Type != "skill" {
			t.Errorf("expected type 'skill', got '%s'", resp.Type)
		}
	})

	t.Run("invalid json returns bad request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/skills", bytes.NewReader([]byte("{bad json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.CreateSkill(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
		var resp models.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Errorf("failed to parse error response: %v", err)
		}
		if resp.Error.Type != "invalid_request_error" {
			t.Errorf("expected error type 'invalid_request_error', got '%s'", resp.Error.Type)
		}
	})
}

func TestSkillsHandler_ListSkills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	createTestSkill(t, handler, "skill-1")
	createTestSkill(t, handler, "skill-2")

	req := httptest.NewRequest("GET", "/v1/skills", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.ListSkills(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var resp models.ListSkillsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 skills, got %d", len(resp.Data))
	}
}

func TestSkillsHandler_GetSkill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	created := createTestSkill(t, handler, "gettable-skill")

	tests := []struct {
		name         string
		skillID      string
		expectedCode int
	}{
		{"existing skill", created.ID, http.StatusOK},
		{"non-existing skill", "skill_nonexistent", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/skills/"+tt.skillID, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "skill_id", Value: tt.skillID}}

			handler.GetSkill(c)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestSkillsHandler_DeleteSkill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	created := createTestSkill(t, handler, "deletable-skill")

	t.Run("delete existing skill", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/skills/"+created.ID, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "skill_id", Value: created.ID}}

		handler.DeleteSkill(c)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("delete non-existing skill", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/skills/skill_nonexistent", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "skill_id", Value: "skill_nonexistent"}}

		handler.DeleteSkill(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func createTestSkillVersion(t *testing.T, handler *SkillsHandler, skillID string) models.SkillVersionResponse {
	t.Helper()
	body, _ := json.Marshal(models.CreateSkillVersionRequest{
		Instructions: "v1 instructions",
	})
	req := httptest.NewRequest("POST", "/v1/skills/"+skillID+"/versions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "skill_id", Value: skillID}}

	handler.CreateSkillVersion(c)

	var resp models.SkillVersionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse version response: %v", err)
	}
	return resp
}

func TestSkillsHandler_CreateSkillVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	skill := createTestSkill(t, handler, "versioned-skill")

	t.Run("valid version for existing skill", func(t *testing.T) {
		resp := createTestSkillVersion(t, handler, skill.ID)
		if resp.ID == "" {
			t.Error("expected non-empty version ID")
		}
		if resp.SkillID != skill.ID {
			t.Errorf("expected skill ID '%s', got '%s'", skill.ID, resp.SkillID)
		}
	})

	t.Run("invalid json returns bad request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/skills/"+skill.ID+"/versions", bytes.NewReader([]byte("{bad")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "skill_id", Value: skill.ID}}

		handler.CreateSkillVersion(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("skill not found", func(t *testing.T) {
		body, _ := json.Marshal(models.CreateSkillVersionRequest{Instructions: "x"})
		req := httptest.NewRequest("POST", "/v1/skills/skill_nonexistent/versions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "skill_id", Value: "skill_nonexistent"}}

		handler.CreateSkillVersion(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestSkillsHandler_ListSkillVersions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	s := createTestSkill(t, handler, "list-versions-skill")
	createTestSkillVersion(t, handler, s.ID)
	createTestSkillVersion(t, handler, s.ID)

	req := httptest.NewRequest("GET", "/v1/skills/"+s.ID+"/versions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "skill_id", Value: s.ID}}

	handler.ListSkillVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var resp models.ListSkillVersionsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 versions, got %d", len(resp.Data))
	}
}

func TestSkillsHandler_GetSkillVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	s := createTestSkill(t, handler, "get-version-skill")
	v := createTestSkillVersion(t, handler, s.ID)

	tests := []struct {
		name         string
		versionID    string
		expectedCode int
	}{
		{"existing version", v.ID, http.StatusOK},
		{"non-existing version", "skver_nonexistent", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/skill_versions/"+tt.versionID, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "version_id", Value: tt.versionID}}

			handler.GetSkillVersion(c)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestSkillsHandler_DeleteSkillVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewSkillsHandler(cfg)

	s := createTestSkill(t, handler, "delete-version-skill")
	v := createTestSkillVersion(t, handler, s.ID)

	t.Run("delete existing version", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/skill_versions/"+v.ID, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "version_id", Value: v.ID}}

		handler.DeleteSkillVersion(c)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("delete non-existing version", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/skill_versions/skver_nonexistent", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "version_id", Value: "skver_nonexistent"}}

		handler.DeleteSkillVersion(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}
