package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	dir, err := os.MkdirTemp("", "handler-validation-*")
	if err != nil {
		panic(err)
	}
	// 拒绝路径的 ValidateResponseContent 会调用 logRequestWithDetails → database.LogRequest，
	// 需要有效 DB 句柄，否则异步 logger 的 worker goroutine 会在 nil DB 上 panic。
	if err := database.InitDB(filepath.Join(dir, "test.db")); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func newValidationContext() (*ResponseHandler, *gin.Context, *httptest.ResponseRecorder) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return rh, c, w
}

// TestValidateResponseContent_EmptyText_Rejected 空文本块必须被拦截为 overloaded_error，
// 否则会被序列化成 {"type":"text"} 导致 Claude Code o.text.trim 崩溃。
func TestValidateResponseContent_EmptyText_Rejected(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{Type: "text", Text: ""}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, &models.ClaudeMessagesRequest{Model: "deepseek-v4-pro"}, nil)
	if !rejected {
		t.Errorf("empty text content should be rejected")
	}
	if c.Writer.Status() != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", c.Writer.Status())
	}
}

// TestValidateResponseContent_NormalText_Allowed 有内容的文本块必须放行。
func TestValidateResponseContent_NormalText_Allowed(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{Type: "text", Text: "hello"}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, nil, nil)
	if rejected {
		t.Errorf("normal text content should be allowed")
	}
}

// TestValidateResponseContent_ToolOnly_Allowed 纯工具调用响应必须放行（不是空响应）。
func TestValidateResponseContent_ToolOnly_Allowed(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{
			Type:  "tool_use",
			ID:    "toolu_1",
			Name:  "bash",
			Input: map[string]interface{}{"command": "ls"},
		}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, nil, nil)
	if rejected {
		t.Errorf("tool-only response should be allowed")
	}
}

// TestValidateResponseContent_Degenerate_Rejected 伪工具调用标记的文本必须被拦截。
func TestValidateResponseContent_Degenerate_Rejected(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{Type: "text", Text: "</｜DSML｜invoke>"}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, nil, nil)
	if !rejected {
		t.Errorf("degenerate DSML pseudo-tool-call text should be rejected")
	}
}
