package converter

import (
	"testing"
)

func TestDegenerateDetector_DSMLClosingTags(t *testing.T) {
	d := GetDegenerateDetector()

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "DSML invoke closing tag with full-width pipe",
			text:     "  </｜DSML｜invoke>",
			expected: true,
		},
		{
			name:     "DSML tool_calls closing tag with full-width pipe",
			text:     "  </｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "DSML invoke opening tag with full-width pipe",
			text:     "  <｜DSML｜invoke>",
			expected: true,
		},
		{
			name:     "DSML tool_calls opening tag with full-width pipe",
			text:     "  <｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "DSML closing tag with half-width pipe",
			text:     "  </|DSML|invoke>",
			expected: true,
		},
		{
			name:     "DSML opening tag with half-width pipe",
			text:     "  <|DSML|tool_calls>",
			expected: true,
		},
		{
			name:     "DSML tags embedded in longer output",
			text:     "Here is the result:\n  </｜DSML｜invoke>\n  </｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "normal XML should not match",
			text:     "<tool><name>read_file</name></tool>",
			expected: false,
		},
		{
			name:     "normal HTML should not match",
			text:     "<div class=\"container\">Hello</div>",
			expected: false,
		},
		{
			name:     "empty string should not match",
			text:     "",
			expected: false,
		},
		{
			name:     "plain text should not match",
			text:     "This is a normal response from the model.",
			expected: false,
		},
		{
			name:     "XML with pipe character should not match",
			text:     "<data|format>some content</data|format>",
			expected: false,
		},
		{
			name:     "real user example from issue",
			text:     "● ARGUMENTS: 继续\n  </｜DSML｜invoke>\n  </｜DSML｜tool_calls>\n\n✻ Sautéed for 8s",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := d.IsDegenerate(tt.text)
			if got != tt.expected {
				t.Errorf("IsDegenerate(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestDegenerateDetector_AddPattern(t *testing.T) {
	d := newDegenerateDetector()

	// Before adding custom pattern
	got, _ := d.IsDegenerate("  </CUSTOM_TAG>test")
	if got {
		t.Error("should not match before adding custom pattern")
	}

	// Add custom pattern
	err := d.AddPattern(`</CUSTOM_\w+>`)
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	// After adding custom pattern
	got, _ = d.IsDegenerate("  </CUSTOM_TAG>test")
	if !got {
		t.Error("should match after adding custom pattern")
	}
}

func TestDegenerateDetector_InvalidPattern(t *testing.T) {
	d := newDegenerateDetector()

	err := d.AddPattern(`[invalid regex`)
	if err == nil {
		t.Error("AddPattern should return error for invalid regex")
	}
}
