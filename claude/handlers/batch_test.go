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

func TestBatchHandler_CreateBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewBatchHandler(cfg)

	tests := []struct {
		name         string
		request      models.CreateBatchRequest
		expectedCode int
	}{
		{
			name: "Create batch with single request",
			request: models.CreateBatchRequest{
				Requests: []models.BatchMessageRequest{
					{
						CustomID: "req_1",
						MessagesRequest: &models.MessagesRequest{
							Model: "claude-3-opus-20240229",
							Messages: []models.Message{
								{Role: "user", Content: "Hello"},
							},
							MaxTokens: 100,
						},
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Create batch with multiple requests",
			request: models.CreateBatchRequest{
				Requests: []models.BatchMessageRequest{
					{
						CustomID: "req_1",
						MessagesRequest: &models.MessagesRequest{
							Model:     "claude-3-opus-20240229",
							Messages:  []models.Message{{Role: "user", Content: "Hello"}},
							MaxTokens: 100,
						},
					},
					{
						CustomID: "req_2",
						MessagesRequest: &models.MessagesRequest{
							Model:     "claude-3-haiku-20240307",
							Messages:  []models.Message{{Role: "user", Content: "World"}},
							MaxTokens: 50,
						},
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Create empty batch",
			request: models.CreateBatchRequest{
				Requests: []models.BatchMessageRequest{},
			},
			expectedCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/v1/batches", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			handler.CreateBatch(c)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if w.Code == http.StatusOK {
				var response models.BatchResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to parse response: %v", err)
				}

				// 验证响应字段
				if response.ID == "" {
					t.Error("Batch ID should not be empty")
				}
				if response.Type != "message_batch" {
					t.Errorf("Expected type 'message_batch', got '%s'", response.Type)
				}
				if response.ProcessingStatus != "in_progress" {
					t.Errorf("Expected status 'in_progress', got '%s'", response.ProcessingStatus)
				}
				if response.RequestCounts.Total != len(tt.request.Requests) {
					t.Errorf("Request count mismatch: got %d, want %d",
						response.RequestCounts.Total, len(tt.request.Requests))
				}
			}
		})
	}
}

func TestBatchHandler_GetBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewBatchHandler(cfg)

	// 先创建一个batch
	createReq := models.CreateBatchRequest{
		Requests: []models.BatchMessageRequest{
			{
				CustomID: "test_req",
				MessagesRequest: &models.MessagesRequest{
					Model:     "claude-3-opus-20240229",
					Messages:  []models.Message{{Role: "user", Content: "Test"}},
					MaxTokens: 100,
				},
			},
		},
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/v1/batches", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	var createdBatch models.BatchResponse
	json.Unmarshal(w.Body.Bytes(), &createdBatch)

	// 测试获取batch
	tests := []struct {
		name         string
		batchID      string
		expectedCode int
	}{
		{
			name:         "Get existing batch",
			batchID:      createdBatch.ID,
			expectedCode: http.StatusOK,
		},
		{
			name:         "Get non-existing batch",
			batchID:      "batch_nonexistent",
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/batches/"+tt.batchID, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "batch_id", Value: tt.batchID}}

			handler.GetBatch(c)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestBatchHandler_ListBatches(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewBatchHandler(cfg)

	// 创建几个batches
	for i := 0; i < 3; i++ {
		createReq := models.CreateBatchRequest{
			Requests: []models.BatchMessageRequest{
				{
					CustomID: "test_req",
					MessagesRequest: &models.MessagesRequest{
						Model:     "claude-3-opus-20240229",
						Messages:  []models.Message{{Role: "user", Content: "Test"}},
						MaxTokens: 100,
					},
				},
			},
		}

		body, _ := json.Marshal(createReq)
		req := httptest.NewRequest("POST", "/v1/batches", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.CreateBatch(c)
	}

	// 测试列出batches
	req := httptest.NewRequest("GET", "/v1/batches", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.ListBatches(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response models.ListBatchesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if len(response.Data) < 3 {
		t.Errorf("Expected at least 3 batches, got %d", len(response.Data))
	}
}

func TestBatchHandler_CancelBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewBatchHandler(cfg)

	// 创建一个batch
	createReq := models.CreateBatchRequest{
		Requests: []models.BatchMessageRequest{
			{
				CustomID: "test_req",
				MessagesRequest: &models.MessagesRequest{
					Model:     "claude-3-opus-20240229",
					Messages:  []models.Message{{Role: "user", Content: "Test"}},
					MaxTokens: 100,
				},
			},
		},
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/v1/batches", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	var createdBatch models.BatchResponse
	json.Unmarshal(w.Body.Bytes(), &createdBatch)

	// 取消batch
	req = httptest.NewRequest("POST", "/v1/batches/"+createdBatch.ID+"/cancel", nil)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "batch_id", Value: createdBatch.ID}}

	handler.CancelBatch(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var cancelledBatch models.BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &cancelledBatch)
	if err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if cancelledBatch.ProcessingStatus != "canceled" {
		t.Errorf("Expected status 'canceled', got '%s'", cancelledBatch.ProcessingStatus)
	}

	if cancelledBatch.CancelInitiatedAt == nil {
		t.Error("CancelInitiatedAt should not be nil")
	}
}

func TestBatchHandler_DeleteBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewBatchHandler(cfg)

	// 创建一个batch
	createReq := models.CreateBatchRequest{
		Requests: []models.BatchMessageRequest{
			{
				CustomID: "test_req",
				MessagesRequest: &models.MessagesRequest{
					Model:     "claude-3-opus-20240229",
					Messages:  []models.Message{{Role: "user", Content: "Test"}},
					MaxTokens: 100,
				},
			},
		},
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/v1/batches", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	var createdBatch models.BatchResponse
	json.Unmarshal(w.Body.Bytes(), &createdBatch)

	// 删除batch
	req = httptest.NewRequest("DELETE", "/v1/batches/"+createdBatch.ID, nil)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "batch_id", Value: createdBatch.ID}}

	handler.DeleteBatch(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// 尝试再次获取已删除的batch
	req = httptest.NewRequest("GET", "/v1/batches/"+createdBatch.ID, nil)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "batch_id", Value: createdBatch.ID}}

	handler.GetBatch(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d for deleted batch, got %d", http.StatusNotFound, w.Code)
	}
}
