package converter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/types"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// This file implements the OpenAI Responses API ingress.
//
// Codex CLI speaks the Responses API (/v1/responses); many upstreams (e.g.
// Xunfei/iFlytek MaaS) only speak Chat Completions. These functions translate
// Responses <-> Chat Completions directly on models.OpenAIRequest, bypassing
// the Claude-centric InternalRequest layer (which buys nothing for two
// OpenAI-shaped formats). The logic is ported faithfully from the validated
// Python reference at ~/.local/bin/codex-responses-proxy.py, with a handful of
// deliberate correctness divergences (local sequence_number, usage captured
// before the empty-choices skip, message-item omission on tool-only turns, no
// heartbeat, finish_reason->status mapping, request-field echo-back,
// developer->system coercion, reasoning ignored in v1).

// genResponsesID returns an opaque id like "resp<hex>" / "msg<hex>" / "fc<hex>".
func genResponsesID(prefix string) string {
	h := strings.ReplaceAll(uuid.New().String(), "-", "")
	return prefix + h[:24]
}

// ConvertResponsesToOpenAIRequest parses a Responses API request body and builds
// a Chat Completions upstream request. It also returns the parsed body so the
// response translators can echo request fields (instructions/tools/top_p/...)
// back onto the Responses response object.
func ConvertResponsesToOpenAIRequest(body []byte) (*models.OpenAIRequest, map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}

	model, _ := raw["model"].(string)
	if model == "" {
		model = "gpt-4"
	}

	instructions, _ := raw["instructions"].(string)
	messages := responsesInputToMessages(raw["input"], instructions)

	req := &models.OpenAIRequest{
		Model:    model,
		Messages: messages,
	}

	if stream, ok := raw["stream"].(bool); ok {
		req.Stream = stream
	}
	if toolsRaw, ok := raw["tools"].([]interface{}); ok {
		if tools := convertResponsesTools(toolsRaw); len(tools) > 0 {
			req.Tools = tools
		}
	}
	if tc, ok := raw["tool_choice"]; ok && tc != nil {
		req.ToolChoice = tc
	}
	if temp, ok := raw["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if topP, ok := raw["top_p"].(float64); ok {
		tp := topP
		req.TopP = &tp
	}
	// Responses uses max_output_tokens; fall back to max_tokens. Unlike the
	// Claude path we do NOT default it: Responses treats it as optional.
	if mot, ok := raw["max_output_tokens"].(float64); ok {
		req.MaxTokens = int(mot)
	} else if mt, ok := raw["max_tokens"].(float64); ok {
		req.MaxTokens = int(mt)
	}
	if ptc, ok := raw["parallel_tool_calls"].(bool); ok {
		p := ptc
		req.ParallelToolCalls = &p
	}

	return req, raw, nil
}

// responsesInputToMessages converts a Responses `input` (string | item list) to
// Chat Completions messages. Pending assistant tool_calls are flushed before
// message items and before function_call_output items, matching the Python
// reference so tool calls and their results pair correctly.
func responsesInputToMessages(input interface{}, instructions string) []models.OpenAIMessage {
	var messages []models.OpenAIMessage

	if instructions != "" {
		messages = append(messages, models.OpenAIMessage{
			Role:    "system",
			Content: instructions,
		})
	}

	if s, ok := input.(string); ok {
		messages = append(messages, models.OpenAIMessage{Role: "user", Content: s})
		return messages
	}

	items, ok := input.([]interface{})
	if !ok {
		return messages
	}

	var pendingToolCalls []models.OpenAIToolCall
	flushPending := func() {
		if len(pendingToolCalls) == 0 {
			return
		}
		messages = append(messages, models.OpenAIMessage{
			Role:      "assistant",
			ToolCalls: pendingToolCalls,
			Content:   nil,
		})
		pendingToolCalls = nil
	}

	for _, itemRaw := range items {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := item["type"].(string)
		if itemType == "" {
			itemType = "message"
		}

		switch itemType {
		case "message":
			flushPending()
			role, _ := item["role"].(string)
			if role == "" {
				role = "user"
			}
			// Divergence: Responses uses a "developer" role; coerce to "system"
			// for upstreams that only understand system/user/assistant/tool.
			if role == "developer" {
				role = "system"
			}
			content := responsesMessageContentToText(item["content"])
			messages = append(messages, models.OpenAIMessage{Role: role, Content: content})

		case "function_call":
			callID, _ := item["call_id"].(string)
			if callID == "" {
				callID = genResponsesID("call")
			}
			name, _ := item["name"].(string)
			arguments, _ := item["arguments"].(string)
			pendingToolCalls = append(pendingToolCalls, models.OpenAIToolCall{
				ID:   callID,
				Type: "function",
				Function: models.OpenAIFunctionCall{
					Name:      name,
					Arguments: arguments,
				},
			})

		case "function_call_output":
			flushPending()
			callID, _ := item["call_id"].(string)
			messages = append(messages, models.OpenAIMessage{
				Role:       "tool",
				ToolCallID: callID,
				Content:    responsesOutputToString(item["output"]),
			})
		}
	}

	flushPending()
	return messages
}

