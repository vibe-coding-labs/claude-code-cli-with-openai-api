# Fix Protocol Conversion Causing Claude Code Session Crashes

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修复协议转换中的严重 bug，导致 Claude Code 通过代理时频繁崩溃退出。

**Root Cause Analysis:**

Bug 1 (CRITICAL): Thinking block 发送时 index 与已启动的 text block 冲突
- `response_converter.go:221-228` 立即发送 `content_block_start(index=0, text)`
- `response_converter.go:300-320` thinking 到达时又发送 `content_block_start(index=0, thinking)`
- Claude Code 收到两个 index=0 的 start 事件，导致协议错误崩溃

Bug 2 (CRITICAL): Thinking block 结束后没有发送 `content_block_stop`
Bug 3 (HIGH): `bufio.NewScanner` 默认 64KB 缓冲区，大型 tool_call 参数可能超出导致流中断
Bug 4 (MEDIUM): `defer cancel()` 在 for 循环内导致 context 泄漏
Bug 5 (LOW): 缺少 `X-Accel-Buffering: no` header 导致反向代理缓冲 SSE

**Architecture:** SSE 事件序列修复 — 延迟发送 text block start 直到确认无 thinking 内容；增加 Scanner 缓冲区；修复 context 泄漏。

**Tech Stack:** Go 1.24, Gin, SSE protocol

**Risks:**
- Thinking 块修复改变了 SSE 事件发送顺序 — 必须严格匹配 Claude API SSE 规范
- Scanner 缓冲区从 64KB 增大到 1MB 会增加峰值内存使用

---

### Task 1: Fix Thinking Block Index Collision and Missing Stop Event

**Depends on:** None
**Files:**
- Modify: `converter/response_converter.go:182-370`

- [ ] **Step 1: 修改 ConvertOpenAIStreamingToClaude 延迟 text block start — 修复 thinking/text index 冲突**

文件: `converter/response_converter.go:183-236`

核心改动：
1. 不再立即发送 `content_block_start(text, index=0)` — 延迟到第一次收到实际内容时
2. 添加 `hasStartedTextBlock` 状态跟踪
3. Thinking 到达时使用 index 0，text 使用 index 1
4. Text 先到达时使用 index 0（无 thinking）
5. Thinking 结束时发送 `content_block_stop`

- [ ] **Step 2: 修改 delta 处理逻辑 — 确保在 thinking→text 转换时正确关闭 thinking block**

文件: `converter/response_converter.go:298-360`

确保：
- thinking 结束后发送 `content_block_stop`
- text block 使用正确的 index（thinking 后 index+1）
- 第一次 text delta 时发送 `content_block_start`

- [ ] **Step 3: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`

---

### Task 2: Increase Scanner Buffer Size for Large SSE Events

**Depends on:** None
**Files:**
- Modify: `converter/response_converter.go:239`

- [ ] **Step 1: 增大 Scanner 缓冲区到 1MB — 防止大型 tool_call 参数导致流中断**

文件: `converter/response_converter.go:239`

将 `bufio.NewScanner(reader)` 改为带自定义缓冲区的 scanner：
```go
scanner := bufio.NewScanner(reader)
scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max token size
```

- [ ] **Step 2: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`

---

### Task 3: Fix defer cancel() Context Leak in Retry Loop

**Depends on:** None
**Files:**
- Modify: `client/openai_client.go:533-536`

- [ ] **Step 1: 修复 defer cancel() 泄漏 — 改为在每次循环迭代结束时调用 cancel**

文件: `client/openai_client.go:533-536`

将 `defer cancel()` 改为在循环体内手动调用 `cancel()`。

- [ ] **Step 2: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`

---

### Task 4: Add Missing SSE Headers

**Depends on:** None
**Files:**
- Modify: `converter/response_converter.go:197-201`

- [ ] **Step 1: 添加 X-Accel-Buffering: no header — 防止反向代理缓冲 SSE**

文件: `converter/response_converter.go:197-201`

在 SSE header 设置中添加 `X-Accel-Buffering: no`。

- [ ] **Step 2: 验证编译通过并重新构建**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -o ccr-server .`

- [ ] **Step 3: 提交**
Run: `git add converter/response_converter.go client/openai_client.go && git commit -m "fix(converter): fix thinking block index collision and SSE protocol errors causing Claude Code crashes"`
