package converter

import (
	"testing"
)

func TestNormalizeToolCallID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"toolu_abc123", "toolu_abc123"},
		{"call_abc123", "toolu_abc123"},
		{"fc_abc123", "toolu_abc123"},
		{"abc123", "toolu_abc123"},
		{"", ""},
		{"toolu_", "toolu_"},
	}
	for _, tt := range tests {
		if got := NormalizeToolCallID(tt.input); got != tt.expected {
			t.Errorf("NormalizeToolCallID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"filePath", "file_path"},
		{"oldString", "old_string"},
		{"newString", "new_string"},
		{"searchText", "search_text"},
		{"already_snake", "already_snake"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		if got := CamelToSnake(tt.input); got != tt.expected {
			t.Errorf("CamelToSnake(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSnakeToCamel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"file_path", "filePath"},
		{"old_string", "oldString"},
		{"already", "already"},
	}
	for _, tt := range tests {
		if got := SnakeToCamel(tt.input); got != tt.expected {
			t.Errorf("SnakeToCamel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeToolParameters(t *testing.T) {
	params := map[string]interface{}{
		"filePath":  "/tmp/test.txt",
		"oldString": "hello",
		"command":   "ls",
	}
	normalized := NormalizeToolParameters("Edit", params)

	if _, ok := normalized["file_path"]; !ok {
		t.Error("expected filePath to be converted to file_path")
	}
	if _, ok := normalized["old_string"]; !ok {
		t.Error("expected oldString to be converted to old_string")
	}
	if _, ok := normalized["command"]; !ok {
		t.Error("command should be preserved as-is")
	}
}

func TestNormalizeToolParameters_NilInput(t *testing.T) {
	if result := NormalizeToolParameters("Bash", nil); result != nil {
		t.Error("nil input should return nil")
	}
}
