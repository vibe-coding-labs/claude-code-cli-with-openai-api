package models

import (
	"encoding/json"
	"testing"
)

// TestContentBlock_MarshalJSON covers the custom marshaler that guarantees
// text/thinking/input fields are always emitted even when empty, since
// omitting them breaks strict clients such as the Claude Code CLI.
func TestContentBlock_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		block ContentBlock
		want  map[string]interface{}
	}{
		{
			name:  "text block always includes text field even when empty",
			block: ContentBlock{Type: "text"},
			want:  map[string]interface{}{"type": "text", "text": ""},
		},
		{
			name:  "text block with content",
			block: ContentBlock{Type: "text", Text: "hello"},
			want:  map[string]interface{}{"type": "text", "text": "hello"},
		},
		{
			name:  "thinking block always includes thinking field even when empty",
			block: ContentBlock{Type: "thinking"},
			want:  map[string]interface{}{"type": "thinking", "thinking": ""},
		},
		{
			name:  "thinking block with content",
			block: ContentBlock{Type: "thinking", Thinking: "pondering"},
			want:  map[string]interface{}{"type": "thinking", "thinking": "pondering"},
		},
		{
			name:  "tool_use block with nil input is coerced to empty object",
			block: ContentBlock{Type: "tool_use", ID: "tu_1", Name: "get_weather"},
			want: map[string]interface{}{
				"type":  "tool_use",
				"id":    "tu_1",
				"name":  "get_weather",
				"input": map[string]interface{}{},
			},
		},
		{
			name: "tool_use block preserves provided input",
			block: ContentBlock{
				Type: "tool_use", ID: "tu_2", Name: "get_weather",
				Input: map[string]interface{}{"location": "SF"},
			},
			want: map[string]interface{}{
				"type":  "tool_use",
				"id":    "tu_2",
				"name":  "get_weather",
				"input": map[string]interface{}{"location": "SF"},
			},
		},
		{
			name:  "unknown type falls back to default marshaling, omitting empty optional fields",
			block: ContentBlock{Type: "tool_result", ToolUseID: "tu_3"},
			want: map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": "tu_3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.block)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Errorf("field count mismatch: got %v, want %v", got, tt.want)
			}
			for k, wv := range tt.want {
				gv, ok := got[k]
				if !ok {
					t.Errorf("missing field %q in output %v", k, got)
					continue
				}
				gotJSON, _ := json.Marshal(gv)
				wantJSON, _ := json.Marshal(wv)
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("field %q mismatch: got %s, want %s", k, gotJSON, wantJSON)
				}
			}
		})
	}
}

// TestContentBlock_MarshalJSON_InArray ensures the custom marshaler is also
// honored when blocks are nested inside a slice (e.g. MessagesResponse.Content),
// which is how it's used in practice.
func TestContentBlock_MarshalJSON_InArray(t *testing.T) {
	resp := MessagesResponse{
		Content: []ContentBlock{
			{Type: "text"},
			{Type: "tool_use", Name: "foo"},
		},
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got struct {
		Content []map[string]interface{} `json:"content"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(got.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(got.Content))
	}
	if _, ok := got.Content[0]["text"]; !ok {
		t.Errorf("expected text field present on text block: %v", got.Content[0])
	}
	if _, ok := got.Content[1]["input"]; !ok {
		t.Errorf("expected input field present on tool_use block: %v", got.Content[1])
	}
}
