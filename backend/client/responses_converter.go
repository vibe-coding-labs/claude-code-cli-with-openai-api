package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// convertChatToResponsesRequest transforms a Chat Completions request body into a
// Responses API request body. It handles the core field mapping:
//
//	messages -> input
//	max_tokens -> max_output_tokens (if present)
//	stream, model, tools, tool_choice, temperature, top_p preserved
//
// Convert messages -> input. The Responses API input accepts a list of items:
//
//	message objects           ({type: "message", role, content})
//	function_call items        ({type: "function_call", call_id, name, arguments})
//	function_call_output items ({type: "function_call_output", call_id, output})
//
// We translate Chat Completions messages into this shape:
//
//	user / assistant text  -> message item
//	assistant tool_calls   -> function_call items (each tool call)
//	tool role messages     -> function_call_output items (call_id + output)
func convertMessagesToResponsesInput(messages []interface{}) []interface{} {
	var input []interface{}

	for _, mRaw := range messages {
		m, ok := mRaw.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := m["role"].(string)

		switch role {
		case "tool":
			// function_call_output
			callID, _ := m["tool_call_id"].(string)
			output, _ := m["content"].(string)
			item := map[string]interface{}{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			}
			input = append(input, item)

		case "assistant":
			// Message text
			content := ""
			switch c := m["content"].(type) {
			case string:
				content = c
			case []interface{}:
				var parts []string
				for _, pRaw := range c {
					if p, ok := pRaw.(map[string]interface{}); ok {
						if t, ok := p["text"].(string); ok {
							parts = append(parts, t)
						}
					}
				}
				content = strings.Join(parts, "")
			}
			if content != "" {
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    "assistant",
					"content": []interface{}{map[string]interface{}{"type": "output_text", "text": content}},
				})
			}

			// Tool calls -> function_call items
			if tcs, ok := m["tool_calls"].([]interface{}); ok {
				for _, tcRaw := range tcs {
					tc, _ := tcRaw.(map[string]interface{})
					callID, _ := tc["id"].(string)
					fn, _ := tc["function"].(map[string]interface{})
					name, _ := fn["name"].(string)
					args, _ := fn["arguments"].(string)
					input = append(input, map[string]interface{}{
						"type":      "function_call",
						"call_id":   callID,
						"name":      name,
						"arguments": args,
					})
				}
			}

		default:
			// user and system roles
			content := ""
			switch c := m["content"].(type) {
			case string:
				content = c
			case []interface{}:
				var parts []string
				for _, pRaw := range c {
					if p, ok := pRaw.(map[string]interface{}); ok {
						if t, ok := p["text"].(string); ok {
							parts = append(parts, t)
						}
					}
				}
				content = strings.Join(parts, "")
			}
			if content != "" {
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    role,
					"content": []interface{}{map[string]interface{}{"type": "input_text", "text": content}},
				})
			}
		}
	}

	return input
}

func convertChatToResponsesRequest(chatBody []byte) ([]byte, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(chatBody, &raw); err != nil {
		return nil, fmt.Errorf("chat-to-responses: %w", err)
	}

	// Build responses request
	resp := make(map[string]interface{})

	// Copy model
	if model, ok := raw["model"]; ok {
		resp["model"] = model
	}

	// Convert messages -> input
	if messagesRaw, ok := raw["messages"]; ok {
		if messages, ok := messagesRaw.([]interface{}); ok {
			resp["input"] = convertMessagesToResponsesInput(messages)
		}
	}

	// Remove fields that don't exist in Responses API
	delete(raw, "messages")

	// max_tokens -> max_output_tokens
	if mt, ok := raw["max_tokens"]; ok {
		resp["max_output_tokens"] = mt
	}
	if mot, ok := raw["max_output_tokens"]; ok {
		resp["max_output_tokens"] = mot
	}

	// Copy stream setting
	if stream, ok := raw["stream"]; ok {
		resp["stream"] = stream
	}

	// Copy tools, converting from Chat Completions format to Responses API format
	// Chat Completions: {type: "function", function: {name, description, parameters}}
	// Responses API:   {type: "function", name, description, parameters}
	if tools, ok := raw["tools"]; ok {
		if toolsArr, ok := tools.([]interface{}); ok {
			converted := make([]interface{}, 0, len(toolsArr))
			for _, tRaw := range toolsArr {
				if t, ok := tRaw.(map[string]interface{}); ok {
					ttype, _ := t["type"].(string)
					if ttype == "function" {
						if fn, ok := t["function"].(map[string]interface{}); ok {
							newTool := make(map[string]interface{})
							newTool["type"] = "function"
							if n, ok := fn["name"]; ok {
								newTool["name"] = n
							}
							if d, ok := fn["description"]; ok {
								newTool["description"] = d
							}
							if p, ok := fn["parameters"]; ok {
								newTool["parameters"] = p
							}
							if s, ok := fn["strict"]; ok {
								newTool["strict"] = s
							}
							converted = append(converted, newTool)
						} else {
							converted = append(converted, t)
						}
					} else {
						converted = append(converted, t)
					}
				} else {
					converted = append(converted, tRaw)
				}
			}
			resp["tools"] = converted
		} else {
			resp["tools"] = tools
		}
	}

	// Copy tool_choice
	if tc, ok := raw["tool_choice"]; ok {
		resp["tool_choice"] = tc
	}

	// Copy temperature
	if temp, ok := raw["temperature"]; ok {
		resp["temperature"] = temp
	}

	// Copy top_p
	if tp, ok := raw["top_p"]; ok {
		resp["top_p"] = tp
	}

	// Copy metadata/user if present
	if user, ok := raw["user"]; ok {
		resp["user"] = user
	}

	// Copy parallel_tool_calls
	if ptc, ok := raw["parallel_tool_calls"]; ok {
		resp["parallel_tool_calls"] = ptc
	}

	return json.Marshal(resp)
}

