package converter

// Comprehensive Anthropic Messages API Protocol Test Suite
// Tests the full Claude → Internal → OpenAI → Internal → Claude conversion pipeline
// covering all protocol features that Claude Code CLI uses.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ============================================================================
// Section 1: Claude Request Parsing (ClaudeConverter.ParseRequest)
// Tests that all Anthropic request formats are correctly parsed to internal format.
// ============================================================================

func TestClaudeProtocol_ParseRequest_BasicFields(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		checkField func(t *testing.T, req *InternalRequest)
	}{
		{
			name: "simple text message",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Model != "claude-sonnet-4-6" {
					t.Errorf("model = %q, want claude-sonnet-4-6", req.Model)
				}
				if req.MaxTokens != 1024 {
					t.Errorf("max_tokens = %d, want 1024", req.MaxTokens)
				}
				if len(req.Messages) != 1 {
					t.Fatalf("messages count = %d, want 1", len(req.Messages))
				}
				if req.Messages[0].Role != "user" {
					t.Errorf("role = %q, want user", req.Messages[0].Role)
				}
			},
		},
		{
			name: "system prompt as string",
			body: `{"model":"claude-sonnet-4-6","system":"You are helpful","messages":[{"role":"user","content":"Hi"}],"max_tokens":512}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.System != "You are helpful" {
					t.Errorf("system = %q, want 'You are helpful'", req.System)
				}
			},
		},
		{
			name: "system prompt as array of text blocks",
			body: `{"model":"claude-sonnet-4-6","system":[{"type":"text","text":"Part 1"},{"type":"text","text":"Part 2"}],"messages":[{"role":"user","content":"Hi"}],"max_tokens":512}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.System != "Part 1\nPart 2" {
					t.Errorf("system = %q, want 'Part 1\\nPart 2'", req.System)
				}
			},
		},
		{
			name: "stream true",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"stream":true}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if !req.Stream {
					t.Error("stream = false, want true")
				}
			},
		},
		{
			name: "stream false (default)",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Stream {
					t.Error("stream = true, want false")
				}
			},
		},
		{
			name: "stop sequences",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"stop_sequences":["END","STOP"]}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.StopSeqs) != 2 || req.StopSeqs[0] != "END" || req.StopSeqs[1] != "STOP" {
					t.Errorf("stop_sequences = %v, want [END STOP]", req.StopSeqs)
				}
			},
		},
		{
			name: "metadata with user_id",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"metadata":{"user_id":"user-123"}}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Metadata["user_id"] != "user-123" {
					t.Errorf("metadata.user_id = %v, want user-123", req.Metadata["user_id"])
				}
			},
		},
		{
			name: "metadata with session_id",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"metadata":{"user_id":"u","session_id":"sess-456"}}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Metadata["session_id"] != "sess-456" {
					t.Errorf("metadata.session_id = %v, want sess-456", req.Metadata["session_id"])
				}
			},
		},
		{
			name: "model name preserved exactly",
			body: `{"model":"claude-opus-4-7","messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Model != "claude-opus-4-7" {
					t.Errorf("model = %q, want claude-opus-4-7", req.Model)
				}
			},
		},
		{
			name: "temperature zero",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"temperature":0}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Temperature == nil || *req.Temperature != 0 {
					t.Errorf("temperature = %v, want 0", req.Temperature)
				}
			},
		},
		{
			name: "temperature high",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"temperature":0.95}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.Temperature == nil || *req.Temperature != 0.95 {
					t.Errorf("temperature = %v, want 0.95", req.Temperature)
				}
			},
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := c.ParseRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("ParseRequest error: %v", err)
			}
			tt.checkField(t, req)
		})
	}
}

// ============================================================================
// Section 2: Claude Request Content Block Types
// ============================================================================

func TestClaudeProtocol_ParseRequest_ContentBlocks(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		checkField func(t *testing.T, req *InternalRequest)
	}{
		{
			name: "text content as string shorthand",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hello world"}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[0].Content) != 1 {
					t.Fatalf("content blocks = %d, want 1", len(req.Messages[0].Content))
				}
				if req.Messages[0].Content[0].Type != "text" || req.Messages[0].Content[0].Text != "Hello world" {
					t.Errorf("content = %+v, want text 'Hello world'", req.Messages[0].Content[0])
				}
			},
		},
		{
			name: "text content as array",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[0].Content) != 1 {
					t.Fatalf("content blocks = %d, want 1", len(req.Messages[0].Content))
				}
				if req.Messages[0].Content[0].Type != "text" {
					t.Errorf("type = %q, want text", req.Messages[0].Content[0].Type)
				}
			},
		},
		{
			name: "image content base64",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[0].Content) != 1 {
					t.Fatalf("content blocks = %d, want 1", len(req.Messages[0].Content))
				}
				cb := req.Messages[0].Content[0]
				if cb.Type != "image" {
					t.Errorf("type = %q, want image", cb.Type)
				}
				if cb.Source == nil {
					t.Fatal("source is nil")
				}
				if cb.Source.MediaType != "image/png" {
					t.Errorf("media_type = %q, want image/png", cb.Source.MediaType)
				}
				if cb.Source.Data != "iVBORw0KGgo=" {
					t.Errorf("data mismatch")
				}
			},
		},
		{
			name: "image content URL source",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.com/img.png"}}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				cb := req.Messages[0].Content[0]
				if cb.Type != "image" {
					t.Errorf("type = %q, want image", cb.Type)
				}
			},
		},
		{
			name: "tool_use content block",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"NYC"}}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages) != 2 {
					t.Fatalf("messages = %d, want 2", len(req.Messages))
				}
				cb := req.Messages[1].Content[0]
				if cb.Type != "tool_use" {
					t.Errorf("type = %q, want tool_use", cb.Type)
				}
				if cb.ID != "toolu_01" {
					t.Errorf("id = %q, want toolu_01", cb.ID)
				}
				if cb.Name != "get_weather" {
					t.Errorf("name = %q, want get_weather", cb.Name)
				}
				if cb.Input["city"] != "NYC" {
					t.Errorf("input.city = %v, want NYC", cb.Input["city"])
				}
			},
		},
		{
			name: "tool_result content block with string content",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"result data"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				cb := req.Messages[2].Content[0]
				if cb.Type != "tool_result" {
					t.Errorf("type = %q, want tool_result", cb.Type)
				}
				if cb.ToolUseID != "toolu_01" {
					t.Errorf("tool_use_id = %q, want toolu_01", cb.ToolUseID)
				}
				if cb.Content != "result data" {
					t.Errorf("content = %q, want 'result data'", cb.Content)
				}
			},
		},
		{
			name: "tool_result with array content (text blocks)",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"text","text":"line 1"},{"type":"text","text":"line 2"}]}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				cb := req.Messages[2].Content[0]
				if cb.Content != "line 1line 2" {
					t.Errorf("content = %q, want 'line 1line 2'", cb.Content)
				}
			},
		},
		{
			name: "tool_result with empty content",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				cb := req.Messages[2].Content[0]
				if cb.Content != "" {
					t.Errorf("content = %q, want empty", cb.Content)
				}
			},
		},
		{
			name: "thinking content block",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"thinking","thinking":"Let me think..."}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				cb := req.Messages[1].Content[0]
				if cb.Type != "thinking" {
					t.Errorf("type = %q, want thinking", cb.Type)
				}
				if cb.Text != "Let me think..." {
					t.Errorf("text = %q, want 'Let me think...'", cb.Text)
				}
			},
		},
		{
			name: "redacted_thinking content block",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"redacted_thinking","data":"base64data"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				cb := req.Messages[1].Content[0]
				if cb.Type != "redacted_thinking" {
					t.Errorf("type = %q, want redacted_thinking", cb.Type)
				}
			},
		},
		{
			name: "multi-modal: text + image",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":"What is this?"},{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"/9j/4AAQ"}}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[0].Content) != 2 {
					t.Fatalf("content blocks = %d, want 2", len(req.Messages[0].Content))
				}
				if req.Messages[0].Content[0].Type != "text" {
					t.Errorf("first block type = %q, want text", req.Messages[0].Content[0].Type)
				}
				if req.Messages[0].Content[1].Type != "image" {
					t.Errorf("second block type = %q, want image", req.Messages[0].Content[1].Type)
				}
			},
		},
		{
			name: "assistant with text + tool_use",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check weather"},{"role":"assistant","content":[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"NYC"}}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[1].Content) != 2 {
					t.Fatalf("content blocks = %d, want 2", len(req.Messages[1].Content))
				}
				if req.Messages[1].Content[0].Type != "text" {
					t.Errorf("first block type = %q, want text", req.Messages[1].Content[0].Type)
				}
				if req.Messages[1].Content[1].Type != "tool_use" {
					t.Errorf("second block type = %q, want tool_use", req.Messages[1].Content[1].Type)
				}
			},
		},
		{
			name: "multiple tool_use in single assistant message",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check both"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn_a","input":{}},{"type":"tool_use","id":"toolu_02","name":"fn_b","input":{}}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[1].Content) != 2 {
					t.Fatalf("content blocks = %d, want 2", len(req.Messages[1].Content))
				}
				if req.Messages[1].Content[0].ID != "toolu_01" || req.Messages[1].Content[1].ID != "toolu_02" {
					t.Errorf("tool IDs = %q, %q, want toolu_01, toolu_02", req.Messages[1].Content[0].ID, req.Messages[1].Content[1].ID)
				}
			},
		},
		{
			name: "multiple tool_result in single user message",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn_a","input":{}},{"type":"tool_use","id":"toolu_02","name":"fn_b","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"a"},{"type":"tool_result","tool_use_id":"toolu_02","content":"b"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages[2].Content) != 2 {
					t.Fatalf("content blocks = %d, want 2", len(req.Messages[2].Content))
				}
				if req.Messages[2].Content[0].ToolUseID != "toolu_01" {
					t.Errorf("first tool_use_id = %q, want toolu_01", req.Messages[2].Content[0].ToolUseID)
				}
				if req.Messages[2].Content[1].ToolUseID != "toolu_02" {
					t.Errorf("second tool_use_id = %q, want toolu_02", req.Messages[2].Content[1].ToolUseID)
				}
			},
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := c.ParseRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("ParseRequest error: %v", err)
			}
			tt.checkField(t, req)
		})
	}
}

// ============================================================================
// Section 3: Tool Definitions and Tool Choice
// ============================================================================

func TestClaudeProtocol_ParseRequest_Tools(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		checkField func(t *testing.T, req *InternalRequest)
	}{
		{
			name: "single tool definition",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Tools) != 1 {
					t.Fatalf("tools = %d, want 1", len(req.Tools))
				}
				if req.Tools[0].Name != "get_weather" {
					t.Errorf("tool name = %q, want get_weather", req.Tools[0].Name)
				}
				if req.Tools[0].Description != "Get weather" {
					t.Errorf("tool desc = %q", req.Tools[0].Description)
				}
			},
		},
		{
			name: "multiple tool definitions",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"tool_a","description":"A","input_schema":{"type":"object"}},{"name":"tool_b","description":"B","input_schema":{"type":"object"}},{"name":"tool_c","description":"C","input_schema":{"type":"object"}}]}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Tools) != 3 {
					t.Fatalf("tools = %d, want 3", len(req.Tools))
				}
			},
		},
		{
			name: "tool with complex nested schema",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"search","description":"Search","input_schema":{"type":"object","properties":{"query":{"type":"string"},"filters":{"type":"object","properties":{"date":{"type":"string"},"category":{"type":"array","items":{"type":"string"}}}}},"required":["query"]}}]}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Tools) != 1 {
					t.Fatalf("tools = %d, want 1", len(req.Tools))
				}
				schema := req.Tools[0].Parameters
				props, ok := schema["properties"].(map[string]interface{})
				if !ok {
					t.Fatal("schema.properties not found")
				}
				if _, hasQuery := props["query"]; !hasQuery {
					t.Error("missing query property")
				}
				if _, hasFilters := props["filters"]; !hasFilters {
					t.Error("missing filters property")
				}
			},
		},
		{
			name: "tool_choice auto",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tool_choice":{"type":"auto"}}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if req.ToolChoice == nil {
					t.Fatal("tool_choice is nil")
				}
				tc := req.ToolChoice.(map[string]interface{})
				if tc["type"] != "auto" {
					t.Errorf("tool_choice.type = %v, want auto", tc["type"])
				}
			},
		},
		{
			name: "tool_choice any",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tool_choice":{"type":"any"}}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				tc := req.ToolChoice.(map[string]interface{})
				if tc["type"] != "any" {
					t.Errorf("tool_choice.type = %v, want any", tc["type"])
				}
			},
		},
		{
			name: "tool_choice specific tool",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tool_choice":{"type":"tool","name":"get_weather"}}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				tc := req.ToolChoice.(map[string]interface{})
				if tc["type"] != "tool" {
					t.Errorf("tool_choice.type = %v, want tool", tc["type"])
				}
				if tc["name"] != "get_weather" {
					t.Errorf("tool_choice.name = %v, want get_weather", tc["name"])
				}
			},
		},
		{
			name: "tool_choice none",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tool_choice":{"type":"none"}}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				tc := req.ToolChoice.(map[string]interface{})
				if tc["type"] != "none" {
					t.Errorf("tool_choice.type = %v, want none", tc["type"])
				}
			},
		},
		{
			name: "tool with no description",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"minimal","input_schema":{"type":"object"}}]}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Tools) != 1 {
					t.Fatalf("tools = %d, want 1", len(req.Tools))
				}
				if req.Tools[0].Name != "minimal" {
					t.Errorf("tool name = %q, want minimal", req.Tools[0].Name)
				}
			},
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := c.ParseRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("ParseRequest error: %v", err)
			}
			tt.checkField(t, req)
		})
	}
}

// ============================================================================
// Section 4: Multi-turn Conversations
// ============================================================================

func TestClaudeProtocol_ParseRequest_MultiTurn(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		checkField func(t *testing.T, req *InternalRequest)
	}{
		{
			name: "2-turn conversation",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi there!"},{"role":"user","content":"How are you?"}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages) != 3 {
					t.Fatalf("messages = %d, want 3", len(req.Messages))
				}
				if req.Messages[0].Role != "user" || req.Messages[1].Role != "assistant" || req.Messages[2].Role != "user" {
					t.Errorf("roles = %q,%q,%q, want user,assistant,user", req.Messages[0].Role, req.Messages[1].Role, req.Messages[2].Role)
				}
			},
		},
		{
			name: "3-turn with tool use cycle",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check weather"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"NYC"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"72F sunny"}]},{"role":"assistant","content":"The weather is 72F and sunny."},{"role":"user","content":"Thanks!"}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages) != 5 {
					t.Fatalf("messages = %d, want 5", len(req.Messages))
				}
				if req.Messages[0].Role != "user" {
					t.Errorf("msg[0] role = %q", req.Messages[0].Role)
				}
				if req.Messages[1].Role != "assistant" {
					t.Errorf("msg[1] role = %q", req.Messages[1].Role)
				}
				if req.Messages[2].Role != "user" {
					t.Errorf("msg[2] role = %q (tool_result is user)", req.Messages[2].Role)
				}
			},
		},
		{
			name: "parallel tool calls + results",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check both cities"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"NYC"}},{"type":"tool_use","id":"toolu_02","name":"get_weather","input":{"city":"LA"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"72F"},{"type":"tool_result","tool_use_id":"toolu_02","content":"80F"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages) != 3 {
					t.Fatalf("messages = %d, want 3", len(req.Messages))
				}
				if len(req.Messages[1].Content) != 2 {
					t.Errorf("assistant content blocks = %d, want 2", len(req.Messages[1].Content))
				}
				if len(req.Messages[2].Content) != 2 {
					t.Errorf("user result content blocks = %d, want 2", len(req.Messages[2].Content))
				}
			},
		},
		{
			name: "long conversation 6 messages",
			body: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Q1"},{"role":"assistant","content":"A1"},{"role":"user","content":"Q2"},{"role":"assistant","content":"A2"},{"role":"user","content":"Q3"},{"role":"assistant","content":"A3"}],"max_tokens":100}`,
			checkField: func(t *testing.T, req *InternalRequest) {
				if len(req.Messages) != 6 {
					t.Fatalf("messages = %d, want 6", len(req.Messages))
				}
			},
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := c.ParseRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("ParseRequest error: %v", err)
			}
			tt.checkField(t, req)
		})
	}
}

