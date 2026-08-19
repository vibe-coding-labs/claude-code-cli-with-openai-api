package models

import (
	"encoding/json"
	"testing"
)

func TestClaudeContentBlockMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		blk  ClaudeContentBlock
		want string
	}{
		{"empty text block must keep text field",
			ClaudeContentBlock{Type: "text", Text: ""},
			`{"type":"text","text":""}`},
		{"text block with content",
			ClaudeContentBlock{Type: "text", Text: "hi"},
			`{"type":"text","text":"hi"}`},
		{"tool_use block has no text field",
			ClaudeContentBlock{Type: "tool_use", ID: "toolu_1", Name: "bash", Input: map[string]interface{}{"cmd": "ls"}},
			`{"type":"tool_use","id":"toolu_1","name":"bash","input":{"cmd":"ls"}}`},
		{"empty thinking block must keep thinking field",
			ClaudeContentBlock{Type: "thinking", Thinking: ""},
			`{"type":"thinking","thinking":""}`},
		{"thinking block with content",
			ClaudeContentBlock{Type: "thinking", Thinking: "think"},
			`{"type":"thinking","thinking":"think"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := json.Marshal(tt.blk)
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}
			if string(out) != tt.want {
				t.Errorf("got %s, want %s", string(out), tt.want)
			}
		})
	}
}