// flattenToolsRequest transforms a Chat Completions request body so that the
// tools array uses flat format (name at top level) instead of nested function
// format. Certain upstreams (e.g. opencode.ai) require this format.
func flattenToolsRequest(chatBody []byte) ([]byte, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(chatBody, &raw); err != nil {
		return nil, fmt.Errorf("flatten-tools: %w", err)
	}
	toolsRaw, hasTools := raw["tools"]
	if !hasTools {
		return chatBody, nil
	}
	toolsArr, ok := toolsRaw.([]interface{})
	if !ok {
		return chatBody, nil
	}
	converted := make([]interface{}, 0, len(toolsArr))
	for _, tRaw := range toolsArr {
		t, ok := tRaw.(map[string]interface{})
		if !ok {
			converted = append(converted, tRaw)
			continue
		}
		ttype, _ := t["type"].(string)
		if ttype != "function" {
			converted = append(converted, t)
			continue
		}
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			converted = append(converted, t)
			continue
		}
		newTool := make(map[string]interface{})
		newTool["type"] = "function"
		for k, v := range fn {
			newTool[k] = v
		}
		converted = append(converted, newTool)
	}
	raw["tools"] = converted
	return json.Marshal(raw)
}

// convertResponsesToChatResponse transforms a Responses API response body into a
// Chat Completions response body. Extracts the first message output item.
// If the response is already Chat Completions format (object = "chat.completion"),
// it passes through unchanged.
func convertResponsesToChatResponse(respBody []byte, chatModel string) ([]byte, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("responses-to-chat: %w", err)
	}

	// If upstream already returned Chat Completions format, pass through directly
	if obj, _ := raw["object"].(string); obj == "chat.completion" {
		return respBody, nil
	}

	status, _ := raw["status"].(string)
	respID, _ := raw["id"].(string)

	// Build choices from output
	outputRaw, hasOutput := raw["output"]
	var choices []map[string]interface{}

	if hasOutput {
		if outputArr, ok := outputRaw.([]interface{}); ok {
			for _, itemRaw := range outputArr {
				if item, ok := itemRaw.(map[string]interface{}); ok {
					itemType, _ := item["type"].(string)
					if itemType == "message" {
						choice := extractChoiceFromMessage(item)
						if choice != nil {
							choices = append(choices, choice)
						}
					}
					if itemType == "function_call" {
						choice := extractChoiceFromFunctionCall(item)
						if choice != nil {
							choices = append(choices, choice)
						}
					}
				}
			}
		}
	}

	// If no output, provide empty choices
	if len(choices) == 0 {
		finishReason := mapStatusToFinishReason(status)
		choices = []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant"},
				"finish_reason": finishReason,
			},
		}
	}

	// Map usage field names
	usage := make(map[string]interface{})
	if u, ok := raw["usage"].(map[string]interface{}); ok {
		if pt, ok := u["input_tokens"]; ok {
			usage["prompt_tokens"] = pt
		}
		if ot, ok := u["output_tokens"]; ok {
			usage["completion_tokens"] = ot
		}
		if tt, ok := u["total_tokens"]; ok {
			usage["total_tokens"] = tt
		}
	}

	chatResp := map[string]interface{}{
		"id":      respID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   chatModel,
		"choices": choices,
		"usage":   usage,
	}

	return json.Marshal(chatResp)
}

