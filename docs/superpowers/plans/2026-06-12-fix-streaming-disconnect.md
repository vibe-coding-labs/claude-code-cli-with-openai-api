# Bug Fix: 流式响应中客户端断开后代理继续消耗资源

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Symptom:** 对话过程中 Claude 偶尔断掉。表现为客户端断开后，代理服务器不知道客户端已离开，继续从上游 API 读取数据、处理 chunk、尝试写入已关闭的连接（写操作静默失败），导致：
- 上游 token 配额被浪费（代理为不存在的客户端继续消耗 API 调用）
- goroutine 泄漏（streaming goroutine 和 heartbeat goroutine 持续运行直到 5 分钟超时）
- 下次请求可能复用半关闭的 keepalive 连接 → `InvalidHTTPResponse`

**Root Cause:** 三个逐级递进的问题：

1. **sendSSE 静默忽略写入错误（主因）** — `backend/converter/sse_utils.go:13-14` 中 `c.Writer.Write()` 和 `c.Writer.Flush()` 的返回值被完全忽略。当客户端断开时，写入返回 `broken pipe` 或 `connection reset` 错误，但这些错误从未被检测，流处理继续运行。

2. **Heartbeat goroutine 不检测连接状态（次因）** — `backend/converter/heartbeat.go:28-31` 中每 5 秒发送 ping，但不检查写入是否成功。客户端断开后心跳仍在发送，goroutine 永远不会自行停止（只在 ctx.Done() 时退出）。

3. **Streaming goroutine 无写入错误传播（加固缺失）** — `response_converter.go` 中 `scanner.Scan()` goroutine 只检测 ctx.Done() 和 scanner 错误，不检测 SSE 写入错误。写入错误的通道不存在，goroutine 一直处理上游数据直到自然结束。

**Impact:** 所有使用流式响应的用户受影响。高频对话场景（Claude Code 多轮工具调用）下每日可触发多次。浪费 API 配额、goroutine、文件描述符。

**Scope:** Small（2 个文件，约 30 行改动）
**Risk:** Low（纯防御性加固，不改变正常路径的行为）
**Risks:**
- Task 1 修改 sendSSE 签名，所有调用方需更新 → 缓解：sendSSE 已有 20+ 调用点，需逐一更新为 `if !sendSSE(...) { return }`
- Task 2 心跳停止时需确保 scanner goroutine 也能感知到 → 缓解：通过 ctx.Done() 传播

**Autonomy Level:** Full

---

## 设计决策

### 写入错误传播机制

使用**返回值传递**而非 panic/channel：
- `sendSSE` 从 `(void)` 改为返回 `bool`（写入成功/失败）
- 每个 `emitContentBlock*` 函数返回 `bool`，传播 `sendSSE` 的结果
- scanner goroutine 中检测到写入失败后设置 `writeErr` 并通过 `errChan` 退出
- heartbeat goroutine 检测到 ping 写入失败后自行停止（close(stopChan) 通知调用方）

### 为什么不用 `http.ResponseController`

Gin v1.10 的 `c.Writer.Flush()` 内部已调用 `http.ResponseController.Flush()`。改用 `c.Stream()` 需要重构 streaming 路径的整个数据结构（当前是事件驱动的 SSE 生成，不适合 `c.Stream()` 的 `io.Reader` 接口）。保持当前架构，只添加错误检测即可。

---

### Task 1: 添加 SSE 写入错误检测 — 断开时立即中断流处理

**Depends on:** None
**Files:**
- Modify: `backend/converter/sse_utils.go:11-27`（sendSSE 返回值 + sendSSEError 返回值）
- Modify: `backend/converter/response_converter.go:189-519`（所有 emit* 函数返回值 + scanner goroutine 中写入错误检测）
- Modify: `backend/converter/heartbeat.go:13-43`（心跳检测写入错误并自行停止）

- [ ] **Step 1: 修改 sendSSE 返回写入状态**

文件: `backend/converter/sse_utils.go:11-15`（替换 sendSSE 函数）

```go
// sendSSE sends a Server-Sent Event. Returns true if the write succeeded,
// false if the connection is broken (client disconnected).
func sendSSE(c *gin.Context, event string, data interface{}) bool {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false
	}
	_, writeErr := c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData))))
	if writeErr != nil {
		// Client disconnected — stop sending
		return false
	}
	flushErr := c.Writer.Flush()
	if flushErr != nil {
		// Flush failed (broken pipe / connection reset) — client gone
		return false
	}
	return true
}
```

文件: `backend/converter/sse_utils.go:18-27`（替换 sendSSEError 函数）

