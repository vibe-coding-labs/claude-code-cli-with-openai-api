# Deep Protocol Compatibility Phase 3 — litellm + one-api Additional Patterns

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 将 litellm 和 one-api 中第二轮深度分析发现的 5 个新协议模式移植到我们的 converter 层，进一步增强 Claude Code CLI 兼容性。

**Architecture:** Claude Code CLI → (Claude Messages API) → handler.go → converter/ 转换层。本次增强 5 个具体模式：redacted_thinking 流式处理、citation 内容块、system message cache_control 保留、compaction 流式 delta、Claude 4.6+ output_config 支持。

**Tech Stack:** Go 1.22, Gin Web Framework

**Risks:**
- Task 1 修改 streaming_state 添加新 block type，可能影响现有流式行为 → 缓解：只添加新枚举值和 case 分支，不改现有逻辑
- Task 3 修改 system message 解析逻辑 → 缓解：保持向后兼容，只提取 cache_control 字段
- Task 5 添加 output_config 字段 → 缓解：omitempty 确保 JSON 兼容

---

## Task 1: Add redacted_thinking Streaming Support

**Depends on:** None
**Files:**
- Modify: `converter/streaming_state.go:15-19`（ContentBlockType 枚举）
- Modify: `converter/streaming_state.go:143-180`（detectBlockType 函数）

- [ ] **Step 1: 添加 BlockRedactedThinking 到 ContentBlockType 枚举 — 支持 Claude 4.x redacted thinking blocks**

文件: `converter/streaming_state.go:15-19`

```go
const (
	BlockText            ContentBlockType = "text"
	BlockToolUse         ContentBlockType = "tool_use"
	BlockThinking        ContentBlockType = "thinking"
	BlockRedactedThinking ContentBlockType = "redacted_thinking"
)
```

- [ ] **Step 2: 修改 detectBlockType 以检测 redacted_thinking blocks — 从 OpenAI 特殊字段识别**

文件: `converter/streaming_state.go:143-180`（detectBlockType 函数中 ReasoningContent 检测之后添加）

在 `detectBlockType` 中，在 `if delta.ReasoningContent != ""` 检测之后，添加 redacted_thinking 检测：

```go
// Redacted thinking (Claude 4.x sends opaque thinking blocks)
if delta.ReasoningContent == "" && len(delta.ToolCalls) == 0 {
	if rc, ok := delta.Content.(string); ok && strings.HasPrefix(rc, "─redacted─") {
		return BlockRedactedThinking, map[string]interface{}{
			"type": "redacted_thinking",
			"data": "",
		}
	}
}
```

注意：这个检测基于最佳努力。真正的 redacted_thinking 在 OpenAI 侧没有直接对应，这个 case 主要是作为安全网。实际 redacted_thinking 通过非流式路径处理（claude.go 已有 `case "redacted_thinking"` 处理）。

- [ ] **Step 3: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0
  - No error output

---

## Task 2: Add Citation Content Block Support

**Depends on:** None
**Files:**
- Modify: `models/claude.go:38-50`（ClaudeContentBlock 添加 Citations 字段）
- Modify: `converter/claude.go:160-168`（内容块解析保留 citations）

- [ ] **Step 1: 添加 Citations 字段到 ClaudeContentBlock — 支持 Anthropic citation 功能**

文件: `models/claude.go:38-50`

在 `ClaudeContentBlock` 结构体中添加 `Citations` 字段：

```go
type ClaudeContentBlock struct {
	Type        string                 `json:"type"`
	Text        string                 `json:"text,omitempty"`
	Thinking    string                 `json:"thinking,omitempty"`
	Source      interface{}            `json:"source,omitempty"`
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	ToolUseID   string                 `json:"tool_use_id,omitempty"`
	Content     interface{}            `json:"content,omitempty"`
	IsError     bool                   `json:"is_error,omitempty"`
	CacheControl interface{}           `json:"cache_control,omitempty"`
	Citations   interface{}            `json:"citations,omitempty"`   // Anthropic citation support
}
```

- [ ] **Step 2: 在 claude.go 解析中保留 citations — 从 Claude 请求透传 citations**

文件: `converter/claude.go:162-168`（在 cache_control 保留之后添加）

在 claude.go 的 `// Preserve cache_control` 代码块之后，添加：

```go
// Preserve citations from text content blocks (litellm pattern)
if cits, ok := blockMap["citations"]; ok {
	cb.Citations = cits
}
```

注意：这需要将 `cb` 的类型从 `ContentBlock` 扩展以包含 Citations 字段。但 `ContentBlock` 是内部格式，Citations 只在 Claude→Claude 路径中有意义。更合理的做法是在内部 ContentBlock 中也加 Citations：

文件: `converter/internal.go:41-53`（ContentBlock struct）

添加 `Citations` 字段：

