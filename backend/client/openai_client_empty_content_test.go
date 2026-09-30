package client

import (
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestIsEmptyOpenAIMessageContent 锁定 Content(interface{}) 与 ToolCalls 的空内容判定。
// 核心回归: goaichat/xinli 对长请求返回 {"choices":[{message:{content:""}}]}——
// 1 个 choice、content 为空字符串。该 case 必须判空(触发服务端重试),否则被当成功返回
// 导致"每30秒一次 empty response"死循环。
func TestIsEmptyOpenAIMessageContent(t *testing.T) {
	tests := []struct {
		name string
		msg  models.OpenAIMessage
		want bool
	}{
		{"nil content is empty", models.OpenAIMessage{Content: nil}, true},
		{"empty string content is empty", models.OpenAIMessage{Content: ""}, true},
		{"whitespace string content is empty", models.OpenAIMessage{Content: "   \n\t "}, true},
		{"non-empty string content is not empty", models.OpenAIMessage{Content: "hello"}, false},
		{"empty block array is empty", models.OpenAIMessage{Content: []interface{}{}}, true},
		{
			"block array with empty text is empty",
			models.OpenAIMessage{Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "   "},
			}},
			true,
		},
		{
			"block array with non-empty text is not empty",
			models.OpenAIMessage{Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "real answer"},
			}},
			false,
		},
		{
			"tool-only response is not empty (valid tool call)",
			models.OpenAIMessage{
				Content:   "", // no text
				ToolCalls: []models.OpenAIToolCall{{ID: "call_1"}},
			},
			false,
		},
		{"non-string content treated as non-empty", models.OpenAIMessage{Content: 42}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyOpenAIMessageContent(tt.msg); got != tt.want {
				t.Errorf("isEmptyOpenAIMessageContent(%v) = %v, want %v", tt.msg.Content, got, tt.want)
			}
		})
	}
}

// TestIsOpenAIReponseEmptyContent 覆盖多/单 choice 的聚合判定。
func TestIsOpenAIReponseEmptyContent(t *testing.T) {
	tests := []struct {
		name string
		resp *models.OpenAIResponse
		want bool
	}{
		{"nil response not empty (handled by len==0 branch)", nil, false},
		{"zero choices not empty (handled by len==0 branch)", &models.OpenAIResponse{Choices: []models.OpenAIChoice{}}, false},
		{
			"one empty-content choice is empty (the goaichat bug case)",
			&models.OpenAIResponse{Choices: []models.OpenAIChoice{
				{Message: models.OpenAIMessage{Content: ""}},
			}},
			true,
		},
		{
			"one real-content choice is not empty",
			&models.OpenAIResponse{Choices: []models.OpenAIChoice{
				{Message: models.OpenAIMessage{Content: "answer"}},
			}},
			false,
		},
		{
			"one tool-only choice is not empty",
			&models.OpenAIResponse{Choices: []models.OpenAIChoice{
				{Message: models.OpenAIMessage{ToolCalls: []models.OpenAIToolCall{{ID: "call_2"}}}},
			}},
			false,
		},
		{
			"multiple choices all empty is empty",
			&models.OpenAIResponse{Choices: []models.OpenAIChoice{
				{Message: models.OpenAIMessage{Content: ""}},
				{Message: models.OpenAIMessage{Content: nil}},
			}},
			true,
		},
		{
			"multiple choices with one valid is not empty",
			&models.OpenAIResponse{Choices: []models.OpenAIChoice{
				{Message: models.OpenAIMessage{Content: ""}},
				{Message: models.OpenAIMessage{Content: "value"}},
			}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOpenAIReponseEmptyContent(tt.resp); got != tt.want {
				t.Errorf("isOpenAIReponseEmptyContent() = %v, want %v", got, tt.want)
			}
		})
	}
}