```go
// sendSSEError sends an error event via SSE. Returns false if the write failed.
func sendSSEError(c *gin.Context, errorType, message string) bool {
	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	}
	return sendSSE(c, "error", errorEvent)
}
```

- [ ] **Step 2: 修改所有 emit* 函数返回写入状态**

文件: `backend/converter/response_converter.go:523-640`（替换所有 emit* 函数，添加 bool 返回值）

替换 `emitMessageStart`（约 line 523-543）：

```go
func emitMessageStart(c *gin.Context, state *StreamingState) bool {
	model := state.model
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return sendSSE(c, models.EventMessageStart, map[string]interface{}{
		"type": models.EventMessageStart,
		"message": map[string]interface{}{
			"id":       state.generateMessageID(),
			"type":     "message",
			"role":     "assistant",
			"model":    model,
			"content":  []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         nil,
		},
	})
}
```

替换 `emitPing`（约 line 545-549）：

```go
func emitPing(c *gin.Context) bool {
	return sendSSE(c, models.EventPing, map[string]interface{}{
		"type": models.EventPing,
	})
}
```

替换 `emitMessageDelta`（约 line 585-609）：

```go
func emitMessageDelta(c *gin.Context, state *StreamingState) bool {
	state.mu.Lock()
	stopReason := state.finalStopReason
	usage := state.usage
	state.mu.Unlock()

	usageData := map[string]interface{}{
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 {
		usageData["cache_read_input_tokens"] = usage.CacheReadInputTokens
	}
	return sendSSE(c, models.EventMessageDelta, map[string]interface{}{
		"type": models.EventMessageDelta,
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usageData,
	})
}
```

替换 `emitContentBlockStart`（约 line 551-557）：

```go
func emitContentBlockStart(c *gin.Context, index int, blockData map[string]interface{}) bool {
	return sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
		"type":          models.EventContentBlockStart,
		"index":         index,
		"content_block": blockData,
	})
}
```

替换 `emitContentBlockStop`（约 line 559-564）：

```go
func emitContentBlockStop(c *gin.Context, index int) bool {
	return sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
		"type":  models.EventContentBlockStop,
		"index": index,
	})
}
```

替换 `emitContentBlockDelta`（约 line 566-583）：

```go
func emitContentBlockDelta(c *gin.Context, index int, deltaType string, text string) bool {
	delta := map[string]interface{}{
		"type": deltaType,
	}
	switch deltaType {
	case models.DeltaInputJSON:
		delta["partial_json"] = text
	case "thinking_delta":
		delta["thinking"] = text
	default:
		delta["text"] = text
	}
	return sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
		"type":  models.EventContentBlockDelta,
		"index": index,
		"delta": delta,
	})
}
```

替换 `emitEmptyToolArgsIfNeeded`（约 line 613-618）：

```go
func emitEmptyToolArgsIfNeeded(c *gin.Context, state *StreamingState) bool {
	return emitEmptyToolArgsForBlock(c, state)
}
```

替换 `emitEmptyToolArgsForBlock`（约 line 622-640）— 注意所有 `emitContentBlockDelta` 调用改为 `return emitContentBlockDelta(...)` 或检查返回值：

```go
func emitEmptyToolArgsForBlock(c *gin.Context, state *StreamingState) bool {
	if state.currentBlockType != BlockToolUse {
		return true // not a tool_use block, nothing to do
	}
	// Check all tool_calls for the current block: if none have accumulated args, emit "{}"
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, tc := range state.toolCalls {
		if tc == nil {
			continue
		}
		if tc.argsBufferLen == 0 {
			// This tool call never received arguments — emit "{}"
			tc.argsBuffer = "{}"
			tc.argsBufferLen = 2
			tc.needsClose = false
			if !emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, "{}") {
				return false
			}
			return true
		}
	}
	// All existing tool calls have arguments, but we still need to emit "{}"
	// for the block itself
	if !emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, "{}") {
		return false
	}
	return true
}
```

- [ ] **Step 3: 修改 scanner goroutine 传播写入错误**

文件: `backend/converter/response_converter.go:206-401`（修改主函数中 emit 调用点以检查返回值）

在函数开头添加 `writeErr` channel（替换 line 224 的 `errChan` 声明区域）：

文件: `backend/converter/response_converter.go:206-212`（替换 heartbeat 启动部分）

```go
	// Emit initial events (litellm: sent_first_chunk + sent_content_block_start)
	if !emitMessageStart(c, state) {
		return nil
	}
	if !emitPing(c) {
		return nil
	}

	// Start heartbeat to keep connection alive
	heartbeatStop := StartHeartbeat(c, ctx, 5*time.Second)
	defer StopHeartbeat(heartbeatStop)
```

