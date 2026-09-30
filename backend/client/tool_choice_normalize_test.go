package client

import (
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestNormalizeToolChoiceForRetry 覆盖 apibest.ai 类上游拒绝标准嵌套 tool_choice
// （Missing required parameter: 'tool_choice.name'）时，改写为扁平 Responses-API 形状
// 后重试一次的场景，以及不应触发改写的场景。
func TestNormalizeToolChoiceForRetry(t *testing.T) {
	nestedChoice := map[string]interface{}{
		"type": "function",
		"function": map[string]string{
			"name": "get_weather",
		},
	}
	errBody := `{"error":{"message":"Missing required parameter: 'tool_choice.name'.","type":"invalid_request_error","param":"","code":null}}`

	t.Run("rewrites nested tool_choice to flat shape on matching error", func(t *testing.T) {
		req := &models.OpenAIRequest{Model: "gpt-6-astra", ToolChoice: nestedChoice}
		rewritten, changed := normalizeToolChoiceForRetry(req, errBody)
		if !changed {
			t.Fatalf("expected changed=true")
		}
		choiceMap, ok := rewritten.ToolChoice.(map[string]interface{})
		if !ok {
			t.Fatalf("expected ToolChoice to be map[string]interface{}, got %T", rewritten.ToolChoice)
		}
		if choiceMap["type"] != "function" {
			t.Errorf("expected type=function, got %v", choiceMap["type"])
		}
		if choiceMap["name"] != "get_weather" {
			t.Errorf("expected flat name=get_weather, got %v", choiceMap["name"])
		}
		if _, hasNestedFunction := choiceMap["function"]; hasNestedFunction {
			t.Errorf("expected nested function key removed, still present: %v", choiceMap)
		}
		// original request must not be mutated
		if origMap := req.ToolChoice.(map[string]interface{}); origMap["function"] == nil {
			t.Errorf("original request's ToolChoice must remain unmodified")
		}
	})

	t.Run("does not rewrite on unrelated error", func(t *testing.T) {
		req := &models.OpenAIRequest{Model: "gpt-6-astra", ToolChoice: nestedChoice}
		_, changed := normalizeToolChoiceForRetry(req, `{"error":{"message":"invalid model"}}`)
		if changed {
			t.Fatalf("expected changed=false for unrelated error")
		}
	})

	t.Run("does not rewrite when tool_choice is a plain string (auto/none/required)", func(t *testing.T) {
		req := &models.OpenAIRequest{Model: "gpt-6-astra", ToolChoice: "auto"}
		_, changed := normalizeToolChoiceForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false for string tool_choice")
		}
	})

	t.Run("does not rewrite when tool_choice is nil", func(t *testing.T) {
		req := &models.OpenAIRequest{Model: "gpt-6-astra"}
		_, changed := normalizeToolChoiceForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false for nil tool_choice")
		}
	})

	t.Run("already-flat tool_choice has no name to extract, no rewrite", func(t *testing.T) {
		alreadyFlat := map[string]interface{}{"type": "function", "name": "get_weather"}
		req := &models.OpenAIRequest{Model: "gpt-6-astra", ToolChoice: alreadyFlat}
		_, changed := normalizeToolChoiceForRetry(req, errBody)
		if changed {
			t.Fatalf("expected changed=false when already flat (no nested function.name to extract)")
		}
	})
}
