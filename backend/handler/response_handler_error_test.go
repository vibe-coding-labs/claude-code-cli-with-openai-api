package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// overloadedBody 是 SendErrorResponse 返回的 overloaded_error 响应体结构。
type overloadedBody struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// TestSendErrorResponse_424_ReturnsOverloadedError 验证 424 被兜底转成
// overloaded_error + 503，使 Claude Code 客户端能自动重试而不中断。
func TestSendErrorResponse_424_ReturnsOverloadedError(t *testing.T) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	rh.SendErrorResponse(c, fmt.Errorf("OpenAI API error (status 424): Upstream service temporarily unavailable"))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	var body overloadedBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v (body=%s)", err, w.Body.String())
	}
	if body.Error.Type != "overloaded_error" {
		t.Errorf("error.type = %q, want overloaded_error", body.Error.Type)
	}
}

// TestSendErrorResponse_424_WrappedByRetryEngine 验证被 retry engine 包装的 424
// 错误（"all retry attempts failed, last error: OpenAI API error (status 424): ..."）
// 仍能正确提取出 424 并转成 overloaded_error。这是线上真实场景：client 层重试耗尽后，
// 错误被 fmt.Errorf("...%w") 包装，若仅靠前缀匹配会提取不到状态码导致兜底失效。
func TestSendErrorResponse_424_WrappedByRetryEngine(t *testing.T) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	wrappedErr := fmt.Errorf("all retry attempts failed, last error: OpenAI API error (status 424): {\"error\":{\"message\":\"Upstream service temporarily unavailable\"}}")
	rh.SendErrorResponse(c, wrappedErr)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (424 must be extracted from wrapped error)", w.Code)
	}
	var body overloadedBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v (body=%s)", err, w.Body.String())
	}
	if body.Error.Type != "overloaded_error" {
		t.Errorf("error.type = %q, want overloaded_error", body.Error.Type)
	}
}

// TestSendErrorResponse_401_NotOverloaded 验证非过载码（如 401）保持原状态码
// 与 api_error 类型，不被错误地转成 overloaded_error。
func TestSendErrorResponse_401_NotOverloaded(t *testing.T) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	rh.SendErrorResponse(c, fmt.Errorf("OpenAI API error (status 401): invalid_api_key"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	var body overloadedBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v (body=%s)", err, w.Body.String())
	}
	if body.Error.Type != "api_error" {
		t.Errorf("error.type = %q, want api_error", body.Error.Type)
	}
}