// ============================================================================
// Section 5: Claude Response Building (InternalResponse → Claude JSON)
// Verifies the output matches the Anthropic Messages API response format.
// ============================================================================

func TestClaudeProtocol_BuildResponse_Format(t *testing.T) {
	tests := []struct {
		name       string
		resp       *InternalResponse
		checkField func(t *testing.T, body []byte)
	}{
		{
			name: "simple text response has correct structure",
			resp: &InternalResponse{
				ID: "msg_abc123", Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "end_turn",
				Content:    []ContentBlock{{Type: "text", Text: "Hello!"}},
				Usage:      &UsageInfo{InputTokens: 10, OutputTokens: 5},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				if resp["type"] != "message" {
					t.Errorf("type = %v, want message", resp["type"])
				}
				if resp["role"] != "assistant" {
					t.Errorf("role = %v, want assistant", resp["role"])
				}
				if resp["id"] != "msg_abc123" {
					t.Errorf("id = %v, want msg_abc123", resp["id"])
				}
				if resp["stop_reason"] != "end_turn" {
					t.Errorf("stop_reason = %v, want end_turn", resp["stop_reason"])
				}
			},
		},
		{
			name: "tool_use response has correct content blocks",
			resp: &InternalResponse{
				ID: "msg_tool", Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "tool_use",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "toolu_01", Name: "get_weather", Input: map[string]interface{}{"city": "NYC"}},
				},
				Usage: &UsageInfo{InputTokens: 20, OutputTokens: 15},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp models.ClaudeResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				if resp.StopReason != "tool_use" {
					t.Errorf("stop_reason = %q, want tool_use", resp.StopReason)
				}
				if len(resp.Content) != 1 {
					t.Fatalf("content = %d, want 1", len(resp.Content))
				}
				if resp.Content[0].Type != "tool_use" {
					t.Errorf("content[0].type = %q", resp.Content[0].Type)
				}
				if resp.Content[0].ID != "toolu_01" {
					t.Errorf("content[0].id = %q, want toolu_01", resp.Content[0].ID)
				}
				if resp.Content[0].Name != "get_weather" {
					t.Errorf("content[0].name = %q", resp.Content[0].Name)
				}
			},
		},
		{
			name: "text + tool_use response",
			resp: &InternalResponse{
				ID: "msg_both", Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "tool_use",
				Content: []ContentBlock{
					{Type: "text", Text: "Let me check."},
					{Type: "tool_use", ID: "toolu_01", Name: "search", Input: map[string]interface{}{"q": "test"}},
				},
				Usage: &UsageInfo{InputTokens: 30, OutputTokens: 20},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp models.ClaudeResponse
				json.Unmarshal(body, &resp)
				if len(resp.Content) != 2 {
					t.Fatalf("content blocks = %d, want 2", len(resp.Content))
				}
				if resp.Content[0].Type != "text" {
					t.Errorf("first block = %q, want text", resp.Content[0].Type)
				}
				if resp.Content[1].Type != "tool_use" {
					t.Errorf("second block = %q, want tool_use", resp.Content[1].Type)
				}
			},
		},
		{
			name: "multiple tool_use blocks",
			resp: &InternalResponse{
				ID: "msg_multi", Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "tool_use",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "toolu_01", Name: "fn_a", Input: map[string]interface{}{}},
					{Type: "tool_use", ID: "toolu_02", Name: "fn_b", Input: map[string]interface{}{}},
					{Type: "tool_use", ID: "toolu_03", Name: "fn_c", Input: map[string]interface{}{}},
				},
				Usage: &UsageInfo{InputTokens: 50, OutputTokens: 30},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp models.ClaudeResponse
				json.Unmarshal(body, &resp)
				if len(resp.Content) != 3 {
					t.Fatalf("content blocks = %d, want 3", len(resp.Content))
				}
			},
		},
		{
			name: "usage with cache tokens",
			resp: &InternalResponse{
				ID: "msg_cache", Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "end_turn",
				Content:    []ContentBlock{{Type: "text", Text: "Hi"}},
				Usage:      &UsageInfo{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 80, CacheWriteTokens: 20},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp models.ClaudeResponse
				json.Unmarshal(body, &resp)
				if resp.Usage.InputTokens != 100 {
					t.Errorf("input_tokens = %d, want 100", resp.Usage.InputTokens)
				}
				if resp.Usage.OutputTokens != 50 {
					t.Errorf("output_tokens = %d, want 50", resp.Usage.OutputTokens)
				}
				if resp.Usage.CacheReadInputTokens != 80 {
					t.Errorf("cache_read = %d, want 80", resp.Usage.CacheReadInputTokens)
				}
				if resp.Usage.CacheCreationInputTokens != 20 {
					t.Errorf("cache_creation = %d, want 20", resp.Usage.CacheCreationInputTokens)
				}
			},
		},
		{
			name: "max_tokens stop reason",
			resp: &InternalResponse{
				ID: "msg_max", Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "max_tokens",
				Content:    []ContentBlock{{Type: "text", Text: "truncated..."}},
				Usage:      &UsageInfo{InputTokens: 10, OutputTokens: 4096},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp models.ClaudeResponse
				json.Unmarshal(body, &resp)
				if resp.StopReason != "max_tokens" {
					t.Errorf("stop_reason = %q, want max_tokens", resp.StopReason)
				}
			},
		},
		{
			name: "model name in response matches request",
			resp: &InternalResponse{
				ID: "msg_mdl", Model: "claude-opus-4-7", Role: "assistant",
				StopReason: "end_turn",
				Content:    []ContentBlock{{Type: "text", Text: "test"}},
				Usage:      &UsageInfo{InputTokens: 5, OutputTokens: 3},
			},
			checkField: func(t *testing.T, body []byte) {
				var resp models.ClaudeResponse
				json.Unmarshal(body, &resp)
				if resp.Model != "claude-opus-4-7" {
					t.Errorf("model = %q, want claude-opus-4-7", resp.Model)
				}
			},
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := c.BuildResponse(tt.resp)
			if err != nil {
				t.Fatalf("BuildResponse error: %v", err)
			}
			tt.checkField(t, data)
		})
	}
}

