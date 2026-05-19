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

// TestConvertToOldFormat_ToolSchema 测试工具schema是否正确转换
func TestConvertToOldFormat_ToolSchema(t *testing.T) {
	// 完整的bash工具schema（模拟Claude CLI发送的）
	bashSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in seconds",
			},
		},
		"required": []string{"command"},
	}

	req := &models.MessagesRequest{
		Model: "claude-3-opus-20240229",
		Messages: []models.Message{
			{Role: "user", Content: "列出当前目录"},
		},
		MaxTokens: 1024,
		Tools: []models.Tool{
			{
				Name:        "bash",
				Description: "Execute bash commands in the terminal",
				InputSchema: bashSchema,
				Type:        "client",
			},
		},
	}

	result := convertToOldFormat(req)

	// 验证工具是否正确转换
	if len(result.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result.Tools))
	}

	tool := result.Tools[0]
	if tool.Name != "bash" {
		t.Errorf("Tool name mismatch: got %s, want bash", tool.Name)
	}
	if tool.Description != "Execute bash commands in the terminal" {
		t.Errorf("Tool description mismatch: got %s", tool.Description)
	}
	if tool.InputSchema == nil {
		t.Error("Tool InputSchema is nil")
	} else {
		// 验证schema内容
		schemaType, ok := tool.InputSchema["type"].(string)
		if !ok || schemaType != "object" {
			t.Errorf("Schema type mismatch: got %v, want object", tool.InputSchema["type"])
		}

		properties, ok := tool.InputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Error("Schema properties not found or wrong type")
		} else {
			if _, hasCommand := properties["command"]; !hasCommand {
				t.Error("Schema missing 'command' property")
			}
		}
	}

	t.Logf("Tool conversion successful: %+v", tool)
}

// TestConvertToOldFormat_ClaueCLIScenario 模拟 Claude CLI 的完整场景
func TestConvertToOldFormat_ClaueCLIScenario(t *testing.T) {
	// 模拟 Claude CLI 发送的完整请求（包含系统提示、工具等）
	req := &models.MessagesRequest{
		Model: "claude-3-opus-20240229",
		Messages: []models.Message{
			{
				Role:    "user",
				Content: "列出当前目录下的所有文件",
			},
		},
		System: "You are Claude, a helpful AI assistant...",
		MaxTokens: 4096,
		Tools: []models.Tool{
			{
				Name:        "bash",
				Description: "Execute bash commands",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The bash command to execute",
						},
					},
					"required": []string{"command"},
				},
				Type: "client",
			},
			{
				Name:        "read",
				Description: "Read file contents",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file",
						},
					},
					"required": []string{"file_path"},
				},
				Type: "client",
			},
		},
		Metadata: &models.MessageMetadata{
			UserID: "claude-code-user",
		},
	}

	// 执行转换
	oldReq := convertToOldFormat(req)

	// 验证转换结果
	if oldReq.Model != req.Model {
		t.Errorf("Model mismatch: got %s, want %s", oldReq.Model, req.Model)
	}

	if len(oldReq.Messages) != len(req.Messages) {
		t.Errorf("Messages count mismatch: got %d, want %d",
			len(oldReq.Messages), len(req.Messages))
	}

	// 验证消息内容
	if str, ok := oldReq.Messages[0].Content.(string); !ok || str != "列出当前目录下的所有文件" {
		t.Errorf("Message content mismatch: got %v", oldReq.Messages[0].Content)
	}

	// 验证系统提示
	if sysStr, ok := oldReq.System.(string); !ok || sysStr != "You are Claude, a helpful AI assistant..." {
		t.Errorf("System prompt mismatch: got %v", oldReq.System)
	}

	// 验证工具转换
	if len(oldReq.Tools) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(oldReq.Tools))
	}

	// 验证第一个工具
	if oldReq.Tools[0].Name != "bash" {
		t.Errorf("First tool name: got %s, want bash", oldReq.Tools[0].Name)
	}

	// 验证第二个工具
	if oldReq.Tools[1].Name != "read" {
		t.Errorf("Second tool name: got %s, want read", oldReq.Tools[1].Name)
	}

	// 验证元数据
	if oldReq.Metadata == nil || oldReq.Metadata.UserID != "claude-code-user" {
		t.Errorf("Metadata mismatch: got %+v", oldReq.Metadata)
	}

	t.Logf("Claude CLI scenario conversion successful!")
	t.Logf("  Model: %s", oldReq.Model)
	t.Logf("  Messages: %d", len(oldReq.Messages))
	t.Logf("  Tools: %d", len(oldReq.Tools))
	for i, tool := range oldReq.Tools {
		t.Logf("    Tool[%d]: %s - %s", i, tool.Name, tool.Description)
	}
}

// TestConvertToOldFormat_ArrayContent 测试数组内容的消息转换
func TestConvertToOldFormat_ArrayContent(t *testing.T) {
	// 测试包含数组内容的消息（如 tool_result）
	req := &models.MessagesRequest{
		Model: "claude-3-opus-20240229",
		Messages: []models.Message{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_123",
						"content":     "file1.txt\nfile2.txt",
					},
				},
			},
		},
		MaxTokens: 1024,
	}

	oldReq := convertToOldFormat(req)

	// 验证数组内容是否正确传递
	if len(oldReq.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(oldReq.Messages))
	}

	content := oldReq.Messages[0].Content
	if content == nil {
		t.Error("Content is nil")
	}

	// 验证是数组类型
	if arr, ok := content.([]interface{}); ok {
		if len(arr) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(arr))
		}
		if block, ok := arr[0].(map[string]interface{}); ok {
			if block["type"] != "tool_result" {
				t.Errorf("Expected tool_result, got %v", block["type"])
			}
		}
	} else {
		t.Errorf("Content is not array: %T", content)
	}
}
func TestFormatContent(t *testing.T) {
	tests := []struct {
		name     string
		content  interface{}
		expected string
	}{
		{
			name:     "string content",
			content:  "Hello world",
			expected: "Hello world",
		},
		{
			name:    "long string",
			content: "This is a very long string that should be truncated for display purposes",
			expected: "This is a very long string that should be truncate...",
		},
		{
			name: "text block array",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "Hello from array"},
			},
			expected: "[1 blocks: Hello from array]",
		},
		{
			name: "tool use block",
			content: []interface{}{
				map[string]interface{}{"type": "tool_use", "name": "bash", "id": "test_id"},
			},
			expected: "[1 blocks: [tool:bash]]",
		},
		{
			name: "tool result block",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "call_123", "content": "result"},
			},
			expected: "[1 blocks: [tool_result]]",
		},
		{
			name:     "nil content",
			content:  nil,
			expected: "<nil>",
		},
		{
			name:     "int content",
			content:  42,
			expected: "<int>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatContent(tt.content)
			if got != tt.expected {
				t.Errorf("formatContent() = %q, want %q", got, tt.expected)
			}
		})
	}
}