文件: `backend/converter/response_converter.go:223-225`（替换 errChan 声明）

```go
	errChan := make(chan error, 1)
	writeErr := false // track if any SSE write failed (client disconnected)
```

在 goroutine 内所有 `emit*` 调用点添加写入检查。关键位置的修改：

在 line 304（第一个 emitContentBlockStart）：

```go
						if !emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart) {
							writeErr = true
							return
						}
```

在 line 311（emitEmptyToolArgsForBlock）：

```go
							if !emitEmptyToolArgsForBlock(c, state) {
								writeErr = true
								return
							}
```

在 line 312（emitContentBlockStop after block transition）：

```go
							if !emitContentBlockStop(c, state.currentBlockIndex) {
								writeErr = true
								return
							}
```

在 line 318（emitContentBlockStart for new block type）：

```go
							if !emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart) {
								writeErr = true
								return
							}
```

在 line 341（emitContentBlockDelta for tool args in transition）：

```go
											if !emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, tc.Function.Arguments) {
												writeErr = true
												return
											}
```

在 line 354（emitContentBlockDelta for text）：

```go
							if !emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaText, textContent) {
								writeErr = true
								return
							}
```

在 line 377（emitContentBlockDelta for tool args in tool_use block）：

```go
							if !emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, tc.Function.Arguments) {
								writeErr = true
								return
							}
```

在 line 382（emitContentBlockDelta for thinking）：

```go
						if !emitContentBlockDelta(c, state.currentBlockIndex, "thinking_delta", delta.ReasoningContent) {
							writeErr = true
							return
						}
```

在 line 393（emitEmptyToolArgsIfNeeded for finish_reason）：

```go
						if !emitEmptyToolArgsIfNeeded(c, state) {
							writeErr = true
							return
						}
```

在 line 395（emitContentBlockStop for finish_reason）：

```go
						if !emitContentBlockStop(c, state.currentBlockIndex) {
							writeErr = true
							return
						}
```

在 line 397（emitMessageDelta for finish_reason）：

```go
					if !emitMessageDelta(c, state) {
						writeErr = true
						return
					}
```

在 line 398（sent emittedMessageDelta）：

```go
					state.emittedMessageDelta = true
```

在 goroutine 结尾（在 scanner.Err() 检查之前，约 line 403 之前）：

```go
		// If writeErr was set, don't bother reporting scanner errors — the client is gone
		if writeErr {
			errChan <- fmt.Errorf("client disconnected during stream write")
			return
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("scanner error: %w", err)
		}
```

在函数的最终 emit 部分（约 line 441-482），添加写入检查：

文件: `backend/converter/response_converter.go:439-482`（替换 final emit 部分）

```go
	// If content_block_finish was never sent (stream ended without finish_reason),
	// close the current block
	if !state.sentContentBlockFinish {
		// one-api pattern: emit {} for tool_use blocks that never received arguments
		if !emitEmptyToolArgsIfNeeded(c, state) {
			return nil
		}
		if !emitContentBlockStop(c, state.currentBlockIndex) {
			return nil
		}
		state.sentContentBlockFinish = true
	}

	// If no stop reason was set, default to end_turn
	state.mu.Lock()
	if state.finalStopReason == "" {
		state.finalStopReason = models.StopEndTurn
	}
	finalStopReason := state.finalStopReason
	usage := state.usage
	toolCalls := make(map[int]*toolCallInfo)
	for k, v := range state.toolCalls {
		toolCalls[k] = v
	}
	state.mu.Unlock()

	// Emit final message_delta if not already sent (finish_reason path already emits it)
	if !state.emittedMessageDelta {
		usageData := map[string]interface{}{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		}
		if usage.CacheReadInputTokens > 0 {
			usageData["cache_read_input_tokens"] = usage.CacheReadInputTokens
		}
		if !sendSSE(c, models.EventMessageDelta, map[string]interface{}{
			"type": models.EventMessageDelta,
			"delta": map[string]interface{}{
				"stop_reason":   finalStopReason,
				"stop_sequence": nil,
			},
			"usage": usageData,
		}) {
			return nil
		}
	}

	// Emit message_stop (litellm: sent_last_message)
	state.sentLastMessage = true
	if !sendSSE(c, models.EventMessageStop, map[string]interface{}{
		"type": models.EventMessageStop,
	}) {
		return nil
	}
```

- [ ] **Step 4: 修改心跳检测写入错误并自行停止**