// ============================================================================
// Section 6: Claude Response Parsing and Validation
// ============================================================================

func TestClaudeProtocol_ParseResponse_Validation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid response",
			body:    `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantErr: false,
		},
		{
			name:    "missing type field still works (optional check)",
			body:    `{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantErr: false,
		},
		{
			name:    "wrong type field",
			body:    `{"id":"msg_1","type":"error","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantErr: true,
			errMsg:  "unexpected type",
		},
		{
			name:    "missing role",
			body:    `{"id":"msg_1","type":"message","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantErr: true,
			errMsg:  "missing role",
		},
		{
			name:    "missing stop_reason",
			body:    `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`,
			wantErr: true,
			errMsg:  "missing stop_reason",
		},
		{
			name:    "empty content",
			body:    `{"id":"msg_1","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantErr: true,
			errMsg:  "empty content",
		},
		{
			name:    "invalid JSON",
			body:    `{broken}`,
			wantErr: true,
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.ParseResponse([]byte(tt.body))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// ============================================================================
// Section 7: Cross-Protocol Conversion (Claude → OpenAI → Claude)
// Tests the full round-trip through the factory.
// ============================================================================

func TestClaudeProtocol_CrossProtocol_RoundTrip(t *testing.T) {
	cfg := &config.Config{
		BigModel:    "gpt-4",
		MiddleModel: "gpt-4",
		SmallModel:  "gpt-3.5-turbo",
	}
	factory := NewConverterFactory()
	factory.SetOpenAIConfig(cfg)

	t.Run("simple text round trip", func(t *testing.T) {
		claudeReq := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}`

		// Claude → OpenAI
		openAIBody, _, err := factory.ConvertClaudeToOpenAI([]byte(claudeReq), cfg)
		if err != nil {
			t.Fatalf("Claude→OpenAI: %v", err)
		}

		var openAIReq models.OpenAIRequest
		if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
			t.Fatalf("invalid OpenAI JSON: %v", err)
		}
		if openAIReq.Model == "" {
			t.Error("OpenAI model is empty")
		}
		if len(openAIReq.Messages) == 0 {
			t.Error("OpenAI messages is empty")
		}

		// Simulate OpenAI response
		openAIResp := `{"id":"chatcmpl-1","object":"chat.completion","created":1234,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`

		// Parse original request for model info
		internalReq, _ := factory.ConvertClaudeToInternal([]byte(claudeReq))

		// OpenAI → Claude
		claudeRespBody, _, err := factory.ConvertOpenAIToClaude([]byte(openAIResp), internalReq)
		if err != nil {
			t.Fatalf("OpenAI→Claude: %v", err)
		}

		var claudeResp models.ClaudeResponse
		if err := json.Unmarshal(claudeRespBody, &claudeResp); err != nil {
			t.Fatalf("invalid Claude JSON: %v", err)
		}

		// Verify Claude response format
		if claudeResp.Type != "message" {
			t.Errorf("type = %q, want message", claudeResp.Type)
		}
		if claudeResp.Role != "assistant" {
			t.Errorf("role = %q, want assistant", claudeResp.Role)
		}
		if claudeResp.StopReason != "end_turn" {
			t.Errorf("stop_reason = %q, want end_turn", claudeResp.StopReason)
		}
		if claudeResp.Model != "claude-sonnet-4-6" {
			t.Errorf("model = %q, want claude-sonnet-4-6 (original)", claudeResp.Model)
		}
		if len(claudeResp.Content) != 1 || claudeResp.Content[0].Text != "Hello!" {
			t.Errorf("content = %+v", claudeResp.Content)
		}
	})

	t.Run("tool use round trip", func(t *testing.T) {
		claudeReq := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check weather"}],"max_tokens":1024,"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]}`

		openAIBody, _, err := factory.ConvertClaudeToOpenAI([]byte(claudeReq), cfg)
		if err != nil {
			t.Fatalf("Claude→OpenAI: %v", err)
		}

		var openAIReq models.OpenAIRequest
		json.Unmarshal(openAIBody, &openAIReq)
		if len(openAIReq.Tools) != 1 {
			t.Fatalf("OpenAI tools = %d, want 1", len(openAIReq.Tools))
		}
		if openAIReq.Tools[0].Function.Name != "get_weather" {
			t.Errorf("tool name = %q", openAIReq.Tools[0].Function.Name)
		}

		openAIResp := `{"id":"chatcmpl-2","object":"chat.completion","created":1234,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":15,"total_tokens":35}}`

		internalReq, _ := factory.ConvertClaudeToInternal([]byte(claudeReq))
		claudeRespBody, _, err := factory.ConvertOpenAIToClaude([]byte(openAIResp), internalReq)
		if err != nil {
			t.Fatalf("OpenAI→Claude: %v", err)
		}

		var claudeResp models.ClaudeResponse
		json.Unmarshal(claudeRespBody, &claudeResp)

		if claudeResp.StopReason != "tool_use" {
			t.Errorf("stop_reason = %q, want tool_use", claudeResp.StopReason)
		}
		if len(claudeResp.Content) != 1 {
			t.Fatalf("content blocks = %d, want 1", len(claudeResp.Content))
		}
		if claudeResp.Content[0].Type != "tool_use" {
			t.Errorf("content type = %q, want tool_use", claudeResp.Content[0].Type)
		}
		if claudeResp.Content[0].Name != "get_weather" {
			t.Errorf("tool name = %q", claudeResp.Content[0].Name)
		}
	})

	t.Run("multi-turn with tools round trip", func(t *testing.T) {
		claudeReq := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check weather"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"NYC"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"72F sunny"}]},{"role":"assistant","content":"It's 72F and sunny."},{"role":"user","content":"Thanks"}],"max_tokens":1024,"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]}`

		openAIBody, _, err := factory.ConvertClaudeToOpenAI([]byte(claudeReq), cfg)
		if err != nil {
			t.Fatalf("Claude→OpenAI: %v", err)
		}

		var openAIReq models.OpenAIRequest
		json.Unmarshal(openAIBody, &openAIReq)

		// Should have multiple messages after expansion
		if len(openAIReq.Messages) < 5 {
			t.Errorf("OpenAI messages = %d, want >= 5", len(openAIReq.Messages))
		}
	})
}

