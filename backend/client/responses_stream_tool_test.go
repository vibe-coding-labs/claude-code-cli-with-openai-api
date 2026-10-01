package client

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesBridgeUsesCompleteArgumentsFromDone(t *testing.T) {
	var out bytes.Buffer
	input := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call-1","name":"lookup"}}`,
		`data: {"type":"response.function_call_arguments.done","call_id":"call-1","arguments":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		"",
	}, "\n")
	if err := convertResponsesStreamingToChat(strings.NewReader(input), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"arguments":"{\"q\":\"x\"}"`) {
		t.Fatalf("complete arguments were not forwarded: %s", out.String())
	}
}

func TestResponsesBridgeInterleavedItemIDs(t *testing.T) {
	input := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item-1","call_id":"call-1","name":"first"}}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item-2","call_id":"call-2","name":"second"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-1","delta":"{\"a\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-2","delta":"{\"b\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-1","delta":"1}"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-2","delta":"2}"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item-1"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item-2"}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		"",
	}, "\n\n")
	var out bytes.Buffer
	if err := convertResponsesStreamingToChat(strings.NewReader(input), &out, "test"); err != nil {
		t.Fatal(err)
	}
	calls := make(map[string]string)
	for _, line := range strings.Split(out.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			t.Fatal(err)
		}
		for _, choice := range chunk.Choices {
			for _, call := range choice.Delta.ToolCalls {
				calls[call.ID] = call.Function.Arguments
			}
		}
	}
	if len(calls) != 2 || calls["call-1"] != `{"a":1}` || calls["call-2"] != `{"b":2}` {
		t.Fatalf("interleaved calls were corrupted: %#v", calls)
	}
}

func TestResponsesBridgeUnknownToolIdentifierFails(t *testing.T) {
	for _, event := range []string{
		`{"type":"response.function_call_arguments.delta","item_id":"unknown","delta":"{}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"unknown","arguments":"{}"}`,
	} {
		t.Run(event, func(t *testing.T) {
			input := "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"lookup\"}}\n\ndata: " + event + "\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
			var out bytes.Buffer
			if err := convertResponsesStreamingToChat(strings.NewReader(input), &out, "test"); err == nil {
				t.Fatal("unknown identifier was accepted")
			}
			if strings.Contains(out.String(), "tool_calls") || strings.Contains(out.String(), "[DONE]") {
				t.Fatalf("unknown identifier produced successful output: %s", out.String())
			}
		})
	}
}
