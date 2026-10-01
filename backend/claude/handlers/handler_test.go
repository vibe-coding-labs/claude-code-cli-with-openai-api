package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// multipartWriter writes a single-file multipart form into buf and returns
// the Content-Type header value the caller should set on the request.
func multipartWriter(t *testing.T, buf *bytes.Buffer, filename, purpose, content string) string {
	t.Helper()
	w := multipart.NewWriter(buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write file content: %v", err)
	}
	if err := w.WriteField("purpose", purpose); err != nil {
		t.Fatalf("failed to write purpose field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}
	return w.FormDataContentType()
}

// TestNewHandler_DelegatesAllMethods exercises every delegation method on
// Handler through the public ClaudeHandler interface, confirming each one
// reaches the correct sub-handler (via observable status codes / JSON
// shape) rather than just asserting "no panic".
func TestNewHandler_DelegatesAllMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	h := NewHandler(cfg)

	run := func(name, method, path string, body []byte, params gin.Params, fn func(*gin.Context)) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = params
		fn(c)
		return w
	}

	t.Run("CountTokens", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"model":    "claude-3-5-sonnet-20241022",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		w := run("CountTokens", "POST", "/v1/messages/count_tokens", body, nil, h.CountTokens)
		if w.Code != http.StatusOK {
			t.Errorf("CountTokens: expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Batch lifecycle", func(t *testing.T) {
		createBody, _ := json.Marshal(models.CreateBatchRequest{
			Requests: []models.BatchMessageRequest{
				{
					CustomID: "r1",
					MessagesRequest: &models.MessagesRequest{
						Model:     "claude-3-opus-20240229",
						Messages:  []models.Message{{Role: "user", Content: "Test"}},
						MaxTokens: 100,
					},
				},
			},
		})
		w := run("CreateBatch", "POST", "/v1/batches", createBody, nil, h.CreateBatch)
		if w.Code != http.StatusOK {
			t.Fatalf("CreateBatch: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var batch models.BatchResponse
		if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil {
			t.Fatalf("failed to parse batch: %v", err)
		}

		w = run("GetBatch", "GET", "/v1/batches/"+batch.ID, nil, gin.Params{{Key: "batch_id", Value: batch.ID}}, h.GetBatch)
		if w.Code != http.StatusOK {
			t.Errorf("GetBatch: expected 200, got %d", w.Code)
		}

		w = run("ListBatches", "GET", "/v1/batches", nil, nil, h.ListBatches)
		if w.Code != http.StatusOK {
			t.Errorf("ListBatches: expected 200, got %d", w.Code)
		}

		w = run("GetBatchResults", "GET", "/v1/batches/"+batch.ID+"/results", nil, gin.Params{{Key: "batch_id", Value: batch.ID}}, h.GetBatchResults)
		if w.Code != http.StatusOK {
			t.Errorf("GetBatchResults: expected 200, got %d", w.Code)
		}

		w = run("CancelBatch", "POST", "/v1/batches/"+batch.ID+"/cancel", nil, gin.Params{{Key: "batch_id", Value: batch.ID}}, h.CancelBatch)
		if w.Code != http.StatusOK {
			t.Errorf("CancelBatch: expected 200, got %d", w.Code)
		}

		w = run("DeleteBatch", "DELETE", "/v1/batches/"+batch.ID, nil, gin.Params{{Key: "batch_id", Value: batch.ID}}, h.DeleteBatch)
		if w.Code != http.StatusOK {
			t.Errorf("DeleteBatch: expected 200, got %d", w.Code)
		}
	})

	t.Run("Files lifecycle", func(t *testing.T) {
		mpBody := &bytes.Buffer{}
		mw := multipartWriter(t, mpBody, "handler.txt", "assistants", "hello")

		req := httptest.NewRequest("POST", "/v1/files", mpBody)
		req.Header.Set("Content-Type", mw)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		h.CreateFile(c)
		if w.Code != http.StatusOK {
			t.Fatalf("CreateFile: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var file models.FileResponse
		if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
			t.Fatalf("failed to parse file: %v", err)
		}

		w = run("ListFiles", "GET", "/v1/files", nil, nil, h.ListFiles)
		if w.Code != http.StatusOK {
			t.Errorf("ListFiles: expected 200, got %d", w.Code)
		}

		w = run("GetFileMetadata", "GET", "/v1/files/"+file.ID, nil, gin.Params{{Key: "file_id", Value: file.ID}}, h.GetFileMetadata)
		if w.Code != http.StatusOK {
			t.Errorf("GetFileMetadata: expected 200, got %d", w.Code)
		}

		w = run("GetFileContent", "GET", "/v1/files/"+file.ID+"/content", nil, gin.Params{{Key: "file_id", Value: file.ID}}, h.GetFileContent)
		if w.Code != http.StatusOK {
			t.Errorf("GetFileContent: expected 200, got %d", w.Code)
		}

		w = run("DeleteFile", "DELETE", "/v1/files/"+file.ID, nil, gin.Params{{Key: "file_id", Value: file.ID}}, h.DeleteFile)
		if w.Code != http.StatusOK {
			t.Errorf("DeleteFile: expected 200, got %d", w.Code)
		}
	})

	t.Run("Skills lifecycle", func(t *testing.T) {
		createBody, _ := json.Marshal(models.CreateSkillRequest{Name: "handler-skill", Instructions: "x"})
		w := run("CreateSkill", "POST", "/v1/skills", createBody, nil, h.CreateSkill)
		if w.Code != http.StatusOK {
			t.Fatalf("CreateSkill: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var skill models.SkillResponse
		if err := json.Unmarshal(w.Body.Bytes(), &skill); err != nil {
			t.Fatalf("failed to parse skill: %v", err)
		}

		w = run("ListSkills", "GET", "/v1/skills", nil, nil, h.ListSkills)
		if w.Code != http.StatusOK {
			t.Errorf("ListSkills: expected 200, got %d", w.Code)
		}

		w = run("GetSkill", "GET", "/v1/skills/"+skill.ID, nil, gin.Params{{Key: "skill_id", Value: skill.ID}}, h.GetSkill)
		if w.Code != http.StatusOK {
			t.Errorf("GetSkill: expected 200, got %d", w.Code)
		}

		verBody, _ := json.Marshal(models.CreateSkillVersionRequest{Instructions: "v1"})
		w = run("CreateSkillVersion", "POST", "/v1/skills/"+skill.ID+"/versions", verBody, gin.Params{{Key: "skill_id", Value: skill.ID}}, h.CreateSkillVersion)
		if w.Code != http.StatusOK {
			t.Fatalf("CreateSkillVersion: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var version models.SkillVersionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil {
			t.Fatalf("failed to parse version: %v", err)
		}

		w = run("ListSkillVersions", "GET", "/v1/skills/"+skill.ID+"/versions", nil, gin.Params{{Key: "skill_id", Value: skill.ID}}, h.ListSkillVersions)
		if w.Code != http.StatusOK {
			t.Errorf("ListSkillVersions: expected 200, got %d", w.Code)
		}

		w = run("GetSkillVersion", "GET", "/v1/skill_versions/"+version.ID, nil, gin.Params{{Key: "version_id", Value: version.ID}}, h.GetSkillVersion)
		if w.Code != http.StatusOK {
			t.Errorf("GetSkillVersion: expected 200, got %d", w.Code)
		}

		w = run("DeleteSkillVersion", "DELETE", "/v1/skill_versions/"+version.ID, nil, gin.Params{{Key: "version_id", Value: version.ID}}, h.DeleteSkillVersion)
		if w.Code != http.StatusOK {
			t.Errorf("DeleteSkillVersion: expected 200, got %d", w.Code)
		}

		w = run("DeleteSkill", "DELETE", "/v1/skills/"+skill.ID, nil, gin.Params{{Key: "skill_id", Value: skill.ID}}, h.DeleteSkill)
		if w.Code != http.StatusOK {
			t.Errorf("DeleteSkill: expected 200, got %d", w.Code)
		}
	})

	t.Run("Models", func(t *testing.T) {
		w := run("ListModels", "GET", "/v1/models", nil, nil, h.ListModels)
		if w.Code != http.StatusOK {
			t.Errorf("ListModels: expected 200, got %d", w.Code)
		}

		w = run("GetModel", "GET", "/v1/models/claude-3-5-sonnet-20241022", nil, gin.Params{{Key: "model_id", Value: "claude-3-5-sonnet-20241022"}}, h.GetModel)
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("GetModel: expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("Admin", func(t *testing.T) {
		w := run("GetMe", "GET", "/v1/me", nil, nil, h.GetMe)
		if w.Code != http.StatusOK {
			t.Errorf("GetMe: expected 200, got %d", w.Code)
		}

		w = run("GetOrganizationUsage", "GET", "/v1/organizations/usage", nil, nil, h.GetOrganizationUsage)
		if w.Code != http.StatusOK {
			t.Errorf("GetOrganizationUsage: expected 200, got %d", w.Code)
		}
	})
}

// TestNewHandlerWithConfig covers both branches: dbConfig == nil (falls
// back to the passed-in cfg) and dbConfig != nil (builds a customConfig
// from the DB-sourced fields).
func TestNewHandlerWithConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("nil dbConfig falls back to cfg", func(t *testing.T) {
		cfg := &config.Config{OpenAIBaseURL: "https://api.example.com"}
		h := NewHandlerWithConfig(cfg, nil)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/me", nil)
		h.GetMe(c)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("non-nil dbConfig builds customConfig", func(t *testing.T) {
		cfg := &config.Config{RequestTimeout: 42}
		dbConfig := &database.APIConfig{
			OpenAIBaseURL:   "https://db.example.com",
			OpenAIAPIKey:    "db-key",
			BigModel:        "big",
			MiddleModel:     "middle",
			SmallModel:      "small",
			MaxTokensLimit:  1000,
			AnthropicAPIKey: "anthropic-key",
		}
		h := NewHandlerWithConfig(cfg, dbConfig)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/me", nil)
		h.GetMe(c)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}