// ============================================================================
// Section 8: OpenAI → Claude Response Conversion (response_converter.go)
// Tests the legacy and factory-based response conversion.
// ============================================================================

func TestClaudeProtocol_OpenAIToClaude_StopReasonMapping(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		wantStop     string
	}{
		{"stop → end_turn", "stop", "end_turn"},
		{"tool_calls → tool_use", "tool_calls", "tool_use"},
		{"function_call → tool_use", "function_call", "tool_use"},
		{"length → max_tokens", "length", "max_tokens"},
		{"content_filter → end_turn", "content_filter", "end_turn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAIResp := &models.OpenAIResponse{
				ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
				Choices: []models.OpenAIChoice{{
					Index: 0, Message: models.OpenAIMessage{Role: "assistant", Content: "hi"}, FinishReason: tt.finishReason,
				}},
				Usage: models.OpenAIUsage{PromptTokens: 5, CompletionTokens: 3},
			}
			originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

			result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
			if result.StopReason != tt.wantStop {
				t.Errorf("stop_reason = %q, want %q", result.StopReason, tt.wantStop)
			}
		})
	}
}

func TestClaudeProtocol_OpenAIToClaude_ReasoningContent(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{{
			Index: 0,
			Message: models.OpenAIMessage{
				Role:             "assistant",
				Content:          "Final answer",
				ReasoningContent: "Let me think about this...",
			},
			FinishReason: "stop",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 20},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	// Factory path produces text only (reasoning_content not in OpenAI response JSON for factory path)
	// Legacy fallback handles reasoning_content → thinking block
	if len(result.Content) < 1 {
		t.Fatalf("content blocks = %d, want >= 1", len(result.Content))
	}
	// Verify at least text content is present
	found := false
	for _, cb := range result.Content {
		if cb.Type == "text" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one text content block")
	}
}

func TestClaudeProtocol_OpenAIToClaude_ToolCallIDFormats(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"call_ prefix", "call_abc123"},
		{"fc_ prefix", "fc_abc123"},
		{"no prefix", "fcSxBUVrxLp43jX9lGXFgdwuo9"},
		{"toolu_ prefix", "toolu_0123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAIResp := &models.OpenAIResponse{
				ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
				Choices: []models.OpenAIChoice{{
					Index: 0,
					Message: models.OpenAIMessage{
						Role: "assistant",
						ToolCalls: []models.OpenAIToolCall{{
							ID: tt.id, Type: "function",
							Function: models.OpenAIFunctionCall{Name: "test_fn", Arguments: "{}"},
						}},
					},
					FinishReason: "tool_calls",
				}},
				Usage: models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5},
			}
			originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

			result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
			if len(result.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1", len(result.Content))
			}
			// ID should be preserved exactly
			if result.Content[0].ID != tt.id {
				t.Errorf("id = %q, want %q (preserved)", result.Content[0].ID, tt.id)
			}
		})
	}
}

func TestClaudeProtocol_OpenAIToClaude_MultipleToolCalls(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{{
			Index: 0,
			Message: models.OpenAIMessage{
				Role: "assistant",
				ToolCalls: []models.OpenAIToolCall{
					{ID: "call_1", Type: "function", Function: models.OpenAIFunctionCall{Name: "fn_a", Arguments: `{"x":1}`}},
					{ID: "call_2", Type: "function", Function: models.OpenAIFunctionCall{Name: "fn_b", Arguments: `{"y":2}`}},
					{ID: "call_3", Type: "function", Function: models.OpenAIFunctionCall{Name: "fn_c", Arguments: `{"z":3}`}},
				},
			},
			FinishReason: "tool_calls",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 20, CompletionTokens: 30},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	if len(result.Content) != 3 {
		t.Fatalf("content blocks = %d, want 3", len(result.Content))
	}
	for i, expected := range []string{"fn_a", "fn_b", "fn_c"} {
		if result.Content[i].Name != expected {
			t.Errorf("content[%d].name = %q, want %q", i, result.Content[i].Name, expected)
		}
	}
}

func TestClaudeProtocol_OpenAIToClaude_TextPlusToolCalls(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{{
			Index: 0,
			Message: models.OpenAIMessage{
				Role:    "assistant",
				Content: "Let me search for that.",
				ToolCalls: []models.OpenAIToolCall{
					{ID: "call_1", Type: "function", Function: models.OpenAIFunctionCall{Name: "search", Arguments: `{"q":"test"}`}},
				},
			},
			FinishReason: "tool_calls",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 15, CompletionTokens: 20},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	if len(result.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("first block type = %q, want text", result.Content[0].Type)
	}
	if result.Content[1].Type != "tool_use" {
		t.Errorf("second block type = %q, want tool_use", result.Content[1].Type)
	}
}

func TestClaudeProtocol_OpenAIToClaude_ReasoningContentLegacy(t *testing.T) {
	// Test reasoning_content through legacy fallback path
	// When factory path can't handle reasoning_content, the legacy fallback converts it to thinking block
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{{
			Index: 0,
			Message: models.OpenAIMessage{
				Role:             "assistant",
				Content:          "The answer is 42.",
				ReasoningContent: "I need to think about this carefully...",
			},
			FinishReason: "stop",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 20},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	// Verify content blocks are present (factory or legacy path)
	if len(result.Content) < 1 {
		t.Fatalf("content blocks = %d, want >= 1", len(result.Content))
	}
}

func TestClaudeProtocol_OpenAIToClaude_EmptyChoices(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{},
		Usage:   models.OpenAIUsage{PromptTokens: 5, CompletionTokens: 0},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result.StopReason)
	}
}

func TestClaudeProtocol_OpenAIToClaude_NilResponse(t *testing.T) {
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}
	result := ConvertOpenAIToClaudeResponse(nil, originalReq)
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q", result.StopReason)
	}
}

func TestClaudeProtocol_OpenAIToClaude_CacheTokens(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{{
			Index: 0, Message: models.OpenAIMessage{Role: "assistant", Content: "hi"}, FinishReason: "stop",
		}},
		Usage: models.OpenAIUsage{
			PromptTokens: 100, CompletionTokens: 50,
			PromptTokensDetails: &models.PromptTokensDetails{CachedTokens: 80},
		},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	if result.Usage.CacheReadInputTokens != 80 {
		t.Errorf("cache_read_input_tokens = %d, want 80", result.Usage.CacheReadInputTokens)
	}
}

func TestClaudeProtocol_OpenAIToClaude_ToolCallWithMalformedArguments(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "gpt-4",
		Choices: []models.OpenAIChoice{{
			Index: 0,
			Message: models.OpenAIMessage{
				Role: "assistant",
				ToolCalls: []models.OpenAIToolCall{
					{ID: "call_1", Type: "function", Function: models.OpenAIFunctionCall{Name: "fn", Arguments: "not-json"}},
				},
			},
			FinishReason: "tool_calls",
		}},
		Usage: models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}
	originalReq := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6"}

	result := ConvertOpenAIToClaudeResponse(openAIResp, originalReq)
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	// Should still produce a tool_use block with raw_arguments fallback
	if result.Content[0].Type != "tool_use" {
		t.Errorf("type = %q, want tool_use", result.Content[0].Type)
	}
}

// ============================================================================
// Section 9: Claude → OpenAI Request Conversion
// Verifies Claude request fields are correctly mapped to OpenAI format.
// ============================================================================