```go
type ContentBlock struct {
	Type        string                 `json:"type"`
	Text        string                 `json:"text,omitempty"`
	Source      *ImageSource           `json:"source,omitempty"`
	VideoSource *VideoSource           `json:"video_source,omitempty"`
	AudioSource *AudioSource           `json:"audio_source,omitempty"`
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	ToolUseID   string                 `json:"tool_use_id,omitempty"`
	Content     string                 `json:"content,omitempty"`
	CacheControl interface{}           `json:"cache_control,omitempty"`
	Citations   interface{}            `json:"citations,omitempty"`
}
```

- [ ] **Step 3: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0
  - No error output

---

## Task 3: Preserve System Message Cache Control

**Depends on:** Task 2
**Files:**
- Modify: `converter/claude.go:52-67`（system message 解析）
- Modify: `models/claude.go:5-21`（ClaudeMessagesRequest 的 System 字段类型保持 interface{}）

- [ ] **Step 1: 扩展 system message 解析以保留 cache_control — litellm 在 system messages 上传递 cache_control**

文件: `converter/claude.go:52-67`

当前代码只提取 text 字段并 join。litellm 的模式是在 system message blocks 中保留 `cache_control` 标记。修改 system 解析逻辑，提取 text 的同时保留 cache_control 信息到 metadata：

```go
// 解析 system — 支持 cache_control (litellm pattern)
if claudeReq.System != nil {
	switch v := claudeReq.System.(type) {
	case string:
		req.System = v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
				// litellm pattern: track cache_control markers on system messages
				if cc, ok := m["cache_control"]; ok {
					if req.Metadata == nil {
						req.Metadata = make(map[string]interface{})
					}
					req.Metadata["system_cache_control"] = cc
				}
			}
		}
		req.System = strings.Join(parts, "\n")
	}
}
```

- [ ] **Step 2: 验证编译和测试通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server . && go test ./converter/... -count=1 -timeout 60s`
Expected:
  - Exit code: 0
  - Output contains: "ok"

---

## Task 4: Add Compaction and server_tool_use Streaming Delta Support

**Depends on:** Task 1
**Files:**
- Modify: `converter/streaming_state.go:15-19`（添加 BlockCompaction 枚举）
- Modify: `converter/response_converter.go:314-345`（添加 compaction delta 处理）

- [ ] **Step 1: 添加 BlockCompaction 到 ContentBlockType 枚举**

文件: `converter/streaming_state.go:15-19`

```go
const (
	BlockText             ContentBlockType = "text"
	BlockToolUse          ContentBlockType = "tool_use"
	BlockThinking         ContentBlockType = "thinking"
	BlockRedactedThinking ContentBlockType = "redacted_thinking"
	BlockCompaction       ContentBlockType = "compaction"
)
```

- [ ] **Step 2: 验证编译和测试通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server . && go test ./converter/... -count=1 -timeout 60s`
Expected:
  - Exit code: 0
  - Output contains: "ok"

---

## Task 5: Add Claude 4.6+ output_config Support and Update Documentation

**Depends on:** Task 1, Task 2, Task 3, Task 4
**Files:**
- Modify: `models/claude.go:5-21`（ClaudeMessagesRequest 添加 OutputConfig 字段）
- Modify: `docs/litellm-protocol-analysis.md`

- [ ] **Step 1: 添加 OutputConfig 字段到 ClaudeMessagesRequest — 支持 Claude 4.6+ 的 effort 参数**

文件: `models/claude.go:5-21`

```go
type ClaudeMessagesRequest struct {
	Model                  string                 `json:"model"`
	Messages               []ClaudeMessage        `json:"messages"`
	System                 interface{}            `json:"system,omitempty"`
	MaxTokens              int                    `json:"max_tokens"`
	Temperature            float64                `json:"temperature,omitempty"`
	TopP                   *float64               `json:"top_p,omitempty"`
	TopK                   *int                   `json:"top_k,omitempty"`
	Stream                 bool                   `json:"stream,omitempty"`
	StopSequences          []string               `json:"stop_sequences,omitempty"`
	Metadata               *ClaudeMetadata        `json:"metadata,omitempty"`
	Tools                  []ClaudeTool           `json:"tools,omitempty"`
	ToolChoice             map[string]interface{} `json:"tool_choice,omitempty"`
	DisableParallelToolUse *bool                  `json:"disable_parallel_tool_use,omitempty"`
	ContextManagement      interface{}            `json:"context_management,omitempty"`
	Thinking               *ClaudeThinking        `json:"thinking,omitempty"`
	OutputConfig           interface{}            `json:"output_config,omitempty"` // Claude 4.6+ effort control
}
```

- [ ] **Step 2: 更新 litellm 协议分析文档 — 添加 Phase 3 新发现**

在 `docs/litellm-protocol-analysis.md` 末尾追加 Phase 3 分析结果。

- [ ] **Step 3: 运行完整测试验证所有改动**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o ccr-server . && go test ./converter/... -count=1 -timeout 120s`
Expected:
  - Exit code: 0
  - Output contains: "ok"
  - No "--- FAIL" lines
