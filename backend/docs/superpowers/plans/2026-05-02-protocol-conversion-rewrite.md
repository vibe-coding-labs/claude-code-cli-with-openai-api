# Protocol Conversion Rewrite Plan — 1:1 litellm Port

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** Based on deep analysis of litellm source code, rewrite the Go protocol converter to achieve 1:1 feature parity with litellm's Anthropic adapter, fixing all session interruption and tool call parsing failures.

**Architecture:** Claude Code SSE request → Handler → converter.ConvertClaudeToOpenAI (Claude→OpenAI request) → OpenAI API → converter.ConvertOpenAIStreamingToClaude (OpenAI→Claude SSE response). The streaming response converter uses a queue-based state machine (litellm's AnthropicStreamWrapper pattern) with holding mechanisms for proper event ordering.

**Tech Stack:** Go 1.22, Gin, Anthropic Messages API SSE protocol, litellm adapter architecture

**Risks:**
- Task 2 rewrites streaming_state.go which all streaming tests depend on → mitigate: run all tests after changes
- Task 3 modifies response_converter.go streaming flow → mitigate: comprehensive E2E test coverage
- Task 4 changes request conversion which affects all requests → mitigate: existing request tests

---

### Task 1: Add tool name mapping to streaming converter

**Depends on:** None
**Files:**
- Modify: `converter/streaming_state.go` (StreamingState struct, detectBlockType, shouldStartNewBlock)
- Modify: `converter/response_converter.go` (ConvertOpenAIStreamingToClaude signature)

litellm creates a `tool_name_mapping` when converting the REQUEST (truncating long tool names), then uses it in the RESPONSE to restore original names. Our code truncates but never restores.

- [ ] **Step 1: Add ToolNameMapping field to StreamingState**

文件: `converter/streaming_state.go:29-56` (修改 StreamingState struct)

```go
// StreamingState holds the state for streaming conversion.
type StreamingState struct {
	mu sync.Mutex

	messageID string
	model     string

	// Tool name mapping (truncated → original) for restoring names in response
	ToolNameMapping map[string]string

	// State machine flags (litellm pattern)
	sentFirstChunk         bool
	sentContentBlockStart  bool
	sentContentBlockFinish bool
	sentLastMessage        bool

	// Current content block tracking
	currentBlockType  ContentBlockType
	currentBlockIndex int
	currentBlockStart map[string]interface{}

	// Usage tracking
	usage           models.ClaudeUsage
	finalStopReason string

	// Track whether message_delta was already emitted
	emittedMessageDelta bool

	// Tool tracking for OpenAI's streaming tool call deltas
	toolCalls map[int]*toolCallInfo
}
```

- [ ] **Step 2: Update newStreamingState to accept tool name mapping**

文件: `converter/streaming_state.go:71-81` (替换 newStreamingState 函数)

```go
func newStreamingState(model string, toolNameMapping map[string]string) *StreamingState {
	return &StreamingState{
		messageID:        generateMessageID(),
		model:            model,
		ToolNameMapping:  toolNameMapping,
		currentBlockType: BlockText,
		currentBlockIndex: 0,
		currentBlockStart: map[string]interface{}{"type": "text", "text": ""},
		toolCalls:         make(map[int]*toolCallInfo),
	}
}
```

- [ ] **Step 3: Restore tool names in detectBlockType**

文件: `converter/streaming_state.go` (在 detectBlockType 中，tool_use 块创建后添加 name restoration)

In detectBlockType, after creating the tool_use block data, add:

```go
		// Restore original tool name if it was truncated (litellm pattern)
		toolName := tc.Function.Name
		if s.ToolNameMapping != nil {
			if original, ok := s.ToolNameMapping[toolName]; ok {
				toolName = original
			}
		}
		return BlockToolUse, map[string]interface{}{
			"type":  "tool_use",
			"id":    toolID,
			"name":  toolName,
			"input": map[string]interface{}{},
		}
```

- [ ] **Step 4: Restore tool names in shouldStartNewBlock parallel tool call path**

Same name restoration logic in the parallel tool call detection path.

- [ ] **Step 5: Update response_converter.go to pass tool name mapping**

文件: `converter/response_converter.go:182` (修改 ConvertOpenAIStreamingToClaude 函数)

Change function signature to accept tool name mapping and pass to newStreamingState.

- [ ] **Step 6: Update handler.go call site**

文件: `handler/handler.go:535` (更新 ConvertOpenAIStreamingToClaude 调用)

- [ ] **Step 7: Verify tests pass**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

---

### Task 2: Fix streaming state machine to match litellm's AnthropicStreamWrapper

**Depends on:** Task 1
**Files:**
- Modify: `converter/streaming_state.go` (shouldStartNewBlock, detectBlockType)
- Modify: `converter/response_converter.go` (streaming goroutine logic)

The core issue: our streaming state machine doesn't match litellm's queue-based approach. We emit events directly instead of queuing them, which can cause incorrect ordering (e.g., content_block_delta before content_block_start for tool arguments in transition chunks).

- [ ] **Step 1: Fix detectBlockType to match litellm exactly — detect tool_use whenever tool_calls exist with function**

Already done in previous session. Verify it's correct.

- [ ] **Step 2: Fix tool argument handling during block transitions**

In litellm, when a block transition happens AND the trigger chunk contains tool arguments, the arguments must be emitted as a separate content_block_delta AFTER the new content_block_start. Our code currently drops these arguments.

文件: `converter/response_converter.go` (在 shouldStartNewBlock 块中)

Add tool argument emission after content_block_start when the trigger chunk has arguments:

```go
				if shouldStart {
					emitContentBlockStop(c, state.currentBlockIndex)
					state.currentBlockIndex++
					state.currentBlockType = newType
					if blockStartData != nil {
						state.currentBlockStart = blockStartData
					}
					emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart)

					// litellm pattern: if trigger chunk has tool arguments, emit them
					// after content_block_start (some providers send args with name/ID)
					if state.currentBlockType == BlockToolUse && len(delta.ToolCalls) > 0 {
						tc := delta.ToolCalls[0]
						if tc.Function.Arguments != "" {
							idx := tc.Index
							if state.toolCalls[idx] == nil {
								state.toolCalls[idx] = &toolCallInfo{}
							}
							state.toolCalls[idx].argsBuffer += tc.Function.Arguments
							emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, tc.Function.Arguments)
						}
					}
				}
```

- [ ] **Step 3: Fix message_start to include cache token fields in initial usage**

Claude Code expects cache token fields in the initial message_start event to know that prompt caching is supported.

文件: `converter/response_converter.go` (emitMessageStart function)

Update usage to include cache_creation_input_tokens and cache_read_input_tokens initialized to 0:

```go
			"usage": map[string]int{
				"input_tokens":                0,
				"output_tokens":               0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
			},
```

- [ ] **Step 4: Verify tests pass**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

---

### Task 3: Fix request converter gaps (Claude → OpenAI)

**Depends on:** None (can run in parallel with Task 1)
**Files:**
- Modify: `converter/openai.go` (tool choice mapping, thinking conversion)
- Modify: `converter/request_converter.go` (tool name mapping extraction)

- [ ] **Step 1: Fix tool_choice mapping to match litellm**

litellm maps: `any` → `required`, `auto` → `auto`, `none` → `none`, `tool` → `function` with name.
Our code needs to handle the `tool` type properly.

文件: `converter/openai.go` (find tool_choice conversion code)

- [ ] **Step 2: Extract and propagate tool name mapping from request conversion**

When tools are truncated, create a mapping of truncated → original names. Pass this mapping through the handler to the streaming converter for response name restoration.

文件: `converter/request_converter.go` (add tool name mapping return value)

- [ ] **Step 3: Verify tests pass**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

---

### Task 4: Add comprehensive E2E streaming tests for tool call scenarios

**Depends on:** Task 2
**Files:**
- Modify: `converter/anthropic_streaming_e2e_test.go`

- [ ] **Step 1: Add test for tool call with arguments in first chunk**

Some providers (e.g., xAI, Gemini) include tool arguments in the same chunk as the function name/ID. Verify our converter handles this correctly.

- [ ] **Step 2: Add test for tool call with no ID in first chunk (only name)**

- [ ] **Step 3: Add test for parallel tool calls**

- [ ] **Step 4: Add test for tool name restoration**

- [ ] **Step 5: Verify all tests pass**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -v -run "TestStreamingE2E" -count=1 2>&1 | grep "PASS\|FAIL" | head -30`
Expected:
  - Exit code: 0
  - All tests show "PASS"

---

### Task 5: Build, restart, and smoke test

**Depends on:** Task 2, Task 3
**Files:** None (integration testing)

- [ ] **Step 1: Build binary**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0

- [ ] **Step 2: Restart server**
Run: `kill $(pgrep -f 'ccr-server server') 2>/dev/null; sleep 1 && cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && nohup ./ccr-server server --port 54988 > /tmp/ccr-server.log 2>&1 &`

- [ ] **Step 3: Run all tests**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./... -count=1 2>&1 | tail -10`
Expected:
  - Exit code: 0