// convertResponsesStreamingToChat reads Responses API SSE events from reader and
// writes Chat Completions SSE format to writer. Returns when the stream ends.
func convertResponsesStreamingToChat(reader io.Reader, writer io.Writer, chatModel string) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	roleSent := false
	var fullText strings.Builder
	type toolCall struct {
		ID    string
		Name  string
		Args  strings.Builder
		Index int
	}
	var toolCalls []toolCall
	lastToolIdx := -1
	finishReason := ""

	writeSSE := func(data map[string]interface{}) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(writer, "data: %s\n\n", string(b))
		return err
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		var chunkData string
		switch {
		case strings.HasPrefix(line, "data: "):
			chunkData = strings.TrimPrefix(line, "data: ")
		case strings.HasPrefix(line, "data:"):
			chunkData = strings.TrimPrefix(line, "data:")
		default:
			continue
		}

		if strings.TrimSpace(chunkData) == "[DONE]" {
			break
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(chunkData), &raw); err != nil {
			continue
		}

		eventType, _ := raw["type"].(string)

		switch eventType {
		case "response.output_item.added":
			// Capture the function_call item's id+name for later emission. We do
			// NOT emit a tool_calls delta here (with empty arguments) — that would
			// make the downstream converter open an empty tool_use block which is
			// then left dangling (partial_json "{}") before the real arguments
			// arrive in a second block. Instead we buffer and emit the complete
			// tool call once on function_call_arguments.done.
			if item, ok := raw["item"].(map[string]interface{}); ok {
				if itype, _ := item["type"].(string); itype == "function_call" {
					itemName, _ := item["name"].(string)
					itemCallID, _ := item["call_id"].(string)
					if itemName != "" {
						found := false
						for i, tc := range toolCalls {
							if tc.ID == itemCallID {
								found = true
								lastToolIdx = i
								break
							}
						}
						if !found {
							tc := toolCall{ID: itemCallID, Name: itemName, Index: 0}
							toolCalls = append(toolCalls, tc)
							lastToolIdx = len(toolCalls) - 1
						}
					}
				}
			}

		case "response.output_text.delta":
			delta, _ := raw["delta"].(string)
			fullText.WriteString(delta)

			choice := map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"content": delta,
				},
			}
			if !roleSent {
				choice["delta"].(map[string]interface{})["role"] = "assistant"
				roleSent = true
			}
			if err := writeSSE(map[string]interface{}{
				"choices": []interface{}{choice},
			}); err != nil {
				return err
			}

		case "response.function_call_arguments.delta":
			// Accumulate argument fragments into the tracked tool call. Do NOT
			// emit a tool_calls delta per fragment — the downstream Claude
			// streaming converter opens a NEW content_block per tool_calls delta,
			// which would split one tool call across many blocks. We emit the
			// full tool call once on function_call_arguments.done instead.
			delta, _ := raw["delta"].(string)
			callID, _ := raw["call_id"].(string)

			found := false
			for i, tc := range toolCalls {
				if tc.ID == callID {
					toolCalls[i].Args.WriteString(delta)
					found = true
					lastToolIdx = i
					break
				}
			}
			if !found {
				// opencode.ai does NOT include call_id in arguments.delta events.
				// If we already announced a tool call via output_item.added,
				// append the arguments to the most recent one. Otherwise create
				// a fresh entry keyed by callID (or empty).
				if len(toolCalls) > 0 && lastToolIdx >= 0 {
					toolCalls[lastToolIdx].Args.WriteString(delta)
					found = true
				} else {
					tc := toolCall{ID: callID, Index: len(toolCalls)}
					tc.Args.WriteString(delta)
					toolCalls = append(toolCalls, tc)
					lastToolIdx = len(toolCalls) - 1
				}
			}

		case "response.function_call_arguments.done":
			// Arguments complete: emit ONE tool_calls delta per tracked call.
			callID, _ := raw["call_id"].(string)
			if callID == "" && len(toolCalls) > 0 && lastToolIdx >= 0 {
				callID = toolCalls[lastToolIdx].ID
			}
			var targetIdx = lastToolIdx
			if callID != "" {
				for i, tc := range toolCalls {
					if tc.ID == callID {
						targetIdx = i
						break
					}
				}
			}
			if targetIdx < 0 || targetIdx >= len(toolCalls) {
				break
			}
			tc := toolCalls[targetIdx]
			toolDelta := map[string]interface{}{
				"index":    targetIdx,
				"id":       tc.ID,
				"type":     "function",
				"function": map[string]interface{}{"name": tc.Name, "arguments": tc.Args.String()},
			}
			choice := map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{toolDelta},
				},
			}
			if !roleSent {
				choice["delta"].(map[string]interface{})["role"] = "assistant"
				roleSent = true
			}
			if err := writeSSE(map[string]interface{}{
				"choices": []interface{}{choice},
			}); err != nil {
				return err
			}

		case "response.completed":
			response, _ := raw["response"].(map[string]interface{})
			if response != nil {
				respStatus, _ := response["status"].(string)
				finishReason = mapStatusToFinishReason(respStatus)
			}

			usage := map[string]interface{}{}
			if response != nil {
				if u, ok := response["usage"].(map[string]interface{}); ok {
					if pt, ok := u["input_tokens"]; ok {
						usage["prompt_tokens"] = pt
					}
					if ot, ok := u["output_tokens"]; ok {
						usage["completion_tokens"] = ot
					}
					if tt, ok := u["total_tokens"]; ok {
						usage["total_tokens"] = tt
					}
				}
			}

			if len(usage) > 0 {
				choice := map[string]interface{}{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": finishReason,
				}
				if err := writeSSE(map[string]interface{}{
					"choices": []interface{}{choice},
					"usage":   usage,
				}); err != nil {
					return err
				}
			}

			fmt.Fprintf(writer, "data: [DONE]\n\n")
			return nil

		case "response.failed":
			writeSSE(map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "error",
					},
				},
			})
			fmt.Fprintf(writer, "data: [DONE]\n\n")
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("responses streaming scanner: %w", err)
	}

	if finishReason == "" {
		finishReason = "stop"
	}
	choice := map[string]interface{}{
		"index":         0,
		"delta":         map[string]interface{}{},
		"finish_reason": finishReason,
	}
	writeSSE(map[string]interface{}{
		"choices": []interface{}{choice},
	})
	fmt.Fprintf(writer, "data: [DONE]\n\n")
	return nil
}