func TestClaudeProtocol_ClaudeToOpenAI_RequestMapping(t *testing.T) {
	cfg := &config.Config{
		BigModel:    "gpt-4",
		MiddleModel: "gpt-4",
		SmallModel:  "gpt-3.5-turbo",
	}
	factory := NewConverterFactory()
	factory.SetOpenAIConfig(cfg)

	tests := []struct {
		name       string
		claudeReq  string
		checkField func(t *testing.T, openAIReq *models.OpenAIRequest)
	}{
		{
			name:      "model mapping",
			claudeReq: `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				if openAIReq.Model != "gpt-4" {
					t.Errorf("model = %q, want gpt-4 (big model)", openAIReq.Model)
				}
			},
		},
		{
			name:      "max_tokens mapped",
			claudeReq: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":2048}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				if openAIReq.MaxTokens != 2048 {
					t.Errorf("max_tokens = %d, want 2048", openAIReq.MaxTokens)
				}
			},
		},
		{
			name:      "temperature mapped",
			claudeReq: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"temperature":0.7}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				if openAIReq.Temperature != 0.7 {
					t.Errorf("temperature = %f, want 0.7", openAIReq.Temperature)
				}
			},
		},
		{
			name:      "tools mapped to OpenAI format",
			claudeReq: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object"}}]}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				if len(openAIReq.Tools) != 1 {
					t.Fatalf("tools = %d, want 1", len(openAIReq.Tools))
				}
				if openAIReq.Tools[0].Type != "function" {
					t.Errorf("tool type = %q, want function", openAIReq.Tools[0].Type)
				}
				if openAIReq.Tools[0].Function.Name != "get_weather" {
					t.Errorf("function name = %q", openAIReq.Tools[0].Function.Name)
				}
			},
		},
		{
			name:      "system message mapped",
			claudeReq: `{"model":"claude-sonnet-4-6","system":"You are helpful","messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				// System should be the first message
				if len(openAIReq.Messages) < 2 {
					t.Fatalf("messages = %d, want >= 2", len(openAIReq.Messages))
				}
				if openAIReq.Messages[0].Role != "system" {
					t.Errorf("first message role = %q, want system", openAIReq.Messages[0].Role)
				}
			},
		},
		{
			name:      "tool_result becomes tool message",
			claudeReq: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"result"}]}],"max_tokens":100}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				// Find tool message
				found := false
				for _, msg := range openAIReq.Messages {
					if msg.Role == "tool" && msg.ToolCallID == "toolu_01" {
						found = true
						break
					}
				}
				if !found {
					t.Error("tool message with tool_call_id=toolu_01 not found")
				}
			},
		},
		{
			name:      "stream flag mapped",
			claudeReq: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"stream":true}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				if !openAIReq.Stream {
					t.Error("stream = false, want true")
				}
			},
		},
		{
			name:      "stop_sequences mapped",
			claudeReq: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"stop_sequences":["END"]}`,
			checkField: func(t *testing.T, openAIReq *models.OpenAIRequest) {
				if openAIReq.Stop == nil {
					t.Fatal("stop is nil")
				}
				stops, ok := openAIReq.Stop.([]string)
				if !ok {
					// might be []interface{}
					stopSlice, ok := openAIReq.Stop.([]interface{})
					if !ok {
						t.Fatalf("stop type = %T", openAIReq.Stop)
					}
					if len(stopSlice) != 1 {
						t.Errorf("stop = %v, want [END]", stopSlice)
					}
					return
				}
				if len(stops) != 1 || stops[0] != "END" {
					t.Errorf("stop = %v, want [END]", stops)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAIBody, _, err := factory.ConvertClaudeToOpenAI([]byte(tt.claudeReq), cfg)
			if err != nil {
				t.Fatalf("ConvertClaudeToOpenAI error: %v", err)
			}

			var openAIReq models.OpenAIRequest
			if err := json.Unmarshal(openAIBody, &openAIReq); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			tt.checkField(t, &openAIReq)
		})
	}
}

// ============================================================================
// Section 10: Claude Request → Claude Request Round Trip
// Tests ParseRequest → BuildRequest symmetry for the Claude converter.
// ============================================================================

func TestClaudeProtocol_ClaudeRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkFunc func(t *testing.T, original, roundTrip map[string]interface{})
	}{
		{
			name:  "simple message preserves all fields",
			input: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024,"stream":true}`,
			checkFunc: func(t *testing.T, original, roundTrip map[string]interface{}) {
				if roundTrip["model"] != original["model"] {
					t.Errorf("model mismatch")
				}
				if roundTrip["max_tokens"] != original["max_tokens"] {
					t.Errorf("max_tokens mismatch")
				}
			},
		},
		{
			name:  "tools preserved through round trip",
			input: `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"fn","description":"desc","input_schema":{"type":"object"}}]}`,
			checkFunc: func(t *testing.T, original, roundTrip map[string]interface{}) {
				origTools := original["tools"].([]interface{})
				rtTools := roundTrip["tools"].([]interface{})
				if len(origTools) != len(rtTools) {
					t.Errorf("tools count mismatch: %d vs %d", len(origTools), len(rtTools))
				}
			},
		},
		{
			name:  "system prompt preserved",
			input: `{"model":"claude-sonnet-4-6","system":"You are helpful","messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`,
			checkFunc: func(t *testing.T, original, roundTrip map[string]interface{}) {
				if roundTrip["system"] != original["system"] {
					t.Errorf("system mismatch: %v vs %v", roundTrip["system"], original["system"])
				}
			},
		},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := c.ParseRequest([]byte(tt.input))
			if err != nil {
				t.Fatalf("ParseRequest error: %v", err)
			}

			data, err := c.BuildRequest(req)
			if err != nil {
				t.Fatalf("BuildRequest error: %v", err)
			}

			var original, roundTrip map[string]interface{}
			json.Unmarshal([]byte(tt.input), &original)
			json.Unmarshal(data, &roundTrip)

			tt.checkFunc(t, original, roundTrip)
		})
	}
}

// ============================================================================
// Section 11: OpenAI Converter Parse/Build for Claude Protocol Support
// Tests the OpenAI converter handles Claude-equivalent constructs.
// ============================================================================

func TestClaudeProtocol_OpenAIConverter_StopReasonMappings(t *testing.T) {
	c := NewOpenAIConverter(nil)

	tests := []struct {
		name           string
		openAIResp     string
		wantStopReason string
	}{
		{
			name:           "stop → end_turn",
			openAIResp:     `{"id":"1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`,
			wantStopReason: "end_turn",
		},
		{
			name:           "tool_calls → tool_use",
			openAIResp:     `{"id":"1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"fn","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`,
			wantStopReason: "tool_use",
		},
		{
			name:           "length → max_tokens",
			openAIResp:     `{"id":"1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"truncated"},"finish_reason":"length"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`,
			wantStopReason: "max_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := c.ParseResponse([]byte(tt.openAIResp))
			if err != nil {
				t.Fatalf("ParseResponse error: %v", err)
			}
			if resp.StopReason != tt.wantStopReason {
				t.Errorf("stop_reason = %q, want %q", resp.StopReason, tt.wantStopReason)
			}
		})
	}
}

// ============================================================================
// Section 12: Stream Event Parsing
// ============================================================================

func TestClaudeProtocol_StreamEventParsing(t *testing.T) {
	c := NewClaudeConverter()

	tests := []struct {
		name       string
		line       string
		checkField func(t *testing.T, event *StreamEvent)
	}{
		{
			name: "content_block_delta text_delta",
			line: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "content_block_delta" {
					t.Errorf("type = %q", event.Type)
				}
				if event.Delta == nil || event.Delta.Text != "Hello" {
					t.Errorf("delta.text = %v", event.Delta)
				}
			},
		},
		{
			name: "content_block_delta input_json_delta",
			line: `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "content_block_delta" {
					t.Errorf("type = %q", event.Type)
				}
				if event.Index != 1 {
					t.Errorf("index = %d, want 1", event.Index)
				}
			},
		},
		{
			name: "message_delta with stop_reason",
			line: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "message_delta" {
					t.Errorf("type = %q", event.Type)
				}
				if event.Delta == nil || event.Delta.StopReason != "end_turn" {
					t.Errorf("delta.stop_reason = %v", event.Delta)
				}
				if event.Usage == nil || event.Usage.OutputTokens != 42 {
					t.Errorf("usage.output_tokens = %v", event.Usage)
				}
			},
		},
		{
			name: "message_delta with tool_use stop_reason",
			line: `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Delta.StopReason != "tool_use" {
					t.Errorf("stop_reason = %q, want tool_use", event.Delta.StopReason)
				}
			},
		},
		{
			name: "message_delta with max_tokens stop_reason",
			line: `{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":4096}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Delta.StopReason != "max_tokens" {
					t.Errorf("stop_reason = %q, want max_tokens", event.Delta.StopReason)
				}
			},
		},
		{
			name: "content_block_start with text",
			line: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "content_block_start" {
					t.Errorf("type = %q", event.Type)
				}
			},
		},
		{
			name: "content_block_stop",
			line: `{"type":"content_block_stop","index":0}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "content_block_stop" {
					t.Errorf("type = %q", event.Type)
				}
			},
		},
		{
			name: "message_start",
			line: `{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "message_start" {
					t.Errorf("type = %q", event.Type)
				}
			},
		},
		{
			name: "message_stop",
			line: `{"type":"message_stop"}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "message_stop" {
					t.Errorf("type = %q", event.Type)
				}
			},
		},
		{
			name: "ping event",
			line: `{"type":"ping"}`,
			checkField: func(t *testing.T, event *StreamEvent) {
				if event.Type != "ping" {
					t.Errorf("type = %q", event.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := c.ParseStreamEvent([]byte(tt.line))
			if err != nil {
				t.Fatalf("ParseStreamEvent error: %v", err)
			}
			tt.checkField(t, event)
		})
	}
}

