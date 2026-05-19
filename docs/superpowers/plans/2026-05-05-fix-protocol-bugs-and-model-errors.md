# Fix Protocol Conversion Bugs and "Unknown Aliased Model" Errors

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** Fix the intermittent "unknown aliased model" 400 errors that propagate raw API errors to Claude Code, and fix multiple protocol conversion bugs in streaming tool call handling and load balancer path.

**Architecture:** Two independent problem areas: (1) Upstream model errors need model-level fallback + graceful error wrapping so Claude Code receives proper Anthropic-format errors instead of raw OpenAI 400s. (2) Protocol converter bugs cause tool call failures — `emitEmptyToolArgsForBlock` checks wrong tool calls, load balancer path discards tool name mapping and beta headers.

**Tech Stack:** Go 1.23, Gin, SQLite, existing converter/handler architecture

**Risks:**
- Task 1 modifies the error handling path — must not break successful request flow
- Task 2 modifies streaming state machine — must maintain SSE event ordering
- Task 3 modifies load balancer code path — must verify with existing LB tests

---

## Pre-Planning Analysis

**Feature:** Protocol bug fixes + model error handling
**Scope:** Multiple subsystems (converter, handler, client)
**Files Create:** None
**Files Modify:**
- `backend/converter/response_converter.go:585-596` — emitEmptyToolArgsForBlock logic
- `backend/handler/handler.go:805` — LB path beta headers
- `backend/handler/handler.go:807` — LB path tool name mapping
- `backend/client/openai_client.go` — error wrapping for model errors
**Tasks:** 3 tasks
**Order:** Task 1 (error handling) → Task 2 (converter fix) → Task 3 (LB path fix)
**Risks:**
- Task 2 changes core streaming logic — must not break existing streaming tests
- Task 3 affects LB code path — must verify LB integration still works

---

### Task 1: Fix "Unknown Aliased Model" Error Propagation

**Depends on:** None
**Files:**
- Modify: `backend/client/openai_client.go` — error classification and wrapping
- Modify: `backend/handler/handler.go:574-578` — stream error response
- Modify: `backend/handler/handler.go:617-621` — non-stream error response

**Root Cause Analysis:**

The upstream API `api.bilezan.cn` intermittently returns HTTP 400 with `{"error":{"message":"unknown aliased model"}}` for the `glm-5.1` model. The proxy's retry mechanism (3 retries with backoff) sometimes exhausts all attempts. When all retries fail, the raw OpenAI-format error propagates to Claude Code, which expects Anthropic-format errors. This causes Claude Code to show a confusing `API Error: 400 {"error":{"message":"..."}}` instead of a clean error message.

**Fix Strategy:** Wrap upstream model errors into Anthropic-format API errors so Claude Code can display them properly. The error classification already exists in `isRetryableModelRoutingError` — we just need to ensure the final error response uses Anthropic format.

- [ ] **Step 1: Wrap model routing errors as Anthropic-format errors in SendErrorResponse**

Read the `SendErrorResponse` method in `backend/handler/response_handler.go` to understand how errors are currently sent to clients.

Modify `SendErrorResponse` to detect model routing errors and wrap them in Anthropic error format:

```go
// In the error response handler, detect model routing errors and format them as Anthropic errors
// The key check: if the error message contains "unknown aliased model" or similar model routing errors,
// return an Anthropic-format error instead of passing through the raw OpenAI error
```

- [ ] **Step 2: Add Anthropic error wrapper for model errors in the streaming path**

In `backend/handler/handler.go`, the streaming error path (around line 574-578) currently calls `h.responseHandler.SendErrorResponse`. Verify that `SendErrorResponse` properly wraps the error.

If the client is streaming (SSE), the error must be sent as an SSE event, not a JSON response. Check the existing `sendSSEError` function in `backend/converter/response_converter.go` and ensure it formats errors in Anthropic style:

```
event: error
data: {"type":"error","error":{"type":"api_error","message":"Model 'glm-5.1' is temporarily unavailable. Please try again."}}
```

- [ ] **Step 3: Verify error propagation works end-to-end**

Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0
  - No compilation errors

- [ ] **Step 4: Commit**

Run: `git add backend/client/openai_client.go backend/handler/handler.go && git commit -m "fix(handler): wrap upstream model errors as Anthropic-format errors for Claude Code compatibility"`

---

### Task 2: Fix emitEmptyToolArgsForBlock Checking Wrong Tool Calls

**Depends on:** None
**Files:**
- Modify: `backend/converter/response_converter.go:585-596` — emitEmptyToolArgsForBlock

**Root Cause Analysis:**

