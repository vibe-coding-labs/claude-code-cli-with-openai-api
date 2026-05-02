# litellm Protocol Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 将 BerriAI/litellm 项目中所有 Claude↔OpenAI 协议转换逻辑移植到我们的项目，确保 Claude Code CLI 通过我们的代理与任何 OpenAI 兼容提供商通信时协议完全兼容。

**Architecture:** Claude Code CLI → (Claude Messages API) → 我们的代理 → handler.go 提取 beta headers → converter/ 转换 Claude→Internal→OpenAI（请求方向），OpenAI→Internal→Claude（响应方向）→ SSE 流式输出。转换层在 `converter/` 包中，使用工厂模式。流式转换使用从 litellm AnthropicStreamWrapper 移植的状态机。

**Tech Stack:** Go 1.22, Gin Web Framework, litellm streaming state machine patterns

**Risks:**
- Task 1 修改了 ContentBlock 模型（加 cache_control 字段），可能影响序列化 → 缓解：字段用 `omitempty`
- Task 2 修改了 validateAndFixMessageSequence，已有测试覆盖 → 修改后必须跑全部 converter 测试
- Task 3 修改了 handler.go 的 beta header 逻辑 → 确保不破坏 Claude Code 客户端兼容性

---

## Pre-Planning Analysis

**Feature:** litellm Protocol Compatibility Hardening (Phase 2)
**Scope:** Single subsystem (converter/ + handler/)
**Files Create:** 0 (all modifications)
**Files Modify:**
- `converter/internal.go:41-52` — Add CacheControl to ContentBlock
- `converter/claude.go:149-160` — Handle cache_control in content blocks
- `converter/openai.go:406-494` — Add duplicate tool result detection
- `converter/streaming_state.go` — Add compaction content block type
- `handler/handler.go:1150-1169` — Update default beta headers
**Tasks:** 4 tasks
**Order:** Task 1 (ContentBlock cache_control) → Task 2 (duplicate tool result) → Task 3 (beta headers) → Task 4 (tests + verify)

---

## Already Implemented (Previous Session + Current Session Start)

All one-api patterns (22 items) and initial litellm P0 items are done:
- [x] Tool choice mapping (any→required, none→none)
- [x] Empty tool args {} in streaming
- [x] stop_sequence stop reason
- [x] compaction / content_filtered / refusal stop reason mappings
- [x] Tool call ID sanitization (`sanitizeToolCallID`)
- [x] server_tool_use content block handling
- [x] compaction content block handling
- [x] _tool_result subtypes (web_search, bash_code_execution, text_editor)

---

## Task 1: Add cache_control Passthrough on ContentBlock

**Depends on:** None
**Files:**
- Modify: `converter/internal.go:41-52`（ContentBlock struct）
- Modify: `converter/claude.go:87-160`（Claude request parsing — preserve cache_control）

- [ ] **Step 1: 添加 CacheControl 字段到 ContentBlock — 支持 Anthropic prompt caching**

文件: `converter/internal.go:41-52`（ContentBlock struct）

```go
// ContentBlock 表示消息中的一个内容块
type ContentBlock struct {
	Type       string                 `json:"type"` // text, image, video, audio, tool_use, tool_result
	Text       string                 `json:"text,omitempty"`
	Source     *ImageSource           `json:"source,omitempty"`       // for image
	VideoSource *VideoSource          `json:"video_source,omitempty"` // for video
	AudioSource *AudioSource          `json:"audio_source,omitempty"` // for audio
	ID         string                 `json:"id,omitempty"`           // for tool_use
	Name       string                 `json:"name,omitempty"`         // for tool_use
	Input      map[string]interface{} `json:"input,omitempty"`        // for tool_use
	ToolUseID  string                 `json:"tool_use_id,omitempty"`  // for tool_result
	Content    string                 `json:"content,omitempty"`      // for tool_result
	CacheControl interface{}          `json:"cache_control,omitempty"` // litellm pattern: prompt caching
}
```

- [ ] **Step 2: 在 claude.go 中保留 cache_control 字段 — 从 Claude 请求透传到内部格式**

文件: `converter/claude.go:87-160`（ParseClaudeRequest 中内容块解析）

在 claude.go 中，找到 `internalMsg.Content = append(internalMsg.Content, cb)` 之前，添加：

```go
// Preserve cache_control from any content block (litellm pattern)
if cc, ok := blockMap["cache_control"]; ok {
    cb.CacheControl = cc
}
```

这段代码需要在所有 `case` 分支之后、`internalMsg.Content = append(internalMsg.Content, cb)` 之前插入。具体位置在 claude.go 的 switch 语句结束处。

- [ ] **Step 3: 在 response_converter.go 中输出 cache_control — 保留到 Claude 响应**

