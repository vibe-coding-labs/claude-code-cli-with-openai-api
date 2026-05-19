package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func TestModelsHandler_ListModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewModelsHandler(cfg)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.ListModels(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response models.ListModelsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	// 验证响应
	if response.Object != "list" {
		t.Errorf("Expected object 'list', got '%s'", response.Object)
	}

	if len(response.Data) == 0 {
		t.Error("Expected at least one model in the list")
	}

	// 验证包含预期的模型
	expectedModels := []string{
		"claude-3-opus-20240229",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-haiku-20240307",
	}

	modelMap := make(map[string]bool)
	for _, model := range response.Data {
		modelMap[model.ID] = true

		// 验证模型字段
		if model.Object != "model" {
			t.Errorf("Expected object 'model' for %s, got '%s'", model.ID, model.Object)
		}
		if model.OwnedBy != "anthropic" {
			t.Errorf("Expected owner 'anthropic' for %s, got '%s'", model.ID, model.OwnedBy)
		}
		if model.ContextSize != 200000 {
			t.Errorf("Expected context size 200000 for %s, got %d", model.ID, model.ContextSize)
		}
	}

	for _, expectedModel := range expectedModels {
		if !modelMap[expectedModel] {
			t.Errorf("Expected model %s not found in response", expectedModel)
		}
	}
}

func TestModelsHandler_GetModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewModelsHandler(cfg)

	tests := []struct {
		name         string
		modelID      string
		expectedCode int
	}{
		{
			name:         "Get claude-3-opus model",
			modelID:      "claude-3-opus-20240229",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Get claude-3-5-sonnet model",
			modelID:      "claude-3-5-sonnet-20241022",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Get claude-3-5-haiku model",
			modelID:      "claude-3-5-haiku-20241022",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Get claude-3-haiku model",
			modelID:      "claude-3-haiku-20240307",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Get non-existent model",
			modelID:      "claude-2-invalid",
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/models/"+tt.modelID, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "model_id", Value: tt.modelID}}

			handler.GetModel(c)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if w.Code == http.StatusOK {
				var response models.ModelResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to parse response: %v", err)
				}

				// 验证模型ID匹配
				if response.ID != tt.modelID {
					t.Errorf("Expected model ID %s, got %s", tt.modelID, response.ID)
				}

				// 验证基本字段
				if response.Object != "model" {
					t.Errorf("Expected object 'model', got '%s'", response.Object)
				}
				if response.OwnedBy != "anthropic" {
					t.Errorf("Expected owner 'anthropic', got '%s'", response.OwnedBy)
				}
			}
		})
	}
}
