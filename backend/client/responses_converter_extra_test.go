package client

import (
	"encoding/json"
	"testing"
)

func TestMapStatusToFinishReason(t *testing.T) {
	tests := []struct{ status, want string }{
		{"completed", "stop"},
		{"failed", "error"},
		{"incomplete", "length"},
		{"in_progress", "stop"},
		{"", "stop"},
		{"something_unknown", "stop"},
	}
	for _, tt := range tests {
		if got := mapStatusToFinishReason(tt.status); got != tt.want {
			t.Errorf("mapStatusToFinishReason(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestConvertMessagesToResponsesInput(t *testing.T) {
	t.Run("user/system text messages become input_text message items", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{"role": "system", "content": "You are helpful."},
			map[string]interface{}{"role": "user", "content": "Hello"},
		}
		input := convertMessagesToResponsesInput(messages)
		if len(input) != 2 {
			t.Fatalf("expected 2 input items, got %d: %+v", len(input), input)
		}
		item0 := input[0].(map[string]interface{})
		if item0["type"] != "message" || item0["role"] != "system" {
			t.Errorf("unexpected system item: %+v", item0)
		}
	})

	t.Run("user content as block array joins text parts", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "part1 "},
					map[string]interface{}{"type": "text", "text": "part2"},
				},
			},
		}
		input := convertMessagesToResponsesInput(messages)
		item := input[0].(map[string]interface{})
		contentArr := item["content"].([]interface{})
		block := contentArr[0].(map[string]interface{})
		if block["text"] != "part1 part2" {
			t.Errorf("expected joined text, got %q", block["text"])
		}
	})

	t.Run("empty content user message produces no item", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{"role": "user", "content": ""},
		}
		input := convertMessagesToResponsesInput(messages)
		if len(input) != 0 {
			t.Errorf("expected no input items for empty content, got %+v", input)
		}
	})

	t.Run("tool role becomes function_call_output", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{"role": "tool", "tool_call_id": "call_1", "content": "42"},
		}
		input := convertMessagesToResponsesInput(messages)
		item := input[0].(map[string]interface{})
		if item["type"] != "function_call_output" || item["call_id"] != "call_1" || item["output"] != "42" {
			t.Errorf("unexpected tool item: %+v", item)
		}
	})

	t.Run("assistant text message becomes output_text message item", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{"role": "assistant", "content": "answer"},
		}
		input := convertMessagesToResponsesInput(messages)
		item := input[0].(map[string]interface{})
		if item["type"] != "message" || item["role"] != "assistant" {
			t.Errorf("unexpected assistant item: %+v", item)
		}
		block := item["content"].([]interface{})[0].(map[string]interface{})
		if block["type"] != "output_text" || block["text"] != "answer" {
			t.Errorf("unexpected assistant content block: %+v", block)
		}
	})

	t.Run("assistant block-array content joins text parts", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"text": "a"},
					map[string]interface{}{"text": "b"},
				},
			},
		}
		input := convertMessagesToResponsesInput(messages)
		item := input[0].(map[string]interface{})
		block := item["content"].([]interface{})[0].(map[string]interface{})
		if block["text"] != "ab" {
			t.Errorf("expected joined 'ab', got %q", block["text"])
		}
	})

	t.Run("assistant tool_calls become function_call items alongside text", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{
				"role":    "assistant",
				"content": "checking weather",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_1",
						"function": map[string]interface{}{
							"name":      "get_weather",
							"arguments": `{"city":"NYC"}`,
						},
					},
				},
			},
		}
		input := convertMessagesToResponsesInput(messages)
		if len(input) != 2 {
			t.Fatalf("expected message item + function_call item, got %d: %+v", len(input), input)
		}
		fcItem := input[1].(map[string]interface{})
		if fcItem["type"] != "function_call" || fcItem["call_id"] != "call_1" || fcItem["name"] != "get_weather" {
			t.Errorf("unexpected function_call item: %+v", fcItem)
		}
	})

	t.Run("assistant with only tool_calls and no text produces only function_call item", func(t *testing.T) {
		messages := []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":       "call_2",
						"function": map[string]interface{}{"name": "f", "arguments": "{}"},
					},
				},
			},
		}
		input := convertMessagesToResponsesInput(messages)
		if len(input) != 1 {
			t.Fatalf("expected exactly 1 item, got %d: %+v", len(input), input)
		}
	})

	t.Run("non-map message entries are skipped", func(t *testing.T) {
		messages := []interface{}{"not-a-map", 42}
		input := convertMessagesToResponsesInput(messages)
		if len(input) != 0 {
			t.Errorf("expected no items from non-map entries, got %+v", input)
		}
	})
}

