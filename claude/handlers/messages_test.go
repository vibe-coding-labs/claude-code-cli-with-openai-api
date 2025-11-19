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

func TestMessagesHandler_CreateMessage(t *testing.T) {
	// 设置测试路由
	gin.SetMode(gin.TestMode)

	// 创建测试配置
	cfg := &config.Config{
		OpenAIBaseURL:  "https://api.openai.com/v1",
		OpenAIAPIKey:   "test-key",
		BigModel:       "gpt-4",
		MiddleModel:    "gpt-4",
		SmallModel:     "gpt-3.5-turbo",
		MaxTokensLimit: 4096,
	}

	// 创建处理器
	_ = NewMessagesHandler(cfg)

	// 测试用例
	tests := []struct {
		name         string
		request      models.MessagesRequest
		expectedCode int
		checkBody    bool
	}{
		{
			name: "Valid message request",
			request: models.MessagesRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{
						Role:    "user",
						Content: "Hello, Claude!",
					},
				},
				MaxTokens: 1024,
			},
			expectedCode: http.StatusOK,
			checkBody:    true,
		},
		{
			name: "Message with system prompt",
			request: models.MessagesRequest{
				Model:  "claude-3-opus-20240229",
				System: "You are a helpful assistant.",
				Messages: []models.Message{
					{
						Role:    "user",
						Content: "What is 2+2?",
					},
				},
				MaxTokens: 100,
			},
			expectedCode: http.StatusOK,
			checkBody:    true,
		},
		{
			name: "Message with tools",
			request: models.MessagesRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{
						Role:    "user",
						Content: "What's the weather?",
					},
				},
				MaxTokens: 1024,
				Tools: []models.Tool{
					{
						Name:        "get_weather",
						Description: "Get current weather",
						InputSchema: map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"location": map[string]interface{}{
									"type": "string",
								},
							},
						},
					},
				},
			},
			expectedCode: http.StatusOK,
			checkBody:    true,
		},
		{
			name: "Invalid request - missing messages",
			request: models.MessagesRequest{
				Model:     "claude-3-opus-20240229",
				MaxTokens: 1024,
			},
			expectedCode: http.StatusOK, // 仍然会处理空消息
			checkBody:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建请求
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// 注意：由于需要真实的OpenAI调用，这里只测试请求解析部分
			// 在实际测试中，应该模拟OpenAI客户端

			// 验证请求可以正确解析
			var parsedReq models.MessagesRequest
			err := c.ShouldBindJSON(&parsedReq)
			if err != nil && tt.expectedCode == http.StatusOK {
				t.Errorf("Failed to parse request: %v", err)
			}

			// 验证解析后的数据
			if tt.checkBody {
				if parsedReq.Model != tt.request.Model {
					t.Errorf("Model mismatch: got %s, want %s", parsedReq.Model, tt.request.Model)
				}
				if len(parsedReq.Messages) != len(tt.request.Messages) {
					t.Errorf("Messages count mismatch: got %d, want %d",
						len(parsedReq.Messages), len(tt.request.Messages))
				}
			}
		})
	}
}

func TestMessagesHandler_CountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewMessagesHandler(cfg)

	tests := []struct {
		name         string
		request      models.CountTokensRequest
		expectedCode int
		minTokens    int
		maxTokens    int
	}{
		{
			name: "Count tokens for simple message",
			request: models.CountTokensRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{
						Role:    "user",
						Content: "Hello, world!",
					},
				},
			},
			expectedCode: http.StatusOK,
			minTokens:    1,
			maxTokens:    10,
		},
		{
			name: "Count tokens with system prompt",
			request: models.CountTokensRequest{
				Model:  "claude-3-opus-20240229",
				System: "You are a helpful assistant.",
				Messages: []models.Message{
					{
						Role:    "user",
						Content: "What is the meaning of life?",
					},
				},
			},
			expectedCode: http.StatusOK,
			minTokens:    5,
			maxTokens:    30,
		},
		{
			name: "Count tokens with tools",
			request: models.CountTokensRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{
						Role:    "user",
						Content: "Get weather",
					},
				},
				Tools: []models.Tool{
					{
						Name:        "get_weather",
						Description: "Get current weather for a location",
						InputSchema: map[string]interface{}{
							"type": "object",
						},
					},
				},
			},
			expectedCode: http.StatusOK,
			minTokens:    50, // 包含工具定义的tokens
			maxTokens:    100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建请求
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// 执行处理器
			handler.CountTokens(c)

			// 验证响应代码
			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			// 解析响应
			if w.Code == http.StatusOK {
				var response models.CountTokensResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to parse response: %v", err)
				}

				// 验证token计数范围
				if response.InputTokens < tt.minTokens || response.InputTokens > tt.maxTokens {
					t.Errorf("Token count out of expected range: got %d, want %d-%d",
						response.InputTokens, tt.minTokens, tt.maxTokens)
				}
			}
		})
	}
}

func TestConvertToOldFormat(t *testing.T) {
	tests := []struct {
		name string
		req  *models.MessagesRequest
	}{
		{
			name: "Basic conversion",
			req: &models.MessagesRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{Role: "user", Content: "Hello"},
				},
				MaxTokens: 1024,
			},
		},
		{
			name: "With thinking config",
			req: &models.MessagesRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{Role: "user", Content: "Hello"},
				},
				MaxTokens: 1024,
				Thinking: &models.ThinkingConfig{
					Type:         "enabled",
					BudgetTokens: 1024,
				},
			},
		},
		{
			name: "With tools and metadata",
			req: &models.MessagesRequest{
				Model: "claude-3-opus-20240229",
				Messages: []models.Message{
					{Role: "user", Content: "Hello"},
				},
				MaxTokens: 1024,
				Metadata: &models.MessageMetadata{
					UserID: "test-user",
				},
				Tools: []models.Tool{
					{
						Name:        "test_tool",
						Description: "Test",
						InputSchema: map[string]interface{}{"type": "object"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToOldFormat(tt.req)

			// 验证基本字段
			if result.Model != tt.req.Model {
				t.Errorf("Model mismatch: got %s, want %s", result.Model, tt.req.Model)
			}
			if result.MaxTokens != tt.req.MaxTokens {
				t.Errorf("MaxTokens mismatch: got %d, want %d", result.MaxTokens, tt.req.MaxTokens)
			}
			if len(result.Messages) != len(tt.req.Messages) {
				t.Errorf("Messages count mismatch: got %d, want %d",
					len(result.Messages), len(tt.req.Messages))
			}

			// 验证Thinking配置转换
			if tt.req.Thinking != nil {
				if result.Thinking == nil {
					t.Error("Thinking config not converted")
				} else if result.Thinking.BudgetTokens != tt.req.Thinking.BudgetTokens {
					t.Errorf("Thinking budget mismatch: got %d, want %d",
						result.Thinking.BudgetTokens, tt.req.Thinking.BudgetTokens)
				}
			}

			// 验证工具转换
			if len(result.Tools) != len(tt.req.Tools) {
				t.Errorf("Tools count mismatch: got %d, want %d",
					len(result.Tools), len(tt.req.Tools))
			}
		})
	}
}
