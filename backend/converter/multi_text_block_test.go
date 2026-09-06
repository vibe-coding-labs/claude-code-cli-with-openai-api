package converter

import (
	"strings"
	"testing"
)

// Regression guard: Claude Code sends user messages as multiple text blocks
// (e.g. a system-reminder block followed by the actual prompt). The old code
// kept only the first block and silently dropped the rest, so the model saw
// the reminder and answered to a greeting instead of the user's question.
func TestConvertInternalMessage_MultiTextBlocksMerged(t *testing.T) {
	c := NewConverterFactory()
	msg := &InternalMessage{Role: "user", Content: []ContentBlock{
		{Type: "text", Text: "Repeat this word: BANANA"},
		{Type: "text", Text: "What is 1+1? Answer with the number only."},
		{Type: "text", Text: "Third block: PINEAPPLE"},
	}}

	out := c.openAIConverter.convertInternalMessageToOpenAI(msg)

	content, ok := out.Content.(string)
	if !ok {
		t.Fatalf("content type = %T, want string", out.Content)
	}
	for _, want := range []string{"BANANA", "1+1", "PINEAPPLE"} {
		if !strings.Contains(content, want) {
			t.Errorf("content %q missing %q — a text block was dropped", content, want)
		}
	}
}
