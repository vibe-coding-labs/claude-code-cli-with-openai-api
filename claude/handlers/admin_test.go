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

func TestAdminHandler_GetMe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewAdminHandler(cfg)

	tests := []struct {
		name          string
		configID      string
		expectedOrgID string
	}{
		{
			name:          "Get default organization",
			configID:      "",
			expectedOrgID: "org_default",
		},
		{
			name:          "Get custom config organization",
			configID:      "custom",
			expectedOrgID: "org_custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/me", nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			if tt.configID != "" {
				c.Params = []gin.Param{{Key: "id", Value: tt.configID}}
			}

			handler.GetMe(c)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
			}

			var response models.OrganizationResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
			}

			// 验证组织信息
			if response.ID != tt.expectedOrgID {
				t.Errorf("Expected org ID %s, got %s", tt.expectedOrgID, response.ID)
			}

			if response.Type != "organization" {
				t.Errorf("Expected type 'organization', got '%s'", response.Type)
			}

			if response.Name == "" {
				t.Error("Organization name should not be empty")
			}
		})
	}
}

func TestAdminHandler_GetOrganizationUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	handler := NewAdminHandler(cfg)

	tests := []struct {
		name  string
		orgID string
	}{
		{
			name:  "Get usage for default org",
			orgID: "org_default",
		},
		{
			name:  "Get usage for custom org",
			orgID: "org_custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/organizations/"+tt.orgID+"/usage", nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "org_id", Value: tt.orgID}}

			handler.GetOrganizationUsage(c)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
			}

			var response models.OrganizationUsageResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
			}

			// 验证使用情况响应
			if response.Object != "usage" {
				t.Errorf("Expected object 'usage', got '%s'", response.Object)
			}

			if len(response.Data) == 0 {
				t.Error("Expected at least one usage entry")
			}

			// 验证包含的使用条目类型
			hasBalance := false
			hasCreditsUsed := false
			hasTokensUsed := false

			for _, entry := range response.Data {
				switch entry.Type {
				case "credit_balance":
					hasBalance = true
					if entry.CreditBalance <= 0 {
						t.Error("Credit balance should be positive")
					}
				case "credits_used":
					hasCreditsUsed = true
				case "tokens_used":
					hasTokensUsed = true
				}
			}

			if !hasBalance {
				t.Error("Expected credit_balance entry in usage data")
			}

			if !hasCreditsUsed {
				t.Error("Expected credits_used entry in usage data")
			}

			if !hasTokensUsed {
				t.Error("Expected tokens_used entry in usage data")
			}
		})
	}
}

// TestGetMeForClaudeCodeCLI 测试Claude Code CLI调用GetMe端点的场景
func TestGetMeForClaudeCodeCLI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		AnthropicAPIKey: "test-api-key",
	}
	handler := NewAdminHandler(cfg)

	// 模拟Claude Code CLI的请求
	req := httptest.NewRequest("GET", "/v1/me", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("User-Agent", "Claude-Code-CLI/1.0")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetMe(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		t.Logf("Response body: %s", w.Body.String())
	}

	var response models.OrganizationResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	// 验证响应格式符合Claude Code CLI的期望
	if response.ID == "" {
		t.Error("Organization ID should not be empty")
	}

	if response.Name == "" {
		t.Error("Organization name should not be empty")
	}

	if response.Type != "organization" {
		t.Errorf("Expected type 'organization', got '%s'", response.Type)
	}

	// 确保响应可以被Claude Code CLI正确解析
	// Claude Code CLI期望的响应格式：
	// {
	//   "id": "org_xxx",
	//   "name": "Organization Name",
	//   "type": "organization"
	// }

	// 验证响应包含所有必需字段
	responseMap := make(map[string]interface{})
	json.Unmarshal(w.Body.Bytes(), &responseMap)

	requiredFields := []string{"id", "name", "type"}
	for _, field := range requiredFields {
		if _, exists := responseMap[field]; !exists {
			t.Errorf("Required field '%s' missing in response", field)
		}
	}
}