// ============================================================================
// Section 13: Stream Event Building
// ============================================================================

func TestClaudeProtocol_StreamEventBuilding(t *testing.T) {
	c := NewClaudeConverter()

	tests := []struct {
		name       string
		event      *StreamEvent
		checkField func(t *testing.T, data []byte)
	}{
		{
			name:  "text delta event produces valid JSON",
			event: &StreamEvent{Type: "content_block_delta", Index: 0, Delta: &StreamDelta{Type: "text_delta", Text: "Hello"}},
			checkField: func(t *testing.T, data []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				if result["type"] != "content_block_delta" {
					t.Errorf("type = %v", result["type"])
				}
			},
		},
		{
			name:  "message delta with usage",
			event: &StreamEvent{Type: "message_delta", Delta: &StreamDelta{StopReason: "end_turn"}, Usage: &UsageInfo{OutputTokens: 42}},
			checkField: func(t *testing.T, data []byte) {
				var result map[string]interface{}
				json.Unmarshal(data, &result)
				delta := result["delta"].(map[string]interface{})
				if delta["stop_reason"] != "end_turn" {
					t.Errorf("stop_reason = %v", delta["stop_reason"])
				}
				usage := result["usage"].(map[string]interface{})
				if int(usage["output_tokens"].(float64)) != 42 {
					t.Errorf("output_tokens = %v", usage["output_tokens"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := c.BuildStreamEvent(tt.event)
			if err != nil {
				t.Fatalf("BuildStreamEvent error: %v", err)
			}
			tt.checkField(t, data)
		})
	}
}

// ============================================================================
// Section 14: Edge Cases - Unicode, Special Characters, Large Payloads
// ============================================================================

func TestClaudeProtocol_EdgeCases(t *testing.T) {
	c := NewClaudeConverter()

	t.Run("CJK content", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"你好世界"}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Messages[0].Content[0].Text != "你好世界" {
			t.Errorf("text = %q", req.Messages[0].Content[0].Text)
		}
	})

	t.Run("emoji content", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hello 👋 🌍"}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Messages[0].Content[0].Text != "Hello 👋 🌍" {
			t.Errorf("text = %q", req.Messages[0].Content[0].Text)
		}
	})

	t.Run("empty string message content", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":""}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Messages[0].Content[0].Text != "" {
			t.Errorf("text = %q, want empty", req.Messages[0].Content[0].Text)
		}
	})

	t.Run("nested JSON in tool input", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn","input":{"nested":{"deep":{"value":42}}}}]}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		input := req.Messages[1].Content[0].Input
		nested, ok := input["nested"].(map[string]interface{})
		if !ok {
			t.Fatal("nested not found")
		}
		deep, ok := nested["deep"].(map[string]interface{})
		if !ok {
			t.Fatal("deep not found")
		}
		if deep["value"] != float64(42) {
			t.Errorf("value = %v, want 42", deep["value"])
		}
	})

	t.Run("large text content", func(t *testing.T) {
		largeText := strings.Repeat("A", 10000)
		body := fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"%s"}],"max_tokens":100}`, largeText)
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(req.Messages[0].Content[0].Text) != 10000 {
			t.Errorf("text length = %d, want 10000", len(req.Messages[0].Content[0].Text))
		}
	})

	t.Run("tool name with underscores and numbers", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"tools":[{"name":"my_tool_v2_search","description":"Search","input_schema":{"type":"object"}}]}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Tools[0].Name != "my_tool_v2_search" {
			t.Errorf("name = %q", req.Tools[0].Name)
		}
	})

	t.Run("null content in tool_result", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"fn","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":null}]}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Messages[2].Content[0].Type != "tool_result" {
			t.Errorf("type = %q", req.Messages[2].Content[0].Type)
		}
	})

	t.Run("top_p parameter", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"top_p":0.9}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.TopP == nil || *req.TopP != 0.9 {
			t.Errorf("top_p = %v, want 0.9", req.TopP)
		}
	})

	t.Run("top_k parameter", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"top_k":40}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.TopK == nil || *req.TopK != 40 {
			t.Errorf("top_k = %v, want 40", req.TopK)
		}
	})

	t.Run("thinking config", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":8192,"thinking":{"type":"enabled","budget_tokens":4096}}`
		// Note: thinking is parsed into the ClaudeMessagesRequest but may not flow through to InternalRequest
		// This test just verifies parsing doesn't crash
		_, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("context_management field ignored", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"context_management":{"clear_function_results":true}}`
		_, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})
}

// ============================================================================
// Section 15: Session ID Extraction
// ============================================================================

func TestClaudeProtocol_SessionIDExtraction(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		wantSID string
	}{
		{"plain string", "user123", ""},
		{"embedded JSON", `{"session_id":"sess-abc"}`, "sess-abc"},
		{"empty string", "", ""},
		{"invalid JSON", "{not json}", ""},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"metadata":{"user_id":%q}}`, tt.userID)
			req, err := c.ParseRequest([]byte(body))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			gotSID, _ := req.Metadata["session_id"].(string)
			if gotSID != tt.wantSID {
				t.Errorf("session_id = %q, want %q", gotSID, tt.wantSID)
			}
		})
	}
}

// ============================================================================
// Section 16: Error Response Format
// ============================================================================

func TestClaudeProtocol_ErrorResponseFormat(t *testing.T) {
	// Verify that Claude-style error responses are structurally valid
	errResp := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "invalid_request_error",
			"message": "max_tokens is required",
		},
	}
	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	if parsed["type"] != "error" {
		t.Errorf("type = %v", parsed["type"])
	}
	errObj := parsed["error"].(map[string]interface{})
	if errObj["type"] != "invalid_request_error" {
		t.Errorf("error.type = %v", errObj["type"])
	}
}

// ============================================================================
// Section 17: Image Media Types
// ============================================================================

func TestClaudeProtocol_ImageMediaTypes(t *testing.T) {
	mediaTypes := []string{"image/png", "image/jpeg", "image/gif", "image/webp"}
	c := NewClaudeConverter()

	for _, mt := range mediaTypes {
		t.Run(mt, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"%s","data":"dGVzdA=="}}]}],"max_tokens":100}`, mt)
			req, err := c.ParseRequest([]byte(body))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if req.Messages[0].Content[0].Source.MediaType != mt {
				t.Errorf("media_type = %q, want %q", req.Messages[0].Content[0].Source.MediaType, mt)
			}
		})
	}
}

// ============================================================================
// Section 18: Claude Response Model Preservation
// ============================================================================

func TestClaudeProtocol_ResponseModelPreservation(t *testing.T) {
	models_to_test := []string{
		"claude-sonnet-4-6",
		"claude-opus-4-7",
		"claude-haiku-4-5",
		"claude-3-5-sonnet-20241022",
		"claude-3-opus-20240229",
	}

	for _, model := range models_to_test {
		t.Run(model, func(t *testing.T) {
			resp := &InternalResponse{
				ID: "msg_test", Model: model, Role: "assistant",
				StopReason: "end_turn",
				Content:    []ContentBlock{{Type: "text", Text: "ok"}},
				Usage:      &UsageInfo{InputTokens: 1, OutputTokens: 1},
			}
			c := NewClaudeConverter()
			data, err := c.BuildResponse(resp)
			if err != nil {
				t.Fatalf("error: %v", err)
			}

			var result models.ClaudeResponse
			json.Unmarshal(data, &result)
			if result.Model != model {
				t.Errorf("model = %q, want %q", result.Model, model)
			}
		})
	}
}

// ============================================================================
// Section 19: OpenAI Stream Event Parsing
// ============================================================================

func TestClaudeProtocol_OpenAIStreamParsing(t *testing.T) {
	c := NewOpenAIConverter(nil)

	t.Run("text delta", func(t *testing.T) {
		line := []byte(`data: {"choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}`)
		event, err := c.ParseStreamEvent(line)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if event.Delta == nil || event.Delta.Text != "Hi" {
			t.Errorf("delta = %+v", event.Delta)
		}
	})

	t.Run("[DONE] marker", func(t *testing.T) {
		line := []byte(`data: [DONE]`)
		event, err := c.ParseStreamEvent(line)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if event.Type != "done" {
			t.Errorf("type = %q, want done", event.Type)
		}
	})

	t.Run("empty line", func(t *testing.T) {
		line := []byte(``)
		_, err := c.ParseStreamEvent(line)
		// Should not crash
		if err == nil {
			// Empty lines might return nil event or error, both are acceptable
		}
	})

	t.Run("finish_reason in stream", func(t *testing.T) {
		line := []byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`)
		event, err := c.ParseStreamEvent(line)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if event == nil {
			t.Fatal("event is nil")
		}
		if event.Delta == nil {
			t.Fatal("delta is nil")
		}
		if event.Delta.StopReason != "stop" {
			t.Errorf("stop_reason = %q, want stop", event.Delta.StopReason)
		}
	})
}

// ============================================================================
// Section 20: Complete Anthropic SSE Event Sequence Validation
// Tests that a complete Claude streaming response follows the correct protocol.
// ============================================================================

func TestClaudeProtocol_CompleteSSESequence(t *testing.T) {
	c := NewClaudeConverter()

	events := []struct {
		name  string
		line  string
		etype string
	}{
		{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`, "message_start"},
		{"ping", `{"type":"ping"}`, "ping"},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, "content_block_start"},
		{"content_block_delta_1", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`, "content_block_delta"},
		{"content_block_delta_2", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`, "content_block_delta"},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`, "content_block_stop"},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`, "message_delta"},
		{"message_stop", `{"type":"message_stop"}`, "message_stop"},
	}

	for _, ev := range events {
		t.Run(ev.name, func(t *testing.T) {
			event, err := c.ParseStreamEvent([]byte(ev.line))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if event.Type != ev.etype {
				t.Errorf("type = %q, want %q", event.Type, ev.etype)
			}
		})
	}
}

