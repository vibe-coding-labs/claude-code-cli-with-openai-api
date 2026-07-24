package converter

import (
	"strings"
	"testing"
)

func TestDegenerateDetector_StreamContentDetection(t *testing.T) {
	d := GetDegenerateDetector()

	// Simulate content that would be collected during streaming
	testCases := []struct {
		name         string
		content      string
		isDegenerate bool
	}{
		{
			name:         "real issue example - DSML invoke and tool_calls",
			content:      "● ARGUMENTS: 继续\n  </｜DSML｜invoke>\n  </｜DSML｜tool_calls>\n\n✻ Sautéed for 8s",
			isDegenerate: true,
		},
		{
			name:         "only DSML invoke closing",
			content:      "Some text before\n  </｜DSML｜invoke>\nSome text after",
			isDegenerate: true,
		},
		{
			name:         "only DSML tool_calls closing",
			content:      "  </｜DSML｜tool_calls>",
			isDegenerate: true,
		},
		{
			name:         "half-width pipe variant",
			content:      "  </|DSML|invoke>\n  </|DSML|tool_calls>",
			isDegenerate: true,
		},
		{
			name:         "normal tool use XML - not degenerate",
			content:      "<tool><name>read_file</name><parameters><path>/tmp/test</path></parameters></tool>",
			isDegenerate: false,
		},
		{
			name:         "normal markdown response - not degenerate",
			content:      "Here's the implementation:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```",
			isDegenerate: false,
		},
		{
			name:         "empty content - not degenerate",
			content:      "",
			isDegenerate: false,
		},
		{
			name:         "legitimate XML with pipes - not degenerate",
			content:      "<data|separator>value</data|separator>",
			isDegenerate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, pattern := d.IsDegenerate(tc.content)
			if got != tc.isDegenerate {
				t.Errorf("IsDegenerate(%q) = %v (pattern=%s), want %v",
					tc.content, got, pattern, tc.isDegenerate)
			}
		})
	}
}

func TestDegenerateDetector_MultiplePatternsInContent(t *testing.T) {
	d := GetDegenerateDetector()

	// Content with multiple degenerate markers
	content := "Starting analysis...\n<｜DSML｜invoke>\n  </｜DSML｜tool_calls>\nDone."
	got, _ := d.IsDegenerate(content)
	if !got {
		t.Error("content with multiple DSML markers should be detected as degenerate")
	}
}

func TestDegenerateDetector_StreamingIncrementalContent(t *testing.T) {
	d := GetDegenerateDetector()

	// Simulate how content accumulates during streaming
	chunks := []string{
		"● ARGUMENTS: 继续\n",
		"  </｜DSML｜invoke>\n",
		"  </｜DSML｜tool_calls>\n",
		"\n✻ Sautéed for 8s",
	}

	var full strings.Builder
	detectedAtChunk := -1
	for i, chunk := range chunks {
		full.WriteString(chunk)
		if isDegenerate, _ := d.IsDegenerate(full.String()); isDegenerate {
			detectedAtChunk = i
			break
		}
	}

	if detectedAtChunk < 0 {
		t.Error("degenerate content should be detected at some point during accumulation")
	}
	if detectedAtChunk != 1 {
		t.Errorf("expected detection at chunk 1 (first DSML marker), got chunk %d", detectedAtChunk)
	}
}
