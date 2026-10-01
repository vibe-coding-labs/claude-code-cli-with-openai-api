package client

import (
	"bytes"
	"strings"
	"testing"
)

func TestResponsesBridgeRequiresTerminalEvent(t *testing.T) {
	for _, tail := range []string{"", "data: [DONE]\n\n", "data: {bad}\n\n", "data: {\"type\":\"response.failed\"}\n\n"} {
		t.Run(tail, func(t *testing.T) {
			var out bytes.Buffer
			input := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" + tail
			if err := convertResponsesStreamingToChat(strings.NewReader(input), &out, "test"); err == nil {
				t.Fatal("failed stream reported success")
			}
			if strings.Contains(out.String(), "[DONE]") || strings.Contains(out.String(), "finish_reason") {
				t.Fatalf("failed stream emitted successful terminal: %s", out.String())
			}
		})
	}
}

func TestResponsesBridgeIncompleteMapsToLength(t *testing.T) {
	var out bytes.Buffer
	input := "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n"
	if err := convertResponsesStreamingToChat(strings.NewReader(input), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"finish_reason":"length"`) || !strings.Contains(out.String(), "[DONE]") {
		t.Fatalf("missing length terminal: %s", out.String())
	}
}

func TestResponsesBridgeCompletedWithoutUsage(t *testing.T) {
	var out bytes.Buffer
	input := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
	if err := convertResponsesStreamingToChat(strings.NewReader(input), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "finish_reason") || !strings.Contains(out.String(), "[DONE]") {
		t.Fatalf("missing terminal: %s", out.String())
	}
}