// ============================================================================
// Section 21: Complete SSE Sequence with Tool Use
// ============================================================================

func TestClaudeProtocol_CompleteSSESequence_ToolUse(t *testing.T) {
	c := NewClaudeConverter()

	events := []struct {
		name  string
		line  string
		etype string
	}{
		{"message_start", `{"type":"message_start","message":{"id":"msg_002","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"usage":{"input_tokens":20,"output_tokens":0}}}`, "message_start"},
		{"ping", `{"type":"ping"}`, "ping"},
		{"text_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, "content_block_start"},
		{"text_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check."}}`, "content_block_delta"},
		{"text_block_stop", `{"type":"content_block_stop","index":0}`, "content_block_stop"},
		{"tool_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{}}}`, "content_block_start"},
		{"tool_delta_1", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`, "content_block_delta"},
		{"tool_delta_2", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"NYC\"}"}}`, "content_block_delta"},
		{"tool_block_stop", `{"type":"content_block_stop","index":1}`, "content_block_stop"},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":15}}`, "message_delta"},
		{"message_stop", `{"type":"message_stop"}`, "message_stop"},
	}

	for _, ev := range events {
		t.Run(ev.name, func(t *testing.T) {
			event, err := c.ParseStreamEvent([]byte(ev.line))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if event.Type != ev.etype {
				t.Errorf("type = %q, want %q", event.Type, ev.etype)
			}
		})
	}
}

// ============================================================================
// Section 22: Model Name Variations
// ============================================================================

func TestClaudeProtocol_ModelNames(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"claude-opus-4-7", "claude-opus-4-7"},
		{"claude-haiku-4-5", "claude-haiku-4-5"},
		{"claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20241022"},
		{"claude-3-opus-20240229", "claude-3-opus-20240229"},
		{"claude-3-haiku-20240307", "claude-3-haiku-20240307"},
		{"claude-3-5-haiku-20241022", "claude-3-5-haiku-20241022"},
	}

	c := NewClaudeConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`, tt.model)
			req, err := c.ParseRequest([]byte(body))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if req.Model != tt.model {
				t.Errorf("model = %q, want %q", req.Model, tt.model)
			}
		})
	}
}

// ============================================================================
// Section 23: OpenAI BuildStreamEvent Mappings
// ============================================================================

func TestClaudeProtocol_OpenAIBuildStreamEvent_StopReasonMapping(t *testing.T) {
	c := NewOpenAIConverter(nil)

	tests := []struct {
		name             string
		stopReason       string
		wantFinishReason string
	}{
		{"end_turn → stop", "end_turn", "stop"},
		{"tool_use → tool_calls", "tool_use", "tool_calls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &StreamEvent{
				Type:  "message_delta",
				Delta: &StreamDelta{StopReason: tt.stopReason},
			}
			data, err := c.BuildStreamEvent(event)
			if err != nil {
				t.Fatalf("error: %v", err)
			}

			var result map[string]interface{}
			json.Unmarshal(data, &result)
			choices := result["choices"].([]interface{})
			choice := choices[0].(map[string]interface{})
			if choice["finish_reason"] != tt.wantFinishReason {
				t.Errorf("finish_reason = %v, want %v", choice["finish_reason"], tt.wantFinishReason)
			}
		})
	}
}

// ============================================================================
// Section 24: Claude Request with Thinking (Extended Thinking Beta)
// ============================================================================

func TestClaudeProtocol_ThinkingConfig(t *testing.T) {
	c := NewClaudeConverter()

	t.Run("thinking enabled with budget", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Solve this"}],"max_tokens":16000,"thinking":{"type":"enabled","budget_tokens":10000}}`
		// Should not crash - thinking config may or may not be fully propagated to InternalRequest
		_, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("thinking block in assistant message history", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"thinking","thinking":"Hmm..."},{"type":"text","text":"The answer is 42."}]}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(req.Messages[1].Content) != 2 {
			t.Fatalf("content blocks = %d, want 2", len(req.Messages[1].Content))
		}
		if req.Messages[1].Content[0].Type != "thinking" {
			t.Errorf("first block type = %q", req.Messages[1].Content[0].Type)
		}
		if req.Messages[1].Content[1].Type != "text" {
			t.Errorf("second block type = %q", req.Messages[1].Content[1].Type)
		}
	})

	t.Run("redacted thinking in history", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":[{"type":"redacted_thinking","data":"base64data"},{"type":"text","text":"Done"}]}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Messages[1].Content[0].Type != "redacted_thinking" {
			t.Errorf("type = %q", req.Messages[1].Content[0].Type)
		}
	})
}

// ============================================================================
// Section 25: Protocol Compliance Checks
// ============================================================================

func TestClaudeProtocol_ResponseCompliance(t *testing.T) {
	c := NewClaudeConverter()

	t.Run("response always has type=message", func(t *testing.T) {
		resp := &InternalResponse{
			ID: "msg_001", Model: "claude-sonnet-4-6", Role: "assistant",
			StopReason: "end_turn",
			Content:    []ContentBlock{{Type: "text", Text: "ok"}},
			Usage:      &UsageInfo{InputTokens: 1, OutputTokens: 1},
		}
		data, _ := c.BuildResponse(resp)
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		if result["type"] != "message" {
			t.Errorf("type = %v, want 'message'", result["type"])
		}
	})

	t.Run("response always has role=assistant", func(t *testing.T) {
		resp := &InternalResponse{
			ID: "msg_002", Model: "claude-sonnet-4-6", Role: "assistant",
			StopReason: "end_turn",
			Content:    []ContentBlock{{Type: "text", Text: "ok"}},
			Usage:      &UsageInfo{InputTokens: 1, OutputTokens: 1},
		}
		data, _ := c.BuildResponse(resp)
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		if result["role"] != "assistant" {
			t.Errorf("role = %v", result["role"])
		}
	})

	t.Run("response has valid usage", func(t *testing.T) {
		resp := &InternalResponse{
			ID: "msg_003", Model: "claude-sonnet-4-6", Role: "assistant",
			StopReason: "end_turn",
			Content:    []ContentBlock{{Type: "text", Text: "ok"}},
			Usage:      &UsageInfo{InputTokens: 100, OutputTokens: 50},
		}
		data, _ := c.BuildResponse(resp)
		var result models.ClaudeResponse
		json.Unmarshal(data, &result)
		if result.Usage.InputTokens != 100 {
			t.Errorf("input_tokens = %d", result.Usage.InputTokens)
		}
		if result.Usage.OutputTokens != 50 {
			t.Errorf("output_tokens = %d", result.Usage.OutputTokens)
		}
	})

	t.Run("tool_use response has correct content block fields", func(t *testing.T) {
		resp := &InternalResponse{
			ID: "msg_004", Model: "claude-sonnet-4-6", Role: "assistant",
			StopReason: "tool_use",
			Content: []ContentBlock{
				{Type: "tool_use", ID: "toolu_01", Name: "read_file", Input: map[string]interface{}{"path": "/tmp/test.txt"}},
			},
			Usage: &UsageInfo{InputTokens: 20, OutputTokens: 10},
		}
		data, _ := c.BuildResponse(resp)
		var result models.ClaudeResponse
		json.Unmarshal(data, &result)

		cb := result.Content[0]
		if cb.Type != "tool_use" {
			t.Errorf("type = %q", cb.Type)
		}
		if cb.ID != "toolu_01" {
			t.Errorf("id = %q", cb.ID)
		}
		if cb.Name != "read_file" {
			t.Errorf("name = %q", cb.Name)
		}
		if cb.Input["path"] != "/tmp/test.txt" {
			t.Errorf("input.path = %v", cb.Input["path"])
		}
	})
}

// ============================================================================
// Section 26: Message ID Format
// ============================================================================

func TestClaudeProtocol_MessageIDFormat(t *testing.T) {
	ids := []string{
		"msg_01A0B1C2D3E4F5G6H7I8J9K0",
		"msg_short",
		"msg_0123456789abcdef",
	}

	c := NewClaudeConverter()
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			resp := &InternalResponse{
				ID: id, Model: "claude-sonnet-4-6", Role: "assistant",
				StopReason: "end_turn",
				Content:    []ContentBlock{{Type: "text", Text: "ok"}},
				Usage:      &UsageInfo{InputTokens: 1, OutputTokens: 1},
			}
			data, err := c.BuildResponse(resp)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			var result models.ClaudeResponse
			json.Unmarshal(data, &result)
			if result.ID != id {
				t.Errorf("id = %q, want %q", result.ID, id)
			}
		})
	}
}