// extractChoiceFromMessage extracts a Chat Completions choice from a Responses
// message output item.
func extractChoiceFromMessage(item map[string]interface{}) map[string]interface{} {
	role, _ := item["role"].(string)
	if role == "" {
		role = "assistant"
	}

	contentRaw, _ := item["content"].([]interface{})
	var text string
	var toolCalls []map[string]interface{}

	for _, partRaw := range contentRaw {
		if part, ok := partRaw.(map[string]interface{}); ok {
			partType, _ := part["type"].(string)
			switch partType {
			case "output_text":
				if t, ok := part["text"].(string); ok {
					text = t
				}
			case "function_call", "tool_use":
				fc := map[string]interface{}{
					"id":   part["id"],
					"type": "function",
					"function": map[string]interface{}{
						"name":      part["name"],
						"arguments": part["arguments"],
					},
				}
				toolCalls = append(toolCalls, fc)
			}
		}
	}

	message := map[string]interface{}{
		"role":    role,
		"content": text,
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	return map[string]interface{}{
		"index":         0,
		"message":       message,
		"finish_reason": "stop",
	}
}

// extractChoiceFromFunctionCall extracts a choice from a function_call output item.
func extractChoiceFromFunctionCall(item map[string]interface{}) map[string]interface{} {
	name, _ := item["name"].(string)
	args, _ := item["arguments"].(string)
	callID, _ := item["call_id"].(string)

	return map[string]interface{}{
		"index": 0,
		"message": map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id":   callID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      name,
						"arguments": args,
					},
				},
			},
		},
		"finish_reason": "tool_calls",
	}
}

// mapStatusToFinishReason maps a Responses API status to a Chat Completions
// finish_reason.
func mapStatusToFinishReason(status string) string {
	switch status {
	case "completed":
		return "stop"
	case "failed":
		return "error"
	case "incomplete":
		return "length"
	default:
		return "stop"
	}
}
