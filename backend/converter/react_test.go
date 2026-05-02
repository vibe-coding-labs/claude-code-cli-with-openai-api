package converter

import (
	"strings"
	"testing"
)

func TestReactConverter_BuildReactSystemPrompt(t *testing.T) {
	tools := []ToolDefinition{
		{Name: "Bash", Description: "Run a bash command", Parameters: map[string]interface{}{"command": "string"}},
		{Name: "Read", Description: "Read a file", Parameters: map[string]interface{}{"path": "string"}},
	}
	rc := &ReactConverter{}
	prompt := rc.BuildReactSystemPrompt(tools, "You are helpful.")

	if !strings.Contains(prompt, "You are helpful.") {
		t.Error("should preserve original system prompt")
	}
	if !strings.Contains(prompt, "<tool>") {
		t.Error("should contain XML format instruction")
	}
	if !strings.Contains(prompt, "Bash") || !strings.Contains(prompt, "Read") {
		t.Error("should list all tools")
	}
}

func TestReactConverter_BuildReactSystemPrompt_EmptySystem(t *testing.T) {
	rc := &ReactConverter{}
	prompt := rc.BuildReactSystemPrompt([]ToolDefinition{}, "")
	if !strings.Contains(prompt, "TOOL CALLING") {
		t.Error("should contain tool calling instruction")
	}
}

func TestReactConverter_ParseToolCallsFromResponse_SingleTool(t *testing.T) {
	rc := &ReactConverter{}
	text := `I'll read the file for you.
<tool>
<name>Read</name>
<parameters>
{"path": "/tmp/test.txt"}
</parameters>
</tool>`

	calls, remaining, found := rc.ParseToolCallsFromResponse(text)
	if !found {
		t.Fatal("should find tool call")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "Read" {
		t.Errorf("expected Read, got %s", calls[0].Name)
	}
	if calls[0].Parameters["path"] != "/tmp/test.txt" {
		t.Errorf("unexpected parameters: %v", calls[0].Parameters)
	}
	if !strings.Contains(remaining, "I'll read the file") {
		t.Error("should preserve non-tool text")
	}
}

func TestReactConverter_ParseToolCallsFromResponse_MultipleTools(t *testing.T) {
	rc := &ReactConverter{}
	text := `<tool>
<name>Bash</name>
<parameters>
{"command": "ls -la"}
</parameters>
</tool>
<tool>
<name>Read</name>
<parameters>
{"path": "main.go"}
</parameters>
</tool>`

	calls, _, found := rc.ParseToolCallsFromResponse(text)
	if !found {
		t.Fatal("should find tool calls")
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "Bash" || calls[1].Name != "Read" {
		t.Errorf("unexpected call names: %s, %s", calls[0].Name, calls[1].Name)
	}
}

func TestReactConverter_ParseToolCallsFromResponse_NoTools(t *testing.T) {
	rc := &ReactConverter{}
	text := "Just a regular response without any tool calls."

	calls, remaining, found := rc.ParseToolCallsFromResponse(text)
	if found {
		t.Error("should not find tools in plain text")
	}
	if calls != nil {
		t.Error("calls should be nil")
	}
	if remaining != text {
		t.Error("remaining text should be unchanged")
	}
}

func TestReactConverter_ParseToolCallsFromResponse_InvalidJSON(t *testing.T) {
	rc := &ReactConverter{}
	text := `<tool>
<name>Bash</name>
<parameters>
{invalid json}
</parameters>
</tool>`

	calls, _, found := rc.ParseToolCallsFromResponse(text)
	if !found {
		t.Fatal("should find tool call even with invalid JSON")
	}
	if calls[0].Name != "Bash" {
		t.Errorf("expected Bash, got %s", calls[0].Name)
	}
	if calls[0].Parameters["raw"] == nil {
		t.Error("should fallback to raw string for invalid JSON")
	}
}

func TestReactConverter_HasPartialToolCall(t *testing.T) {
	rc := &ReactConverter{}
	if rc.HasPartialToolCall("no tool here") {
		t.Error("should not detect partial in plain text")
	}
	if !rc.HasPartialToolCall("<tool>\n<name>Bash") {
		t.Error("should detect partial tool call")
	}
	if rc.HasPartialToolCall("<tool>\n</tool>") {
		t.Error("should not detect partial for complete block")
	}
}

func TestReactConverter_ConvertToolCallsToContentBlocks(t *testing.T) {
	rc := &ReactConverter{}
	calls := []ParsedToolCall{
		{Name: "Bash", Parameters: map[string]interface{}{"command": "ls"}},
		{Name: "Read", Parameters: map[string]interface{}{"path": "main.go"}},
	}
	blocks := rc.ConvertToolCallsToContentBlocks(calls)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "tool_use" || blocks[0].Name != "Bash" {
		t.Errorf("unexpected first block: %+v", blocks[0])
	}
	if !strings.HasPrefix(blocks[0].ID, "toolu_") {
		t.Errorf("ID should start with toolu_, got %s", blocks[0].ID)
	}
}

func TestNeedsReactFallback(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"ollama/llama3", true},
		{"gpt-4o", false},
		{"claude-3-opus", false},
		{"deepseek-coder-6.7b", true},
		{"qwen2.5-72b", true},
		{"gpt-4o-mini", false},
	}
	for _, tt := range tests {
		if got := NeedsReactFallback(tt.model); got != tt.expected {
			t.Errorf("NeedsReactFallback(%q) = %v, want %v", tt.model, got, tt.expected)
		}
	}
}
