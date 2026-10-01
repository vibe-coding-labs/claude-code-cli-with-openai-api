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
)

func newMultipartFileRequest(t *testing.T, filename, purpose, content string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write file content: %v", err)
		}
	}
	if purpose != "" {
		if err := writer.WriteField("purpose", purpose); err != nil {
			t.Fatalf("failed to write purpose field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}
	req := httptest.NewRequest("POST", "/v1/files", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestFilesHandler_CreateFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewFilesHandler(cfg)

	t.Run("create file with default purpose", func(t *testing.T) {
		req := newMultipartFileRequest(t, "test.txt", "", "hello world")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.CreateFile(c)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp models.FileResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		if resp.Purpose != "assistants" {
			t.Errorf("Expected default purpose 'assistants', got '%s'", resp.Purpose)
		}
		if resp.FileName != "test.txt" {
			t.Errorf("Expected filename 'test.txt', got '%s'", resp.FileName)
		}
	})

	t.Run("create file with explicit purpose", func(t *testing.T) {
		req := newMultipartFileRequest(t, "doc.pdf", "vision", "pdf-bytes")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.CreateFile(c)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp models.FileResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		if resp.Purpose != "vision" {
			t.Errorf("Expected purpose 'vision', got '%s'", resp.Purpose)
		}
	})

	t.Run("missing file field returns bad request", func(t *testing.T) {
		req := newMultipartFileRequest(t, "", "assistants", "")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.CreateFile(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}

		var resp models.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}
		if resp.Error.Type != "invalid_request_error" {
			t.Errorf("Expected error type 'invalid_request_error', got '%s'", resp.Error.Type)
		}
	})
}

func createTestFile(t *testing.T, handler *FilesHandler, filename, purpose string) models.FileResponse {
	t.Helper()
	req := newMultipartFileRequest(t, filename, purpose, "content")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateFile(c)

	var resp models.FileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	return resp
}

func TestFilesHandler_ListFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewFilesHandler(cfg)

	createTestFile(t, handler, "a.txt", "assistants")
	createTestFile(t, handler, "b.txt", "vision")

	t.Run("list all files", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/files", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.ListFiles(c)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var resp models.ListFilesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		if len(resp.Data) != 2 {
			t.Errorf("Expected 2 files, got %d", len(resp.Data))
		}
	})

	t.Run("list files filtered by purpose", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/files?purpose=vision", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.ListFiles(c)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var resp models.ListFilesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		if len(resp.Data) != 1 {
			t.Errorf("Expected 1 file filtered by purpose, got %d", len(resp.Data))
		}
	})
}

func TestFilesHandler_GetFileMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewFilesHandler(cfg)

	created := createTestFile(t, handler, "meta.txt", "assistants")

	tests := []struct {
		name         string
		fileID       string
		expectedCode int
	}{
		{"existing file", created.ID, http.StatusOK},
		{"non-existing file", "file_nonexistent", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/files/"+tt.fileID, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "file_id", Value: tt.fileID}}

			handler.GetFileMetadata(c)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestFilesHandler_GetFileContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewFilesHandler(cfg)

	created := createTestFile(t, handler, "content.txt", "assistants")

	t.Run("existing file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/files/"+created.ID+"/content", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "file_id", Value: created.ID}}

		handler.GetFileContent(c)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}
		if w.Body.Len() == 0 {
			t.Error("Expected non-empty file content")
		}
	})

	t.Run("non-existing file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/files/file_nonexistent/content", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "file_id", Value: "file_nonexistent"}}

		handler.GetFileContent(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestFilesHandler_DeleteFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	handler := NewFilesHandler(cfg)

	created := createTestFile(t, handler, "delete.txt", "assistants")

	t.Run("delete existing file", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/files/"+created.ID, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "file_id", Value: created.ID}}

		handler.DeleteFile(c)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("delete non-existing file", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/files/file_nonexistent", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "file_id", Value: "file_nonexistent"}}

		handler.DeleteFile(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}