The function `emitEmptyToolArgsForBlock` is supposed to emit `"{}"` for the **current** tool_use block if it never received arguments. But it iterates `state.toolCalls` (a map of ALL tool calls) and if **any** tool call has args, it returns without emitting. This is wrong when there are multiple parallel tool calls:

- Tool A (index 0): has args → `argsBuffer != ""`
- Tool B (index 1): no args → should emit `{}`

Current behavior: The function finds Tool A has args and returns, leaving Tool B without arguments. Claude Code then fails to parse Tool B's response.

- [ ] **Step 1: Fix emitEmptyToolArgsForBlock to check only the current block's tool call**

File: `backend/converter/response_converter.go:585-596`

Replace the entire `emitEmptyToolArgsForBlock` function:

```go
// emitEmptyToolArgsForBlock checks if the current tool_use block's tool call
// has received any arguments, and emits "{}" if not.
func emitEmptyToolArgsForBlock(c *gin.Context, state *StreamingState) {
	if state.currentBlockType != BlockToolUse {
		return
	}
	// Check only the tool call associated with the current content block.
	// The current block's tool call is tracked via the toolCalls map keyed by
	// the OpenAI streaming tool_call index. We need to find which tool call
	// corresponds to the current block by checking the currentBlockStart ID.
	blockID, _ := state.currentBlockStart["id"].(string)
	for _, tc := range state.toolCalls {
		if tc.id == blockID {
			// Found the tool call for this block — check if it has args
			if tc.argsBuffer != "" {
				return
			}
			emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, "{}")
			tc.argsBuffer = "{}"
			return
		}
	}
	// No tool call tracked yet for this block — emit {} as default
	emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, "{}")
}
```

- [ ] **Step 2: Run existing converter tests**

Run: `cd backend && go test ./converter/ -v -run "TestStreaming\|TestToolCall\|TestEmptyArgs" -count=1 2>&1 | head -50`
Expected:
  - Exit code: 0 (tests pass) or tests run but some may not exist (that's OK)
  - No compilation errors

- [ ] **Step 3: Build the full backend**

Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 4: Commit**

Run: `git add backend/converter/response_converter.go && git commit -m "fix(converter): emitEmptyToolArgsForBlock now checks only current tool call, not all"`

---

### Task 3: Fix Load Balancer Path — Beta Headers and Tool Name Mapping

**Depends on:** None
**Files:**
- Modify: `backend/handler/handler.go:805` — pass beta headers
- Modify: `backend/handler/handler.go:807` — use tool name mapping

**Root Cause Analysis:**

The `handleMessageWithConfigAndManager` function (load balancer code path) has two bugs:

1. **Beta headers not passed to converter** (line 805):
   ```go
   conversionResult2 := converter.ConvertClaudeToOpenAIWithConfigAndMapping(&req, targetConfig, nil)
   ```
   The third argument is `nil` instead of `betaHeaders`. The main code path correctly passes `betaHeaders`.

2. **Tool name mapping discarded** (line 807):
   ```go
   _ = conversionResult2.ToolNameMapping
   ```
   The tool name mapping is explicitly ignored with `_`. When tool names are truncated (longer than 64 chars), the response won't restore the original names, causing Claude Code to fail matching tool calls to tool definitions.

- [ ] **Step 1: Pass beta headers to converter in LB path**

File: `backend/handler/handler.go:805`

Replace:
```go
conversionResult2 := converter.ConvertClaudeToOpenAIWithConfigAndMapping(&req, targetConfig, nil)
```
With:
```go
conversionResult2 := converter.ConvertClaudeToOpenAIWithConfigAndMapping(&req, targetConfig, betaHeaders)
```

- [ ] **Step 2: Use tool name mapping in LB streaming path**

File: `backend/handler/handler.go:807-808`

Replace:
```go
_ = conversionResult2.ToolNameMapping
hasTools := len(req.Tools) > 0
```
With:
```go
toolNameMapping2 := conversionResult2.ToolNameMapping
hasTools := len(req.Tools) > 0
```

Then update the streaming call to pass the tool name mapping. Find the line where `HandleStreamingResponse` is called (around line 843):
```go
h.responseHandler.HandleStreamingResponse(c, targetClient, openAIReq, &req, configID, startTime, h.sessionHandler, sessionID)
```

Check if `HandleStreamingResponse` accepts a tool name mapping parameter. If not, we need to either:
- Add it as a parameter, or
- Pass it through context

The simplest approach: check `HandleStreamingResponse` signature and add the mapping parameter.

- [ ] **Step 3: Build and verify**

Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 4: Commit**

Run: `git add backend/handler/handler.go && git commit -m "fix(handler): pass beta headers and tool name mapping in load balancer code path"`