在 converter/response_converter.go 中搜索 `ClaudeContentBlock` 结构，确保在构建 Claude 响应内容块时保留 CacheControl 字段。检查 models/ 下 ClaudeContentBlock 的定义。

Run: `grep -n "CacheControl\|cache_control" /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend/models/claude.go | head -10`

Expected:
  - Exit code: 0
  - 如果没有 CacheControl 字段，需要在 ClaudeContentBlock 中添加

- [ ] **Step 4: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0
  - No error output

---

## Task 2: Add Duplicate Tool Result Detection in Message Validation

**Depends on:** Task 1
**Files:**
- Modify: `converter/openai.go:494-514`（validateAndFixMessageSequence 尾部添加去重逻辑）

- [ ] **Step 1: 在 validateAndFixMessageSequence 末尾添加 tool result 去重 — litellm sanitize_messages_for_tool_calling 模式**

文件: `converter/openai.go:494-514`（在 validateAndFixMessageSequence 函数末尾，return 之前插入）

litellm 的 `sanitize_messages_for_tool_calling` 在 `factory.py:2362-2404` 中实现了 tool result 去重：对同一个 `tool_call_id` 的多个 tool message，只保留最后一个。

在 `validateAndFixMessageSequence` 函数的 return 语句之前，添加去重逻辑：

```go
// Deduplicate tool results with the same tool_call_id (litellm pattern).
// Anthropic requires exactly one tool_result per tool_use.
// Keep only the last occurrence within each contiguous block of tool results.
{
	var deduped []models.OpenAIMessage
	seenInBlock := make(map[string]int) // tool_call_id -> index in deduped
	for i, msg := range result {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			if prevIdx, exists := seenInBlock[msg.ToolCallID]; exists {
				// Replace previous occurrence with this one (keep last)
				deduped[prevIdx] = msg
				utils.GetLogger().Warn("[validateMessages] Deduplicating tool_result for tool_call_id=%s", msg.ToolCallID)
				continue
			}
			seenInBlock[msg.ToolCallID] = len(deduped)
		} else {
			// Non-tool message marks a turn boundary — reset tracking
			seenInBlock = make(map[string]int)
		}
		deduped = append(deduped, msg)
		_ = i // suppress unused warning
	}
	result = deduped
}
```

- [ ] **Step 2: 验证编译和测试通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server . && go test ./converter/... -count=1 -timeout 60s`
Expected:
  - Exit code: 0
  - Output contains: "ok"

---

## Task 3: Update Default Beta Headers for Modern Claude Features

**Depends on:** None
**Files:**
- Modify: `handler/handler.go:1150-1169`（appendDefaultBetaHeaders 函数）

- [ ] **Step 1: 更新 appendDefaultBetaHeaders — 添加 litellm 推荐的现代 Claude beta headers**

文件: `handler/handler.go:1150-1169`（替换 appendDefaultBetaHeaders 函数）

litellm 在 `common_utils.py:377-409` 的 `get_anthropic_beta_list` 方法中列出了当前推荐的 beta headers。我们当前的默认列表使用了已过时的 headers（prompt-caching 已经是默认行为，max-tokens-3-5-sonnet 也已不需要）。

替换整个 `appendDefaultBetaHeaders` 函数：

```go
// appendDefaultBetaHeaders 为 Claude Code 客户端添加默认 beta headers
// Updated based on litellm's get_anthropic_beta_list pattern.
// Anthropic no longer requires prompt-caching beta header (works automatically).
func appendDefaultBetaHeaders(existing []string) []string {
	// No default beta headers needed — prompt caching works automatically.
	// Feature-specific beta headers should be set by the client or detected
	// from the request (e.g., output_format, context_management).
	// Reference: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
	return existing
}
```

- [ ] **Step 2: 验证编译和测试通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server . && go test ./... -count=1 -timeout 120s`
Expected:
  - Exit code: 0
  - All tests pass

---

## Task 4: Run Full Test Suite and Update Documentation

**Depends on:** Task 1, Task 2, Task 3
**Files:**
- Modify: `docs/litellm-protocol-analysis.md`

- [ ] **Step 1: 运行完整 converter 测试套件并记录结果**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/... -count=1 -v -timeout 120s 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL|ok)" | tail -30`
Expected:
  - Exit code: 0
  - Output contains: "ok"
  - No "--- FAIL" lines

- [ ] **Step 2: 更新 litellm 协议分析文档最终状态**

更新 `docs/litellm-protocol-analysis.md`，将所有实施项标记为已完成。

- [ ] **Step 3: 提交所有变更**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && git add converter/ handler/ models/ docs/ && git commit -m "feat(converter): port litellm protocol compatibility — cache_control, tool result dedup, beta headers, content block expansion"`