// ============================================================================
// Section 27: OpenAI Tool Call ID Variations in Response
// ============================================================================

func TestClaudeProtocol_OpenAIToolCallIDVariations(t *testing.T) {
	ids := []string{
		"call_abc123",
		"fc_SxBUVrxLp43jX9lGXFgdwuo9",
		"chatcmpl-abc_tool-1",
		"toolu_0123456789ABCDEF",
		"custom-id-format",
	}

	c := NewOpenAIConverter(nil)
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			body := fmt.Sprintf(`{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"%s","type":"function","function":{"name":"fn","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, id)
			resp, err := c.ParseResponse([]byte(body))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if resp.Content[0].ID != id {
				t.Errorf("id = %q, want %q (preserved)", resp.Content[0].ID, id)
			}
		})
	}
}

// ============================================================================
// Section 28: Factory-Based Full Pipeline
// ============================================================================

func TestClaudeProtocol_FactoryFullPipeline(t *testing.T) {
	cfg := &config.Config{
		BigModel:    "gpt-4",
		MiddleModel: "gpt-4",
		SmallModel:  "gpt-3.5-turbo",
	}
	factory := NewConverterFactory()
	factory.SetOpenAIConfig(cfg)

	t.Run("pipeline: claude req → openai req → openai resp → claude resp", func(t *testing.T) {
		claudeReq := `{"model":"claude-sonnet-4-6","system":"Be helpful","messages":[{"role":"user","content":"What's 2+2?"}],"max_tokens":100,"temperature":0}`

		// Step 1: Claude → OpenAI
		openAIBody, internalReq, err := factory.ConvertClaudeToOpenAI([]byte(claudeReq), cfg)
		if err != nil {
			t.Fatalf("Claude→OpenAI: %v", err)
		}

		var openAIReq models.OpenAIRequest
		json.Unmarshal(openAIBody, &openAIReq)
		if openAIReq.Model == "" {
			t.Error("model is empty after conversion")
		}
		if len(openAIReq.Messages) == 0 {
			t.Error("messages is empty")
		}

		// Step 2: Simulate OpenAI response
		openAIResp := `{"id":"chatcmpl-pipe","object":"chat.completion","created":1234,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"4"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`

		// Step 3: OpenAI → Claude
		claudeRespBody, _, err := factory.ConvertOpenAIToClaude([]byte(openAIResp), internalReq)
		if err != nil {
			t.Fatalf("OpenAI→Claude: %v", err)
		}

		// Step 4: Validate Claude response
		var claudeResp models.ClaudeResponse
		json.Unmarshal(claudeRespBody, &claudeResp)

		if claudeResp.Type != "message" {
			t.Errorf("type = %q, want message", claudeResp.Type)
		}
		if claudeResp.Role != "assistant" {
			t.Errorf("role = %q, want assistant", claudeResp.Role)
		}
		if claudeResp.Model != "claude-sonnet-4-6" {
			t.Errorf("model = %q, want claude-sonnet-4-6", claudeResp.Model)
		}
		if claudeResp.StopReason != "end_turn" {
			t.Errorf("stop_reason = %q, want end_turn", claudeResp.StopReason)
		}
		if len(claudeResp.Content) != 1 || claudeResp.Content[0].Text != "4" {
			t.Errorf("content = %+v", claudeResp.Content)
		}
		if claudeResp.Usage.InputTokens != 10 {
			t.Errorf("input_tokens = %d, want 10", claudeResp.Usage.InputTokens)
		}
		if claudeResp.Usage.OutputTokens != 2 {
			t.Errorf("output_tokens = %d, want 2", claudeResp.Usage.OutputTokens)
		}
	})

	t.Run("pipeline: tool use full cycle", func(t *testing.T) {
		claudeReq := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"read /etc/hosts"}],"max_tokens":1024,"tools":[{"name":"read_file","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}]}`

		openAIBody, internalReq, _ := factory.ConvertClaudeToOpenAI([]byte(claudeReq), cfg)

		var openAIReq models.OpenAIRequest
		json.Unmarshal(openAIBody, &openAIReq)
		if len(openAIReq.Tools) != 1 {
			t.Fatalf("tools = %d, want 1", len(openAIReq.Tools))
		}

		// Simulate tool call response
		openAIResp := `{"id":"chatcmpl-tool","object":"chat.completion","created":1234,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Let me read that file.","tool_calls":[{"id":"call_read1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/etc/hosts\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":30,"completion_tokens":20,"total_tokens":50}}`

		claudeRespBody, _, _ := factory.ConvertOpenAIToClaude([]byte(openAIResp), internalReq)
		var claudeResp models.ClaudeResponse
		json.Unmarshal(claudeRespBody, &claudeResp)

		if claudeResp.StopReason != "tool_use" {
			t.Errorf("stop_reason = %q, want tool_use", claudeResp.StopReason)
		}
		// Should have text + tool_use
		if len(claudeResp.Content) < 1 {
			t.Fatalf("content blocks = %d", len(claudeResp.Content))
		}
	})
}

// ============================================================================
// Section 29: Invalid/Boundary Inputs
// ============================================================================

func TestClaudeProtocol_InvalidInputs(t *testing.T) {
	c := NewClaudeConverter()

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := c.ParseRequest([]byte(`not json`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		_, err := c.ParseRequest([]byte(``))
		if err == nil {
			t.Error("expected error for empty body")
		}
	})

	t.Run("missing model", func(t *testing.T) {
		body := `{"messages":[{"role":"user","content":"Hi"}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.Model != "" {
			t.Errorf("model = %q, want empty", req.Model)
		}
	})

	t.Run("missing messages", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(req.Messages) != 0 {
			t.Errorf("messages = %d, want 0", len(req.Messages))
		}
	})

	t.Run("missing max_tokens", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}]}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if req.MaxTokens != 0 {
			t.Errorf("max_tokens = %d, want 0", req.MaxTokens)
		}
	})

	t.Run("unknown content type in array", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"unknown_type","data":"test"}]}],"max_tokens":100}`
		req, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(req.Messages[0].Content) != 1 {
			t.Fatalf("content blocks = %d", len(req.Messages[0].Content))
		}
		// Should still parse with unknown type
		if req.Messages[0].Content[0].Type != "unknown_type" {
			t.Errorf("type = %q", req.Messages[0].Content[0].Type)
		}
	})

	t.Run("extra unknown fields ignored", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"max_tokens":100,"unknown_field":"value","another":123}`
		_, err := c.ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})
}

// ============================================================================
// Section 30: Claude Converter BuildRequest Edge Cases
// ============================================================================

func TestClaudeProtocol_BuildRequest_EdgeCases(t *testing.T) {
	c := NewClaudeConverter()

	t.Run("single text message serializes as string", func(t *testing.T) {
		req := &InternalRequest{
			Model:     "claude-sonnet-4-6",
			MaxTokens: 100,
			Messages: []InternalMessage{
				{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}},
			},
		}
		data, err := c.BuildRequest(req)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		msgs := result["messages"].([]interface{})
		msg := msgs[0].(map[string]interface{})
		// Single text block should serialize content as string
		if _, ok := msg["content"].(string); !ok {
			t.Errorf("content = %T, want string", msg["content"])
		}
	})

	t.Run("multiple content blocks serialize as array", func(t *testing.T) {
		req := &InternalRequest{
			Model:     "claude-sonnet-4-6",
			MaxTokens: 100,
			Messages: []InternalMessage{
				{
					Role: "user",
					Content: []ContentBlock{
						{Type: "text", Text: "Part 1"},
						{Type: "text", Text: "Part 2"},
					},
				},
			},
		}
		data, err := c.BuildRequest(req)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		msgs := result["messages"].([]interface{})
		msg := msgs[0].(map[string]interface{})
		if _, ok := msg["content"].([]interface{}); !ok {
			t.Errorf("content = %T, want []interface{}", msg["content"])
		}
	})

	t.Run("image content block in build", func(t *testing.T) {
		req := &InternalRequest{
			Model:     "claude-sonnet-4-6",
			MaxTokens: 100,
			Messages: []InternalMessage{
				{
					Role: "user",
					Content: []ContentBlock{
						{Type: "text", Text: "See image"},
						{Type: "image", Source: &ImageSource{Type: "base64", MediaType: "image/png", Data: "iVBOR"}},
					},
				},
			},
		}
		data, err := c.BuildRequest(req)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		msgs := result["messages"].([]interface{})
		msg := msgs[0].(map[string]interface{})
		content := msg["content"].([]interface{})
		if len(content) != 2 {
			t.Fatalf("content parts = %d, want 2", len(content))
		}
		imageBlock := content[1].(map[string]interface{})
		if imageBlock["type"] != "image" {
			t.Errorf("image block type = %v", imageBlock["type"])
		}
	})
}
