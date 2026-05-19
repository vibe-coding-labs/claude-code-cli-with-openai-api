# Streaming Tool Call Parsing Robustness Fix

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** Fix streaming converter to reliably produce parseable tool call SSE events for Claude Code, eliminating "The model's tool call could not be parsed" errors.

**Architecture:** OpenAI streaming chunks → StreamingState state machine (detectBlockType → shouldStartNewBlock) → SSE events. Root cause: `detectBlockType` only detects BlockToolUse when name OR ID is non-empty, but some providers send tool call arguments before name/ID. Additionally, lazy start can produce empty text blocks or miss tool_use detection entirely. Fix: align with litellm's eager text block start + robust tool_use detection.

**Tech Stack:** Go 1.22, Gin framework, Anthropic SSE protocol

**Risks:**
- Task 1 modifies detectBlockType which affects all block type detection → mitigate: keep existing behavior for text/thinking, only change tool_use detection
- Task 2 switches from lazy to eager start → mitigate: litellm uses eager start successfully with Claude Code

---

### Task 1: Fix detectBlockType to match litellm behavior

**Depends on:** None
**Files:**
- Modify: `converter/streaming_state.go:127-160` (detectBlockType function)

- [ ] **Step 1: Modify detectBlockType — detect tool_use whenever tool_calls exist (match litellm)**

文件: `converter/streaming_state.go:127-160` (替换整个 detectBlockType 方法)

```go
// detectBlockType determines what type of content block an OpenAI delta implies.
// Ported from litellm's _translate_streaming_openai_chunk_to_anthropic_content_block.
func (s *StreamingState) detectBlockType(delta *models.OpenAIMessage) (ContentBlockType, map[string]interface{}) {
	// Tool calls: detect whenever tool_calls array is non-empty (litellm pattern)
	// litellm checks: choice.delta.tool_calls is not None and len > 0 and function is not None
	if len(delta.ToolCalls) > 0 {
		tc := delta.ToolCalls[0]
		toolID := NormalizeToolCallID(tc.ID)
		if toolID == "" {
			toolID = "toolu_" + generateShortID()
		}
		return BlockToolUse, map[string]interface{}{
			"type":  "tool_use",
			"id":    toolID,
			"name":  tc.Function.Name,
			"input": map[string]interface{}{},
		}
	}

	// Reasoning/thinking content
	if delta.ReasoningContent != "" {
		return BlockThinking, map[string]interface{}{
			"type":     "thinking",
			"thinking": "",
		}
	}

	// Regular text content (only if non-empty)
	if delta.Content != nil {
		if textContent, ok := delta.Content.(string); ok && textContent != "" {
			return BlockText, map[string]interface{}{"type": "text", "text": ""}
		}
	}

	// Default: no change
	return s.currentBlockType, s.currentBlockStart
}
```

- [ ] **Step 2: Modify shouldStartNewBlock — only start new tool_use block when name is non-empty (litellm pattern)**

文件: `converter/streaming_state.go:91-123` (替换整个 shouldStartNewBlock 方法)

```go
// shouldStartNewBlock detects if the current OpenAI streaming chunk indicates
// a content block type change. Ported from litellm's _should_start_new_content_block.
func (s *StreamingState) shouldStartNewBlock(choice *models.OpenAIChoice) (bool, ContentBlockType, map[string]interface{}) {
	if choice == nil || choice.FinishReason != "" {
		return false, s.currentBlockType, nil
	}
	// No block transitions if we haven't started any block yet (lazy start)
	if !s.sentContentBlockStart {
		return false, s.currentBlockType, nil
	}

	delta := choice.Delta
	if delta == nil {
		return false, s.currentBlockType, nil
	}

	// Detect block type from raw chunk
	blockType, blockStart := s.detectBlockType(delta)

	// Check if type changed
	if blockType != s.currentBlockType {
		s.currentBlockType = blockType
		s.currentBlockStart = blockStart
		return true, blockType, blockStart
	}

	// For parallel tool calls: a new tool_use with a name means a new block (litellm pattern)
	if blockType == BlockToolUse && len(delta.ToolCalls) > 0 {
		for _, tc := range delta.ToolCalls {
			if tc.Function.Name != "" {
				// Re-detect with this specific tool call to get correct ID/name
				toolID := NormalizeToolCallID(tc.ID)
				if toolID == "" {
					toolID = "toolu_" + generateShortID()
				}
				blockStart = map[string]interface{}{
					"type":  "tool_use",
					"id":    toolID,
					"name":  tc.Function.Name,
					"input": map[string]interface{}{},
				}
				s.currentBlockType = blockType
				s.currentBlockStart = blockStart
				return true, blockType, blockStart
			}
		}
	}

	return false, s.currentBlockType, nil
}
```

- [ ] **Step 3: Verify tests still pass**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -v -count=1 2>&1 | tail -30`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

---

### Task 2: Switch from lazy start to eager text block start (match litellm)

**Depends on:** Task 1
**Files:**
- Modify: `converter/response_converter.go:193-290` (streaming main logic)
- Modify: `converter/streaming_state.go:71-81` (newStreamingState)

- [ ] **Step 1: Modify newStreamingState — eager text block is the initial block (litellm default)**

文件: `converter/streaming_state.go:71-81` (替换整个 newStreamingState 函数)

No code change needed — `newStreamingState` already initializes with `currentBlockType: BlockText` and `currentBlockStart: map[string]interface{}{"type": "text", "text": ""}`. This matches litellm's eager text block start.

- [ ] **Step 2: Modify ConvertOpenAIStreamingToClaude — switch to eager text block start after message_start (match litellm exactly)**

文件: `converter/response_converter.go:193-203` (替换 message_start 和 ping 之后的注释和代码)

```go
	// Emit initial events (litellm: sent_first_chunk + sent_content_block_start)
	emitMessageStart(c, state)
	emitPing(c)

	// Eager text block start (litellm pattern: always start text block before content)
	// This ensures we have an open block for tool call transitions
	emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart)
	state.sentContentBlockStart = true
```

- [ ] **Step 3: Modify the streaming goroutine — remove lazy start logic, simplify to litellm pattern**

文件: `converter/response_converter.go:261-290` (替换 hasContent 块中的 lazy start 和 block transition 逻辑)

```go
			// --- litellm state machine core ---
			hasContent := delta.Content != nil || delta.ReasoningContent != "" || len(delta.ToolCalls) > 0

			if hasContent {
				shouldStart, newType, blockStartData := state.shouldStartNewBlock(choice)
				if shouldStart {
					emitContentBlockStop(c, state.currentBlockIndex)
					state.currentBlockIndex++
					state.currentBlockType = newType
					if blockStartData != nil {
						state.currentBlockStart = blockStartData
					}
					emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart)
				}

				switch state.currentBlockType {
```

- [ ] **Step 4: Verify tests pass and build succeeds**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -count=1 2>&1 | tail -5 && go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0
  - Output contains: "PASS" and "ok"

- [ ] **Step 5: Restart backend server**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && kill $(pgrep -f 'ccr-server server') 2>/dev/null; sleep 1 && nohup ./ccr-server server --port 54988 > /tmp/ccr-server.log 2>&1 &`
Expected:
  - Backend starts and responds to health check
  - Log contains "Server starting"
