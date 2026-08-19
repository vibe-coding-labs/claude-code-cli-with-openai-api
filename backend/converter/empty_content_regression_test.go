package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestRegression_EmptyTextResponse_NeverYieldsBareTypeText 复刻线上 9139 场景：
// 上游返回 output_tokens=1 的空文本响应，走非流式转换 + 序列化。
// 修复前载荷为 {"type":"text"}（缺 text 字段），会让 Claude Code 的
// o.text.trim() 抛 "undefined is not an object"。
func TestRegression_EmptyTextResponse_NeverYieldsBareTypeText(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:    "chatcmpl-9139",
		Model: "deepseek-v4-pro",
		Choices: []models.OpenAIChoice{
			{
				Message:      models.OpenAIMessage{Content: ""},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{PromptTokens: 39625, CompletionTokens: 1},
	}
	origReq := &models.ClaudeMessagesRequest{Model: "deepseek-v4-pro", Stream: false}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, origReq)
	if claudeResp == nil {
		t.Fatal("ConvertOpenAIToClaudeResponse returned nil")
	}
	if len(claudeResp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(claudeResp.Content))
	}

	// 序列化载荷必须包含 text 字段（即使为空字符串），绝不允许 {"type":"text"}
	raw, err := json.Marshal(claudeResp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `{"type":"text"}`) && !strings.Contains(s, `"text":""`) {
		t.Fatalf("payload contains bare {\"type\":\"text\"} which crashes Claude Code: %s", s)
	}
	if !strings.Contains(s, `"text":""`) {
		t.Errorf("payload missing text field on text block: %s", s)
	}
	t.Logf("serialized payload: %s", s)

	// 空内容检测必须拦截（ValidateResponseContent 由 Task 2 挂载到主非流式路径）
	if !GetDegenerateDetector().IsEmptyOrWhitespace(claudeResp.Content[0].Text) {
		t.Errorf("converted text should be empty/whitespace")
	}
}
