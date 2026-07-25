package converter

import (
	"strings"
	"testing"
)

// TestEdgeCases_EmptyContentVsToolOnly tests the critical distinction between:
// - Empty content (degenerate, should retry)
// - Tool-only response (normal, should NOT retry)
func TestEdgeCases_EmptyContentVsToolOnly(t *testing.T) {
	d := GetDegenerateDetector()

	// Case 1: Pure empty — should be detected as degenerate
	if !d.IsEmptyContent("", false) {
		t.Error("empty text with no tools should be degenerate")
	}

	// Case 2: Whitespace only — should be detected as degenerate
	if !d.IsEmptyContent("   \n\t  ", false) {
		t.Error("whitespace-only text with no tools should be degenerate")
	}

	// Case 3: Empty text but has tool calls — should NOT be degenerate
	// This is the key distinction: tool-only responses are valid
	if d.IsEmptyContent("", true) {
		t.Error("empty text with tool calls should NOT be degenerate (valid tool-only response)")
	}

	// Case 4: Whitespace text but has tool calls — should NOT be degenerate
	if d.IsEmptyContent("  ", true) {
		t.Error("whitespace text with tool calls should NOT be degenerate")
	}

	// Case 5: Normal text, no tools — should NOT be degenerate
	if d.IsEmptyContent("Hello", false) {
		t.Error("normal text should NOT be degenerate")
	}

	// Case 6: Normal text with tools — should NOT be degenerate
	if d.IsEmptyContent("I'll help you", true) {
		t.Error("normal text with tools should NOT be degenerate")
	}
}

// TestEdgeCases_DSMLOnly tests that DSML markers are detected even with surrounding whitespace
func TestEdgeCases_DSMLOnly(t *testing.T) {
	d := GetDegenerateDetector()

	testCases := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "DSML with surrounding whitespace",
			text:     "   \n  </｜DSML｜invoke>  \n  ",
			expected: true,
		},
		{
			name:     "only DSML marker",
			text:     "</｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "DSML in middle of text",
			text:     "Some text\n</｜DSML｜invoke>\nMore text",
			expected: true,
		},
		{
			name:     "legitimate XML should not match",
			text:     "<response><status>ok</status></response>",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := d.IsDegenerate(tc.text)
			if got != tc.expected {
				t.Errorf("IsDegenerate(%q) = %v, want %v", tc.text, got, tc.expected)
			}
		})
	}
}

// TestEdgeCases_WhitespaceVariants tests various whitespace patterns
func TestEdgeCases_WhitespaceVariants(t *testing.T) {
	d := GetDegenerateDetector()

	whitespaceVariants := []string{
		"",
		" ",
		"  ",
		"\t",
		"\n",
		"\r\n",
		" \t\n\r\n ",
		"     \n\n\n     ",
		strings.Repeat(" ", 100),
		strings.Repeat("\n", 50),
	}

	for i, text := range whitespaceVariants {
		t.Run("whitespace_variant_"+string(rune('0'+i)), func(t *testing.T) {
			// With no tool calls, whitespace should be detected as empty content
			if !d.IsEmptyContent(text, false) {
				t.Errorf("whitespace variant %d should be detected as empty content", i)
			}
			// With tool calls, whitespace should NOT be detected as empty content
			if d.IsEmptyContent(text, true) {
				t.Errorf("whitespace variant %d with tools should NOT be detected as empty content", i)
			}
		})
	}
}