// responsesMessageContentToText flattens a Responses message `content` (string
// or list of parts) into a single text string, joining input_text/text/
// output_text parts with newlines.
func responsesMessageContentToText(content interface{}) string {
	if content == nil {
		return ""
	}
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var parts []string
		for _, pRaw := range c {
			if s, ok := pRaw.(string); ok {
				parts = append(parts, s)
				continue
			}
			p, ok := pRaw.(map[string]interface{})
			if !ok {
				continue
			}
			ptype, _ := p["type"].(string)
			if t, ok := p["text"].(string); ok && (ptype == "input_text" || ptype == "text" || ptype == "output_text" || ptype == "") {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	return fmt.Sprintf("%v", content)
}

// responsesOutputToString normalizes a function_call_output `output` value to a
// string (tool message content). Strings pass through; structured output is
// JSON-encoded.
func responsesOutputToString(output interface{}) string {
	if output == nil {
		return ""
	}
	if s, ok := output.(string); ok {
		return s
	}
	b, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprintf("%v", output)
	}
	return string(b)
}

// convertResponsesTools converts Responses tools to Chat Completions tools.
// Handles flat `function` tools, already-nested function tools, and MCP
// `namespace` tools (flattened to mcp__<ns>__<name>). Other types are ignored.
func convertResponsesTools(tools []interface{}) []models.OpenAITool {
	var out []models.OpenAITool
	for _, tRaw := range tools {
		t, ok := tRaw.(map[string]interface{})
		if !ok {
			continue
		}
		ttype, _ := t["type"].(string)
		if ttype == "" {
			ttype = "function"
		}
		switch ttype {
		case "function":
			if fn, ok := t["function"].(map[string]interface{}); ok {
				out = append(out, models.OpenAITool{Type: "function", Function: buildOpenAIFunction(fn)})
			} else {
				out = append(out, models.OpenAITool{Type: "function", Function: buildOpenAIFunction(t)})
			}
		case "namespace":
			nsName, _ := t["name"].(string)
			if nsName == "" {
				nsName = "unknown"
			}
			nested, _ := t["tools"].([]interface{})
			for _, nRaw := range nested {
				n, ok := nRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if ntype, _ := n["type"].(string); ntype != "function" {
					continue
				}
				nName, _ := n["name"].(string)
				fn := map[string]interface{}{"name": fmt.Sprintf("mcp__%s__%s", nsName, nName)}
				if d, ok := n["description"]; ok {
					fn["description"] = d
				}
				if p, ok := n["parameters"]; ok {
					fn["parameters"] = p
				}
				out = append(out, models.OpenAITool{Type: "function", Function: buildOpenAIFunction(fn)})
			}
		}
	}
	return out
}

// buildOpenAIFunction reads name/description/parameters out of a loose map.
func buildOpenAIFunction(m map[string]interface{}) models.OpenAIFunction {
	fn := models.OpenAIFunction{}
	if name, ok := m["name"].(string); ok {
		fn.Name = name
	}
	if desc, ok := m["description"].(string); ok {
		fn.Description = desc
	}
	if p, ok := m["parameters"].(map[string]interface{}); ok {
		fn.Parameters = p
	}
	return fn
}

// statusFromFinishReason maps a Chat Completions finish_reason to a Responses
// response status. Responses has no stop_reason field; status carries the
// terminal state. Defaults to "completed".
func statusFromFinishReason(reason string) string {
	switch reason {
	case "length", "content_filter":
		return "incomplete"
	default:
		return "completed"
	}
}

// makeResponseObject builds the top-level Responses response object, echoing
// back select request fields (instructions/tools/tool_choice/temperature/top_p/
// max_output_tokens/parallel_tool_calls/reasoning/text) when present, matching
// real OpenAI behavior and keeping stricter clients happy.
func makeResponseObject(respID, model, status string, output []map[string]interface{}, reqBody map[string]interface{}) map[string]interface{} {
	if output == nil {
		output = []map[string]interface{}{}
	}
	obj := map[string]interface{}{
		"id":         respID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     status,
		"model":      model,
		"output":     output,
	}
	if reqBody != nil {
		for _, key := range []string{"instructions", "tools", "tool_choice", "temperature", "top_p", "max_output_tokens", "parallel_tool_calls", "reasoning", "text"} {
			if v, ok := reqBody[key]; ok && v != nil {
				obj[key] = v
			}
		}
	}
	return obj
}

// ConvertOpenAIResponseToResponses translates a non-streaming Chat Completions
// response into a Responses API response object. Mirrors translate_non_stream.
// ConvertOpenAIResponseToResponses guards the single-shot Responses conversion:
// a panic on malformed upstream data (nil choices, unexpected field shapes)
// degrades to an empty completed response instead of crashing the whole proxy
// process; the panic (with stack) is logged for post-mortem.
func ConvertOpenAIResponseToResponses(resp *models.OpenAIResponse, model string, reqBody map[string]interface{}) map[string]interface{} {
	return guardConversionPanic(nil, model, func() map[string]interface{} {
		// Minimal, safe fallback: never re-invoke the risky conversion helpers
		// (they may panic again). An empty completed response keeps the client
		// (codex) on its protocol instead of receiving a torn reply.
		return map[string]interface{}{
			"type":   "response",
			"id":     genResponsesID("resp"),
			"status": "completed",
			"output": []map[string]interface{}{{
				"type": "message", "id": genResponsesID("msg"), "status": "completed", "role": "assistant",
				"content": []map[string]interface{}{{"type": "output_text", "text": ""}},
			}},
			"usage": map[string]interface{}{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
		}
	}, func() map[string]interface{} {
		output := []map[string]interface{}{}
		status := "completed"
		usagePrompt, usageCompletion, usageTotal := 0, 0, 0

		if resp != nil {
			usagePrompt = resp.Usage.PromptTokens
			usageCompletion = resp.Usage.CompletionTokens
			usageTotal = resp.Usage.TotalTokens
			if len(resp.Choices) > 0 {
				choice := resp.Choices[0]
				msg := choice.Message
				// v1 ignores reasoning_content (astron-code is not a reasoning model;
				// half-baked reasoning items break codex's stream state machine).
				if contentStr := openAIContentToString(msg.Content); contentStr != "" {
					output = append(output, map[string]interface{}{
						"type":   "message",
						"id":     genResponsesID("msg"),
						"status": "completed",
						"role":   "assistant",
						"content": []map[string]interface{}{
							{"type": "output_text", "text": contentStr},
						},
					})
				}
				for _, tc := range msg.ToolCalls {
					output = append(output, map[string]interface{}{
						"type":      "function_call",
						"id":        genResponsesID("fc"),
						"call_id":   tc.ID,
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					})
				}
				status = statusFromFinishReason(choice.FinishReason)
			}
		}

		if len(output) == 0 {
			output = append(output, map[string]interface{}{
				"type":   "message",
				"id":     genResponsesID("msg"),
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]interface{}{
					{"type": "output_text", "text": ""},
				},
			})
		}

		result := makeResponseObject(genResponsesID("resp"), model, status, output, reqBody)
		result["usage"] = map[string]interface{}{
			"input_tokens":  usagePrompt,
			"output_tokens": usageCompletion,
			"total_tokens":  usageTotal,
		}
		return result
	})
}

// openAIContentToString flattens an OpenAI message content (string or part
// list) to text.
func openAIContentToString(content interface{}) string {
	if content == nil {
		return ""
	}
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var parts []string
		for _, pRaw := range c {
			if p, ok := pRaw.(map[string]interface{}); ok {
				if t, ok := p["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return fmt.Sprintf("%v", content)
}

// toIntFromAny coerces a JSON-decoded numeric (float64/int) to int.
func toIntFromAny(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

type responsesAccumToolCall struct {
	ID   string
	Name string
	Args strings.Builder
}

// ConvertOpenAIStreamingToResponses translates a Chat Completions SSE stream
// into a Responses API SSE stream. Mirrors iter_translate_stream, reusing the
// proxy's SSE infra (serialized writer). There is intentionally NO heartbeat:
// codex does not expect Anthropic ping events. Returns a StreamingResult for
// usage logging (nil on client disconnect / terminal error).
func ConvertOpenAIStreamingToResponses(c *gin.Context, reader io.Reader, model string, reqBody map[string]interface{}, stallTimeout time.Duration) *StreamingResult {
	logger := utils.GetLogger()
	streamStart := time.Now()
	ctx := c.Request.Context()

	if stallTimeout <= 0 {
		stallTimeout = 120 * time.Second
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("X-Accel-Buffering", "no")

	// Serialize all SSE writes (idempotent; safe even if nothing else writes).
	bindSSEWriter(c)

	respID := genResponsesID("resp")
	msgID := genResponsesID("msg")

	// sequence_number is local to THIS invocation (NOT package-global), so
	// concurrent requests get independent sequences.
	seq := 0
	emit := func(eventType string, extra map[string]interface{}) {
		if extra == nil {
			extra = map[string]interface{}{}
		}
		extra["type"] = eventType
		extra["sequence_number"] = seq
		seq++
		sendSSE(c, eventType, extra)
	}

	started := false
	msgOpen := false
	contentOpen := false
	var fullText strings.Builder
	var toolCalls []responsesAccumToolCall
	curTCIdx := -1
	usagePrompt, usageCompletion, usageTotal := 0, 0, 0
	finishReason := ""

	start := func() {
		if started {
			return
		}
		started = true
		emit("response.created", map[string]interface{}{
			"response": makeResponseObject(respID, model, "in_progress", nil, reqBody),
		})
		emit("response.in_progress", map[string]interface{}{
			"response": makeResponseObject(respID, model, "in_progress", nil, reqBody),
		})
	}

	openMsg := func() {
		if msgOpen {
			return
		}
		msgOpen = true
		emit("response.output_item.added", map[string]interface{}{
			"output_index": 0,
			"item": map[string]interface{}{
				"type":    "message",
				"id":      msgID,
				"status":  "in_progress",
				"role":    "assistant",
				"content": []interface{}{},
			},
		})
		emit("response.content_part.added", map[string]interface{}{
			"output_index":  0,
			"content_index": 0,
			"part":          map[string]interface{}{"type": "output_text", "text": ""},
		})
		contentOpen = true
	}

	closeMsg := func() {
		if !msgOpen {
			return
		}
		text := fullText.String()
		if contentOpen {
			emit("response.output_text.done", map[string]interface{}{
				"output_index":  0,
				"content_index": 0,
				"text":          text,
			})
			emit("response.content_part.done", map[string]interface{}{
				"output_index":  0,
				"content_index": 0,
				"part":          map[string]interface{}{"type": "output_text", "text": text},
			})
			contentOpen = false
		}
		emit("response.output_item.done", map[string]interface{}{
			"output_index": 0,
			"item": map[string]interface{}{
				"type":    "message",
				"id":      msgID,
				"status":  "completed",
				"role":    "assistant",
				"content": []map[string]interface{}{{"type": "output_text", "text": text}},
			},
		})
		msgOpen = false
	}

	done := make(chan struct{})
	errChan := make(chan error, 2)
	idleTimer := time.NewTimer(stallTimeout)
	defer idleTimer.Stop()

	// Cap on consecutive malformed chunks: an upstream sending garbage must not
	// degrade into a plausible-but-empty response.completed (codex would see a
	// silent "no output"). Above the cap we end the stream with an explicit
	// response.failed so the client's error path, not a fake success, wins.
	const maxMalformedChunks = 50
	malformed := 0

	// sawDone is set when the upstream explicitly closes the stream: a bare
	// [DONE] sentinel, or a chunk carrying a finish_reason. EOF without either
	// (e.g. a proxy killed mid-stream) means the stream was truncated, which we
	// MUST surface as an explicit failure — otherwise the client sees a
	// plausible-but-incomplete response.completed (status "completed" via
	// statusFromFinishReason("")) and never retries. Same class of silent
	// truncation behind codex's "idle timeout waiting for SSE".
	// Unlike streamFailed this is written by the scan goroutine only.
	sawDone := false

	// Set when the stream is ended with an explicit response.failed. The deferred
	// body then skips close(done), so the main select deterministically takes the
	// errChan path and does NOT also emit a plausible-but-empty response.completed
	// after the failure (codex would treat that as a silent no-output success).
	// Only the scanning goroutine touches this flag, read back in its own defer,
	// so a plain bool is race-free here.
	streamFailed := false

	// Read + translate upstream chunks. This goroutine is the only SSE writer
	// while running; the main goroutine emits terminal events only after
	// <-done (goroutine exited), so emit()/seq are race-free.
	go func() {
		// done is closed only when the stream ended normally (no panic, no
		// explicit response.failed). On a panic the error (with stack) goes to
		// errChan; on an explicit stream failure the caller already pushed to
		// errChan and set streamFailed. In both cases done stays open so the main
		// select takes the error path deterministically and never fabricates a
		// response.completed. recover() must be called directly in this deferred
		// frame (Go's recover semantics) before delegating.
		defer func() {
			r := recover() // must be called directly in this deferred frame
			if !guardStreamPanic(c, errChan, types.StageStreaming, model, r) && !streamFailed {
				close(done)
			}
		}()

		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			idleTimer.Reset(stallTimeout)
			select {
			case <-ctx.Done():
				errChan <- fmt.Errorf("client disconnected")
				return
			default:
			}
			if sseWriteClosed(c) {
				errChan <- fmt.Errorf("client disconnected")
				return
			}

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
				sawDone = true
				return
			}

			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(chunkData), &raw); err != nil {
				malformed++
				if malformed > maxMalformedChunks {
					logger.Warn("[responses-stream] too many malformed chunks (%d), failing stream", malformed)
					start()
					emit("response.failed", map[string]interface{}{
						"response": makeResponseObject(respID, model, "failed", []map[string]interface{}{
							{"type": "error", "message": fmt.Sprintf("upstream sent %d malformed chunks", malformed)},
						}, reqBody),
					})
					// Suppress the trailing response.completed: route the main select
					// to the error path (done stays open) instead of a fake success.
					streamFailed = true
					errChan <- fmt.Errorf("upstream sent %d malformed chunks", malformed)
					return
				}
				continue
			}

			// Capture usage BEFORE the empty-choices skip: upstreams that force
			// stream_options.include_usage send a final chunk with usage and
			// choices:[]. Skipping first would lose token accounting.
			if u, ok := raw["usage"].(map[string]interface{}); ok {
				usagePrompt = toIntFromAny(u["prompt_tokens"])
				usageCompletion = toIntFromAny(u["completion_tokens"])
				usageTotal = toIntFromAny(u["total_tokens"])
			}

			// Upstream-wrapped streaming error (no choices).
			if errVal, hasErr := raw["error"]; hasErr {
				if _, hasChoices := raw["choices"]; !hasChoices {
					start()
					errMsg := fmt.Sprintf("%v", errVal)
					logger.Warn("[responses-stream] upstream error chunk: %s", errMsg)
					emit("response.failed", map[string]interface{}{
						"response": makeResponseObject(respID, model, "failed", []map[string]interface{}{
							{"type": "error", "message": errMsg},
						}, reqBody),
					})
					// Suppress the trailing response.completed: route the main select
					// to the error path (done stays open) instead of a fake success.
					streamFailed = true
					errChan <- fmt.Errorf("upstream error: %s", errMsg)
					return
				}
			}

			var chunk models.OpenAIResponse
			if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
				malformed++
				if malformed > maxMalformedChunks {
					logger.Warn("[responses-stream] too many malformed chunks (%d), failing stream", malformed)
					start()
					emit("response.failed", map[string]interface{}{
						"response": makeResponseObject(respID, model, "failed", []map[string]interface{}{
							{"type": "error", "message": fmt.Sprintf("upstream sent %d malformed chunks", malformed)},
						}, reqBody),
					})
					// Suppress the trailing response.completed: route the main select
					// to the error path (done stays open) instead of a fake success.
					streamFailed = true
					errChan <- fmt.Errorf("upstream sent %d malformed chunks", malformed)
					return
				}
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}

			choice := &chunk.Choices[0]
			delta := choice.Delta
			start()

			if delta != nil {
				// Text content delta (reasoning_content is intentionally ignored in v1).
				if text := openAIContentToString(delta.Content); text != "" {
					openMsg()
					fullText.WriteString(text)
					emit("response.output_text.delta", map[string]interface{}{
						"output_index":  0,
						"content_index": 0,
						"delta":         text,
					})
				}

				// Tool call deltas: a new call carries id+name; subsequent
				// fragments carry argument shards that accumulate to curTCIdx.
				for _, tc := range delta.ToolCalls {
					// Debug: log incoming tool call delta
					logger.Debug("[responses-stream] tool_call delta: id=%q name=%q args_len=%d", tc.ID, tc.Function.Name, len(tc.Function.Arguments))
					if tc.ID != "" && tc.Function.Name != "" {
						toolCalls = append(toolCalls, responsesAccumToolCall{ID: tc.ID, Name: tc.Function.Name})
						curTCIdx = len(toolCalls) - 1
						logger.Debug("[responses-stream] new tool_call: idx=%d id=%q name=%q", curTCIdx, tc.ID, tc.Function.Name)
						if tc.Function.Arguments != "" {
							toolCalls[curTCIdx].Args.WriteString(tc.Function.Arguments)
						}
					} else if curTCIdx >= 0 && tc.Function.Arguments != "" {
						toolCalls[curTCIdx].Args.WriteString(tc.Function.Arguments)
					} else {
						// Log when tool call is ignored
						logger.Warn("[responses-stream] tool_call ignored: id=%q name=%q curTCIdx=%d args_len=%d", tc.ID, tc.Function.Name, curTCIdx, len(tc.Function.Arguments))
					}
				}
			}

			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
				sawDone = true
				// Do NOT return: OpenAI emits a trailing usage-only chunk
				// (choices:[]) AFTER the finish chunk. Returning here would
				// lose token accounting. Keep looping; usage is captured at
				// the top of each iteration, and the stream ends at [DONE]/EOF.
				continue
			}
		}

		if err := scanner.Err(); err != nil {
			// Scanner-level read error (e.g. connection reset, proxy torn down):
			// surface it as an explicit overloaded_error response.failed instead of
			// letting the deferred close(done) race the errChan branch — done would
			// route the main select to the normal path and emit a fake
			// response.completed the client would take as success.
			msg := fmt.Sprintf("upstream stream read error: %v", err)
			logger.Warn("[responses-stream] %s: %s", msg, client.ClassifyOpenAIError(msg))
			start()
			emit("response.failed", map[string]interface{}{
				"response": makeResponseObject(respID, model, "failed", []map[string]interface{}{
					{"type": "error", "code": "overloaded_error", "message": msg},
				}, reqBody),
			})
			streamFailed = true
			errChan <- fmt.Errorf("scanner error: %w", err)
			return
		}

		// Upstream truncated the stream WITHOUT a completion marker: no [DONE]
		// sentinel, no finish_reason chunk — just EOF. This is the signature of a
		// mid-stream disconnect (killed proxy, upstream crash, network drop), and
		// letting it through the normal path would emit a plausible-but-truncated
		// response.completed that the client treats as success and never retries.
		// Surface it as an explicit overloaded_error response.failed so clients
		// (codex/OpenAI SDK) retry. Only the scanning goroutine reads sawDone, and
		// only it writes streamFailed / errChan here (same pattern as the
		// malformed-chunk cap above), so no locks are needed.
		if !sawDone {
			cur := time.Since(streamStart)
			start()
			msg := fmt.Sprintf("upstream closed the stream before completion (truncated after %v)", cur)
			logger.Warn("[responses-stream] %s: %s", msg, client.ClassifyOpenAIError(msg))
			emit("response.failed", map[string]interface{}{
				"response": makeResponseObject(respID, model, "failed", []map[string]interface{}{
					{"type": "error", "code": "overloaded_error", "message": msg},
				}, reqBody),
			})
			// The main select's errChan branch classifies + emits the
			// interruption record once; do NOT duplicate it here.
			streamFailed = true
			errChan <- fmt.Errorf("truncated stream: %s", msg)
			return
		}
	}()

	select {
	case <-done:
		// Normal completion — emit terminal events below.
		logger.Info("[responses-stream] upstream completed normally after %v", time.Since(streamStart))
	case err := <-errChan:
		if strings.Contains(err.Error(), "client disconnected") {
			logger.Info("[responses-stream] ended: client disconnected after %v", time.Since(streamStart))
			EmitInterruption(c, types.CauseClientDisconnect, types.DimensionSubjective, types.StageStreaming, "client disconnected mid-stream", time.Since(streamStart).Milliseconds(), model)
			return nil
		}
		logger.Warn("[responses-stream] ended: %s after %v", client.ClassifyOpenAIError(err.Error()), time.Since(streamStart))
		cause, dimension := types.Classify(err, ctx)
		EmitInterruption(c, cause, dimension, types.StageStreaming, err.Error(), time.Since(streamStart).Milliseconds(), model)
		return nil
	case <-ctx.Done():
		logger.Info("[responses-stream] ended: client cancelled after %v", time.Since(streamStart))
		EmitInterruption(c, types.CauseClientDisconnect, types.DimensionSubjective, types.StageStreaming, "client cancelled (request context done)", time.Since(streamStart).Milliseconds(), model)
		return nil
	case <-idleTimer.C:
		logger.Warn("[responses-stream] ended: upstream stalled (no data for %v) after %v total", stallTimeout, time.Since(streamStart))
		EmitInterruption(c, types.CauseUpstreamStall, types.DimensionInfrastructure, types.StageStreaming, fmt.Sprintf("upstream stalled (no data for %v)", stallTimeout), time.Since(streamStart).Milliseconds(), model)
		return nil
	case <-time.After(streamMaxDuration):
		logger.Warn("[responses-stream] ended: exceeded max stream duration %v", streamMaxDuration)
		return nil
	}

	// --- Terminal events (only reached on normal <-done) ---
	closeMsg()

	baseIdx := 0
	if fullText.String() != "" {
		baseIdx = 1
	}
	resultToolCalls := []map[string]interface{}{}
	for i, tc := range toolCalls {
		fcID := genResponsesID("fc")
		oidx := baseIdx + i
		args := tc.Args.String()
		emit("response.output_item.added", map[string]interface{}{
			"output_index": oidx,
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        fcID,
				"call_id":   tc.ID,
				"name":      tc.Name,
				"arguments": "",
			},
		})
		emit("response.function_call_arguments.delta", map[string]interface{}{
			"output_index": oidx,
			"item_id":      fcID,
			"call_id":      tc.ID,
			"delta":        args,
		})
		emit("response.function_call_arguments.done", map[string]interface{}{
			"output_index": oidx,
			"item_id":      fcID,
			"call_id":      tc.ID,
			"arguments":    args,
		})
		emit("response.output_item.done", map[string]interface{}{
			"output_index": oidx,
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        fcID,
				"call_id":   tc.ID,
				"name":      tc.Name,
				"arguments": args,
			},
		})
		resultToolCalls = append(resultToolCalls, map[string]interface{}{
			"type": "function_call", "id": fcID, "call_id": tc.ID, "name": tc.Name, "arguments": args,
		})
	}

	// Assembled output for response.completed: message item is included when
	// there was text OR there are no tool calls (mirrors Python line 430).
	text := fullText.String()
	output := []map[string]interface{}{}
	if text != "" || len(toolCalls) == 0 {
		output = append(output, map[string]interface{}{
			"type":   "message",
			"id":     msgID,
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]interface{}{
				{"type": "output_text", "text": text},
			},
		})
	}
	for _, tc := range toolCalls {
		output = append(output, map[string]interface{}{
			"type":      "function_call",
			"id":        genResponsesID("fc"),
			"call_id":   tc.ID,
			"name":      tc.Name,
			"arguments": tc.Args.String(),
		})
	}

	final := makeResponseObject(respID, model, statusFromFinishReason(finishReason), output, reqBody)
	final["completed_at"] = time.Now().Unix()
	final["usage"] = map[string]interface{}{
		"input_tokens":  usagePrompt,
		"output_tokens": usageCompletion,
		"total_tokens":  usageTotal,
	}
	emit("response.completed", map[string]interface{}{"response": final})

	c.Writer.Flush()

	return &StreamingResult{
		Content:      text,
		InputTokens:  usagePrompt,
		OutputTokens: usageCompletion,
		StopReason:   statusFromFinishReason(finishReason),
		ToolCalls:    resultToolCalls,
	}
}