func TestConvertChatToResponsesRequest(t *testing.T) {
	t.Run("full request field mapping", func(t *testing.T) {
		chatBody := `{
			"model": "gpt-5",
			"messages": [{"role":"user","content":"hi"}],
			"max_tokens": 100,
			"stream": true,
			"temperature": 0.5,
			"top_p": 0.9,
			"tool_choice": "auto",
			"user": "user-123",
			"parallel_tool_calls": true,
			"tools": [
				{"type":"function","function":{"name":"f1","description":"d1","parameters":{"type":"object"},"strict":true}}
			]
		}`
		out, err := convertChatToResponsesRequest([]byte(chatBody))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("output not valid json: %v", err)
		}
		if resp["model"] != "gpt-5" {
			t.Errorf("model mismatch: %+v", resp["model"])
		}
		if _, has := resp["messages"]; has {
			t.Errorf("messages key must be removed, got %+v", resp)
		}
		if resp["max_output_tokens"] != float64(100) {
			t.Errorf("expected max_output_tokens=100, got %+v", resp["max_output_tokens"])
		}
		if resp["stream"] != true {
			t.Errorf("expected stream=true")
		}
		if resp["temperature"] != 0.5 {
			t.Errorf("expected temperature=0.5")
		}
		if resp["top_p"] != 0.9 {
			t.Errorf("expected top_p=0.9")
		}
		if resp["tool_choice"] != "auto" {
			t.Errorf("expected tool_choice=auto")
		}
		if resp["user"] != "user-123" {
			t.Errorf("expected user=user-123")
		}
		if resp["parallel_tool_calls"] != true {
			t.Errorf("expected parallel_tool_calls=true")
		}
		input, ok := resp["input"].([]interface{})
		if !ok || len(input) != 1 {
			t.Fatalf("expected input array with 1 item, got %+v", resp["input"])
		}
		tools, ok := resp["tools"].([]interface{})
		if !ok || len(tools) != 1 {
			t.Fatalf("expected 1 flattened tool, got %+v", resp["tools"])
		}
		tool0 := tools[0].(map[string]interface{})
		if tool0["name"] != "f1" || tool0["description"] != "d1" || tool0["strict"] != true {
			t.Errorf("unexpected flattened tool: %+v", tool0)
		}
		if _, has := tool0["function"]; has {
			t.Errorf("flattened tool must not retain nested function key: %+v", tool0)
		}
	})

	t.Run("max_output_tokens already present is preserved/overridden by itself", func(t *testing.T) {
		out, err := convertChatToResponsesRequest([]byte(`{"model":"m","max_output_tokens":50}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		if resp["max_output_tokens"] != float64(50) {
			t.Errorf("expected max_output_tokens=50, got %+v", resp["max_output_tokens"])
		}
	})

	t.Run("non-function tool type passed through unchanged", func(t *testing.T) {
		out, err := convertChatToResponsesRequest([]byte(`{"model":"m","tools":[{"type":"custom_tool","foo":"bar"}]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		tools := resp["tools"].([]interface{})
		tool0 := tools[0].(map[string]interface{})
		if tool0["type"] != "custom_tool" || tool0["foo"] != "bar" {
			t.Errorf("unexpected passthrough tool: %+v", tool0)
		}
	})

	t.Run("function tool without nested function object passed through unchanged", func(t *testing.T) {
		out, err := convertChatToResponsesRequest([]byte(`{"model":"m","tools":[{"type":"function"}]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		tools := resp["tools"].([]interface{})
		tool0 := tools[0].(map[string]interface{})
		if tool0["type"] != "function" {
			t.Errorf("unexpected tool: %+v", tool0)
		}
	})

	t.Run("tools field present but not an array passed through unchanged", func(t *testing.T) {
		out, err := convertChatToResponsesRequest([]byte(`{"model":"m","tools":"not-an-array"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		if resp["tools"] != "not-an-array" {
			t.Errorf("expected tools passthrough, got %+v", resp["tools"])
		}
	})

	t.Run("messages field not an array is silently dropped from input", func(t *testing.T) {
		out, err := convertChatToResponsesRequest([]byte(`{"model":"m","messages":"oops"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		if _, has := resp["input"]; has {
			t.Errorf("expected no input key when messages is malformed, got %+v", resp)
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := convertChatToResponsesRequest([]byte(`{not-json`))
		if err == nil {
			t.Fatalf("expected error for malformed JSON")
		}
	})
}

func TestFlattenToolsRequest(t *testing.T) {
	t.Run("no tools key returns original body unchanged", func(t *testing.T) {
		body := []byte(`{"model":"m"}`)
		out, err := flattenToolsRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(out) != string(body) {
			t.Errorf("expected unchanged body, got %s", out)
		}
	})

	t.Run("tools not an array returns original body unchanged", func(t *testing.T) {
		body := []byte(`{"model":"m","tools":"weird"}`)
		out, err := flattenToolsRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(out) != string(body) {
			t.Errorf("expected unchanged body, got %s", out)
		}
	})

	t.Run("flattens nested function fields to top level", func(t *testing.T) {
		body := []byte(`{"model":"m","tools":[{"type":"function","function":{"name":"f1","parameters":{"type":"object"}}}]}`)
		out, err := flattenToolsRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		tools := resp["tools"].([]interface{})
		tool0 := tools[0].(map[string]interface{})
		if tool0["name"] != "f1" {
			t.Errorf("expected flattened name=f1, got %+v", tool0)
		}
		if _, has := tool0["function"]; has {
			t.Errorf("nested function key must be gone: %+v", tool0)
		}
	})

	t.Run("non-function type tool left untouched", func(t *testing.T) {
		body := []byte(`{"model":"m","tools":[{"type":"other","x":1}]}`)
		out, err := flattenToolsRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		tools := resp["tools"].([]interface{})
		tool0 := tools[0].(map[string]interface{})
		if tool0["type"] != "other" || tool0["x"] != float64(1) {
			t.Errorf("unexpected tool: %+v", tool0)
		}
	})

	t.Run("function type tool missing nested function object left untouched", func(t *testing.T) {
		body := []byte(`{"model":"m","tools":[{"type":"function"}]}`)
		out, err := flattenToolsRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		tools := resp["tools"].([]interface{})
		tool0 := tools[0].(map[string]interface{})
		if tool0["type"] != "function" {
			t.Errorf("unexpected tool: %+v", tool0)
		}
	})

	t.Run("non-map tool entry passed through unchanged", func(t *testing.T) {
		body := []byte(`{"model":"m","tools":["not-a-map"]}`)
		out, err := flattenToolsRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		tools := resp["tools"].([]interface{})
		if tools[0] != "not-a-map" {
			t.Errorf("expected passthrough of non-map entry, got %+v", tools[0])
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := flattenToolsRequest([]byte(`{not-json`))
		if err == nil {
			t.Fatalf("expected error for malformed JSON")
		}
	})
}

func TestConvertResponsesToChatResponse(t *testing.T) {
	t.Run("already chat.completion format passes through unchanged", func(t *testing.T) {
		body := []byte(`{"object":"chat.completion","id":"x"}`)
		out, err := convertResponsesToChatResponse(body, "gpt-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(out) != string(body) {
			t.Errorf("expected unchanged passthrough, got %s", out)
		}
	})

	t.Run("message output extracted into single choice", func(t *testing.T) {
		body := []byte(`{
			"id": "resp_1",
			"status": "completed",
			"output": [
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}
			],
			"usage": {"input_tokens": 5, "output_tokens": 3, "total_tokens": 8}
		}`)
		out, err := convertResponsesToChatResponse(body, "gpt-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		if resp["object"] != "chat.completion" || resp["model"] != "gpt-5" || resp["id"] != "resp_1" {
			t.Errorf("unexpected envelope: %+v", resp)
		}
		choices := resp["choices"].([]interface{})
		choice0 := choices[0].(map[string]interface{})
		msg := choice0["message"].(map[string]interface{})
		if msg["content"] != "hello" {
			t.Errorf("expected content=hello, got %+v", msg)
		}
		usage := resp["usage"].(map[string]interface{})
		if usage["prompt_tokens"] != float64(5) || usage["completion_tokens"] != float64(3) || usage["total_tokens"] != float64(8) {
			t.Errorf("unexpected usage mapping: %+v", usage)
		}
	})

	t.Run("function_call output extracted into tool_calls choice", func(t *testing.T) {
		body := []byte(`{
			"id": "resp_2",
			"status": "completed",
			"output": [
				{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"}
			]
		}`)
		out, err := convertResponsesToChatResponse(body, "gpt-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		choices := resp["choices"].([]interface{})
		choice0 := choices[0].(map[string]interface{})
		if choice0["finish_reason"] != "tool_calls" {
			t.Errorf("expected finish_reason=tool_calls, got %+v", choice0)
		}
	})

	t.Run("no output produces single empty-message choice with mapped finish_reason", func(t *testing.T) {
		body := []byte(`{"id":"resp_3","status":"incomplete"}`)
		out, err := convertResponsesToChatResponse(body, "gpt-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		choices := resp["choices"].([]interface{})
		if len(choices) != 1 {
			t.Fatalf("expected 1 fallback choice, got %+v", choices)
		}
		choice0 := choices[0].(map[string]interface{})
		if choice0["finish_reason"] != "length" {
			t.Errorf("expected finish_reason=length (from status=incomplete), got %+v", choice0)
		}
	})

	t.Run("output present but empty array falls back to single empty choice", func(t *testing.T) {
		body := []byte(`{"id":"resp_4","status":"completed","output":[]}`)
		out, err := convertResponsesToChatResponse(body, "gpt-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		choices := resp["choices"].([]interface{})
		if len(choices) != 1 {
			t.Fatalf("expected fallback single choice, got %+v", choices)
		}
	})

	t.Run("output items of unrelated type are ignored", func(t *testing.T) {
		body := []byte(`{"id":"resp_5","status":"completed","output":[{"type":"reasoning","text":"thinking..."}]}`)
		out, err := convertResponsesToChatResponse(body, "gpt-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(out, &resp)
		choices := resp["choices"].([]interface{})
		if len(choices) != 1 {
			t.Fatalf("expected fallback single choice since no message/function_call items, got %+v", choices)
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := convertResponsesToChatResponse([]byte(`{not-json`), "gpt-5")
		if err == nil {
			t.Fatalf("expected error for malformed JSON")
		}
	})
}

func TestExtractChoiceFromMessage(t *testing.T) {
	t.Run("defaults role to assistant when missing", func(t *testing.T) {
		item := map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"type": "output_text", "text": "hi"}},
		}
		choice := extractChoiceFromMessage(item)
		msg := choice["message"].(map[string]interface{})
		if msg["role"] != "assistant" {
			t.Errorf("expected default role=assistant, got %+v", msg)
		}
	})

	t.Run("extracts tool_use/function_call parts as tool_calls", func(t *testing.T) {
		item := map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{"type": "function_call", "id": "call_1", "name": "f", "arguments": "{}"},
				map[string]interface{}{"type": "tool_use", "id": "call_2", "name": "g", "arguments": "{}"},
			},
		}
		choice := extractChoiceFromMessage(item)
		msg := choice["message"].(map[string]interface{})
		toolCalls := msg["tool_calls"].([]map[string]interface{})
		if len(toolCalls) != 2 {
			t.Fatalf("expected 2 tool_calls, got %+v", toolCalls)
		}
		if choice["finish_reason"] != "stop" {
			t.Errorf("extractChoiceFromMessage always reports finish_reason=stop, got %+v", choice)
		}
	})

	t.Run("no tool calls omits tool_calls key", func(t *testing.T) {
		item := map[string]interface{}{
			"role":    "assistant",
			"content": []interface{}{map[string]interface{}{"type": "output_text", "text": "hi"}},
		}
		choice := extractChoiceFromMessage(item)
		msg := choice["message"].(map[string]interface{})
		if _, has := msg["tool_calls"]; has {
			t.Errorf("expected no tool_calls key, got %+v", msg)
		}
	})
}

func TestExtractChoiceFromFunctionCall(t *testing.T) {
	item := map[string]interface{}{
		"name":      "get_weather",
		"arguments": `{"city":"NYC"}`,
		"call_id":   "call_9",
	}
	choice := extractChoiceFromFunctionCall(item)
	if choice["finish_reason"] != "tool_calls" {
		t.Errorf("expected finish_reason=tool_calls, got %+v", choice)
	}
	msg := choice["message"].(map[string]interface{})
	toolCalls := msg["tool_calls"].([]interface{})
	tc0 := toolCalls[0].(map[string]interface{})
	if tc0["id"] != "call_9" {
		t.Errorf("unexpected tool call id: %+v", tc0)
	}
	fn := tc0["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Errorf("unexpected function name: %+v", fn)
	}
}
