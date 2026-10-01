package converter

// Regression test for legacyConvertOpenAIToClaude's stop_reason derivation —
// the panic-safety fallback path for ConvertOpenAIToClaudeResponse. Mirrors
// TestOpenAIConverter_ParseResponse_ToolCallsWithStopFinishReason in
// openai_test.go, which covers the same bug in the primary (factory) path.

import (
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestLegacyConvertOpenAIToClaude_ToolCallsWithStopFinishReason(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-legacy-1",
		Choices: []models.OpenAIChoice{{
			Message: models.OpenAIMessage{
				Role: "assistant",
				ToolCalls: []models.OpenAIToolCall{{
					ID:   "call_xyz",
					Type: "function",
					Function: models.OpenAIFunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Berlin"}`,
					},
				}},
			},
			FinishReason: "stop",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 20, CompletionTokens: 15},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "glm-5.2"}

	result := legacyConvertOpenAIToClaude(openAIResp, originalReq)

	if result.StopReason != models.StopToolUse {
		t.Errorf("expected stop_reason tool_use (derived from tool_calls, not finish_reason=stop), got %q", result.StopReason)
	}

	foundToolUse := false
	for _, cb := range result.Content {
		if cb.Type == models.ContentToolUse {
			foundToolUse = true
		}
	}
	if !foundToolUse {
		t.Error("expected a tool_use content block")
	}
}

func TestLegacyConvertOpenAIToClaude_LengthFinishReasonNotOverriddenByToolCalls(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-legacy-2",
		Choices: []models.OpenAIChoice{{
			Message: models.OpenAIMessage{
				Role: "assistant",
				ToolCalls: []models.OpenAIToolCall{{
					ID:   "call_trunc",
					Type: "function",
					Function: models.OpenAIFunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Berl`,
					},
				}},
			},
			FinishReason: "length",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 20, CompletionTokens: 15},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "glm-5.2"}

	result := legacyConvertOpenAIToClaude(openAIResp, originalReq)

	if result.StopReason != models.StopMaxTokens {
		t.Errorf("expected stop_reason max_tokens to survive despite present tool_calls, got %q", result.StopReason)
	}
}