文件: `backend/converter/heartbeat.go:13-43`（替换整个 StartHeartbeat 函数）

```go
// StartHeartbeat sends periodic ping events to keep the connection alive.
// This prevents proxy/CDN timeouts during long-running operations.
// The heartbeat automatically stops when:
// 1. ctx is cancelled (client disconnected via context)
// 2. The stop channel is closed
// 3. A ping write fails (broken pipe / connection reset — client gone)
func StartHeartbeat(c *gin.Context, ctx context.Context, interval time.Duration) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-ticker.C:
				// Send ping event to keep connection alive.
				// If write fails, client has disconnected — stop immediately.
				if !sendSSE(c, models.EventPing, map[string]interface{}{
					"type": models.EventPing,
				}) {
					return
				}
			}
		}
	}()

	return stopChan
}
```

注意：删除手动 `c.Writer.Flush()` 调用 — `sendSSE` 内部已经调用 `Flush()`。

- [ ] **Step 5: 在 convert 主函数的 select 中添加 writeErr 处理**

文件: `backend/converter/response_converter.go:408-435`（在 select 中增加对 writeErr 的日志说明）

现有的 `case err := <-errChan` 已能处理 "client disconnected during stream write" 错误消息。由于 `sendSSEError` 也返回 bool，但在此情况下向已断开的客户端发送 SSE 错误是无效的，只需记录日志。

确认 line 413-414 处：

```go
		if strings.Contains(err.Error(), "client disconnected") {
			sendSSEError(c, "cancelled", "Request was cancelled by client")
```

此处 sendSSEError 的返回值不需要检查（向断开的连接发送错误本身就会失败，但这是尽力而为的清理动作）。

- [ ] **Step 6: 验证编译通过**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "cannot"

- [ ] **Step 7: 质量门禁 — 交付前多维检查**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./converter/... && go build ./...`
Expected:
  - Exit code: 0
  - 无遗留 debug 语句
  - 无未使用 import

- [ ] **Step 8: 提交**
Run: `git add backend/converter/sse_utils.go backend/converter/response_converter.go backend/converter/heartbeat.go && git commit -m "fix(converter): detect client disconnection in streaming to stop wasting upstream resources"`

---

### Task 2: 在 handler 层添加流处理断开检测 — 完善错误传播链

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/handler.go:601`（ConvertOpenAIStreamingToClaudeWithMapping 返回 nil 时的处理）

- [ ] **Step 1: 处理 ConvertOpenAIStreamingToClaudeWithMapping 返回 nil 的情况**

文件: `backend/handler/handler.go:599-602`（streamResult 赋值处）

```go
		// Upstream is responsive. stallResult.Reader replays first chunk + remaining data.
		defer reader.Close()
		logger.Info("  Stream verified, processing response (stall retries: %d)...", stallRetry)
		streamResult = converter.ConvertOpenAIStreamingToClaudeWithMapping(c, stallResult.Reader, &req, c.Request.Context(), toolNameMapping, stallTimeout)
		if streamResult == nil {
			// Client disconnected during streaming — ConvertOpenAIStreamingToClaudeWithMapping
			// returns nil when SSE writes fail (broken pipe / connection reset).
			// No error response needed — the client is already gone.
			logger.Info("  Streaming aborted: client disconnected mid-stream (write error detected)")
			return fmt.Errorf("client disconnected during streaming")
		}
		break
```

- [ ] **Step 2: 验证编译通过**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 3: 质量门禁**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./handler/... && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add backend/handler/handler.go && git commit -m "fix(handler): detect nil streaming result when client disconnects mid-stream"`

---

## Progress Checkpoint

**Total Tasks:** 2
**Execution Order:** Task 1 → Task 2（严格顺序，Task 2 依赖 Task 1 的 return nil 语义）
**Estimated Impact:** 修复后，当客户端断开时：
- streaming goroutine 会在第一次失败的 SSE 写入后立即退出（而非继续处理整个流）
- heartbeat goroutine 会在第一次失败的 ping 写入后立即退出
- 上游 API 连接会被 `defer reader.Close()` 正确关闭
- 不再浪费 token 配额处理无客户端的流
- `InvalidHTTPResponse` 的发生概率从"偶尔"降至"极低"

**Limitations（本次未覆盖）：**
- Gin 框架的 context 取消延迟 — Go 的 HTTP stack 在检测客户端断开时有固有延迟（TCP keepalive 探测间隔），这层问题在应用层无法完全解决
- HTTP/2 的 stream 级别断开 — 需要 gin v1.11+ 或手动实现 `http.ResponseController` 级别的错误检测，留待后续观察
