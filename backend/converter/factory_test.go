package converter

import (
	"encoding/json"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func TestConverterFactory_ConvertClaudeToInternal(t *testing.T) {
	factory := NewConverterFactory()

	claudeBody := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_tokens": 1024
	}`)

	req, err := factory.ConvertClaudeToInternal(claudeBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %q", req.Model)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", req.MaxTokens)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
}

func TestConverterFactory_ConvertInternalToOpenAI(t *testing.T) {
	factory := NewConverterFactory()
	factory.SetOpenAIConfig(&config.Config{
		BigModel:   "gpt-4",
		SmallModel: "gpt-3.5-turbo",
	})

	req := &InternalRequest{
		Model: "claude-3-opus-20240229",
		Messages: []InternalMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}},
		},
	}

	openAIBody, err := factory.ConvertInternalToOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var openAIReq map[string]interface{}
	if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Model should be mapped using config
	if openAIReq["model"] == "" {
		t.Error("expected model to be set")
	}
	if openAIReq["messages"] == nil {
		t.Error("expected messages to be set")
	}
}

func TestConverterFactory_ConvertOpenAIToInternal(t *testing.T) {
	factory := NewConverterFactory()

	openAIResp := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello there!"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`)

	resp, err := factory.ConvertOpenAIToInternal(openAIResp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected id chatcmpl-123, got %q", resp.ID)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role assistant, got %q", resp.Role)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %q", resp.StopReason)
	}
}

func TestConverterFactory_ConvertInternalToClaude(t *testing.T) {
	factory := NewConverterFactory()

	resp := &InternalResponse{
		ID:         "msg_123",
		Model:      "claude-sonnet-4-6",
		Role:       "assistant",
		StopReason: "end_turn",
		Content: []ContentBlock{
			{Type: "text", Text: "Hello there!"},
		},
		Usage: &UsageInfo{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	claudeBody, err := factory.ConvertInternalToClaude(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var claudeResp map[string]interface{}
	if err := json.Unmarshal(claudeBody, &claudeResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if claudeResp["id"] != "msg_123" {
		t.Errorf("expected id msg_123, got %v", claudeResp["id"])
	}
	if claudeResp["role"] != "assistant" {
		t.Errorf("expected role assistant, got %v", claudeResp["role"])
	}
}

func TestConverterFactory_ConvertClaudeToOpenAI_Full(t *testing.T) {
	factory := NewConverterFactory()
	cfg := &config.Config{
		BigModel:   "gpt-4",
		SmallModel: "gpt-3.5-turbo",
	}

	claudeBody := []byte(`{
		"model": "claude-3-opus-20240229",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_tokens": 1024
	}`)

	openAIBody, internalReq, err := factory.ConvertClaudeToOpenAI(claudeBody, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if internalReq == nil {
		t.Fatal("expected internalReq to be non-nil")
	}
	if internalReq.Model != "claude-3-opus-20240229" {
		t.Errorf("expected model claude-3-opus-20240229, got %q", internalReq.Model)
	}

	var openAIReq map[string]interface{}
	if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if openAIReq["model"] == "" {
		t.Error("expected model to be set in OpenAI request")
	}
}

func TestConverterFactory_ConvertOpenAIToClaude_Full(t *testing.T) {
	factory := NewConverterFactory()

	originalReq := &InternalRequest{
		Model: "claude-sonnet-4-6",
	}

	openAIResp := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello there!"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`)

	claudeBody, internalResp, err := factory.ConvertOpenAIToClaude(openAIResp, originalReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if internalResp == nil {
		t.Fatal("expected internalResp to be non-nil")
	}
	// Model should be set from original request
	if internalResp.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %q", internalResp.Model)
	}

	var claudeResp map[string]interface{}
	if err := json.Unmarshal(claudeBody, &claudeResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if claudeResp["role"] != "assistant" {
		t.Errorf("expected role assistant, got %v", claudeResp["role"])
	}
}

func TestConverterFactory_GetConverters(t *testing.T) {
	factory := NewConverterFactory()

	claudeConverter := factory.GetClaudeConverter()
	if claudeConverter == nil {
		t.Error("expected GetClaudeConverter to return non-nil")
	}

	openAIConverter := factory.GetOpenAIConverter()
	if openAIConverter == nil {
		t.Error("expected GetOpenAIConverter to return non-nil")
	}
}

func TestGlobalFactory(t *testing.T) {
	// Test that GlobalFactory is initialized
	if GlobalFactory == nil {
		t.Fatal("GlobalFactory should be initialized")
	}

	// Test that we can use it
	claudeBody := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_tokens": 1024
	}`)

	req, err := GlobalFactory.ConvertClaudeToInternal(claudeBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %q", req.Model)
	}
}
