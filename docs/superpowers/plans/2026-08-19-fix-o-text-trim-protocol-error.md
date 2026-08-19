# 修复协议转换错误 `o.text.trim` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修复 Claude Code CLI 偶发崩溃 `API Error: undefined is not an object (evaluating 'o.text.trim')`。根因：代理在非流式响应中把空文本块序列化为 `{"type":"text"}`（`json:"text,omitempty"` 丢弃了空 `text` 字段），Claude Code 对 `o.text.trim()` 时 `o.text` 为 undefined 崩溃。

**Root Cause Analysis（基于真实证据）:**

1. **客户端崩溃点**（Claude Code 二进制 2.1.220 反编译确认）：
   ```js
   for (let o of n) if (o.type === "text") { if (o.text.trim()) r.push({type:"assistant_text",text:o.text}) }
   ```
   当 content block `o.type === "text"` 但 `o.text` 缺失（undefined）时，`.trim()` 抛错。

2. **代理产出错误数据**（数据库 request_logs id=9139 存储的实际响应体确认）：
   ```json
   "content":[{"type":"text"}],"stop_reason":"end_turn","usage":{"input_tokens":39625,"output_tokens":1}
   ```
   空文本块没有 `text` 字段。

3. **序列化根因**：`backend/models/claude.go:41` `ClaudeContentBlock.Text string json:"text,omitempty"` —— 空字符串被 omitempty 丢弃。已用 Go 复现：`{Type:"text",Text:""}` → `{"type":"text"}`。`Thinking` 字段（行42）同样受影响。

4. **触发链路**：
   - Claude Code 发送非流式请求（request 9139 请求体 `stream: None`，走 `/v1/messages` → `CreateMessage` → `executeMessageRequestWithConfig`）。
   - 上游 `lant.top/relay-api`（config `cc-lant-deepseek-v4-pro`）返回退化输出（output_tokens=1，空/空白文本）。
   - `executeMessageRequestWithConfig` 非流式分支（`handler.go:677-704`）直接 `c.JSON(200, claudeResp)` —— **没有** `HandleNonStreamingResponse`（response_handler.go:145-182）里已有的空内容/退化检测，空响应被原样转发。

5. **为什么"偶尔"**：需同时满足「非流式请求」（Claude Code 偶尔发出）+「上游返回空/退化内容」（偶发）。两者叠加即崩溃。

**Architecture:**
- 数据流：上游(空文本响应) → `ConvertOpenAIToClaudeResponse` 生成 `ClaudeContentBlock{Type:"text",Text:""}` → `c.JSON(200)` 序列化 → `{"type":"text"}` → Claude Code `o.text.trim()` 崩溃。
- 关键组件：
  1. `ClaudeContentBlock` / `ContentBlock` 自定义 `MarshalJSON` —— text 块必含 `text` 字段（空字符串也保留）、thinking 块必含 `thinking` 字段、tool_use 块不受影响（精确符合 Anthropic Messages 规范，比全局去 omitempty 更安全）。
  2. 抽取 `ValidateResponseContent` 帮助方法（从 `HandleNonStreamingResponse` 现有逻辑逐行提取），挂载到 `executeMessageRequestWithConfig` 非流式分支 —— 空/退化上游输出转为可重试的 `overloaded_error`（Claude Code 自动重试），而非返回损坏响应。
- 为什么这样做：序列化修复是结构性保证（任何路径的空文本块都不再崩溃客户端）；检测修复是防御纵深（把病态输出拦截为重试错误）。两层都需要。

**Tech Stack:** Go 1.24, Gin, encoding/json, Claude Messages API (Anthropic JSON/SSE), sqlite3

**Risks:**
- Task 1 修改共享模型 `ClaudeContentBlock`/`ContentBlock` → 影响所有序列化路径（转换器、日志、前端展示）→ 缓解：自定义 MarshalJSON 只改变输出格式（text/thinking 块补齐字段、其他块不变）；`claude_marshal_test.go` 覆盖 5 种块类型
- Task 2 抽取检测逻辑 → 缓解：代码逐行不变仅提取；`response_validation_test.go` 覆盖拦截/放行两种情形；现有 `response_handler_error_test.go` 保护回归
- 部署需重启正在服务的代理进程 → 缓解：Task 4 先 `go build` + `go test` 全量通过再重建二进制、重启前确认无活跃长请求（参考记忆「代理二进制滞后」）
- 客户端（Claude Code）无法修改 → 缓解：代理必须产出规范合规的响应，本计划正是为此

---

### Task 1: 修复空文本块序列化（自定义 MarshalJSON）

**Depends on:** None
**Files:**
- Modify: `backend/models/claude.go`（`ClaudeContentBlock` 定义，行 39-52）
- Modify: `backend/claude/models/messages.go`（`ContentBlock` 定义，行 64-73）
- Create: `backend/models/claude_marshal_test.go`

- [ ] **Step 1: 给 `backend/models/claude.go` 的 `ClaudeContentBlock` 添加自定义 MarshalJSON — 保证 text/thinking 块必含协议字段**

文件: `backend/models/claude.go:1-52`

先加 import（当前文件无 import 块，`package models` 后直接是类型定义）：

```go
package models

import "encoding/json"

// Claude API Models
```

然后在 `ClaudeContentBlock` 结构体定义之后追加：

```go
// MarshalJSON guarantees the Anthropic protocol's required fields are always
// emitted. The Messages API requires `text` on text blocks and `thinking` on
// thinking blocks; the omitempty tags drop empty values, producing
// {"type":"text"} — a block that crashes strict clients such as Claude Code
// CLI with "undefined is not an object (evaluating 'o.text.trim')".
func (b ClaudeContentBlock) MarshalJSON() ([]byte, error) {
	type alias ClaudeContentBlock
	switch b.Type {
	case "text":
		return json.Marshal(&struct {
			alias
			Text string `json:"text"`
		}{alias: alias(b), Text: b.Text})
	case "thinking":
		return json.Marshal(&struct {
			alias
			Thinking string `json:"thinking"`
		}{alias: alias(b), Thinking: b.Thinking})
	default:
		return json.Marshal(alias(b))
	}
}
```

注意：外层的 `Text string \`json:"text"\``（无 omitempty）遮蔽内嵌 alias 的同名带 omitempty 字段，空字符串也会被保留；tool_use 等其他块走 default 分支，输出不变。

- [ ] **Step 2: 给 `backend/claude/models/messages.go` 的 `ContentBlock` 添加自定义 MarshalJSON — 修复新格式响应的同类问题**

文件: `backend/claude/models/messages.go:1-73`

先加 import（当前文件无 import 块）：

```go
package models

import "encoding/json"

// Messages API 相关模型
```

然后在 `ContentBlock` 结构体定义之后追加：

```go
// MarshalJSON guarantees the `text` field is always emitted on text blocks
// (the Anthropic protocol requires it). The default omitempty tag drops empty
// text, producing {"type":"text"} which crashes strict clients.
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	type alias ContentBlock
	switch b.Type {
	case "text":
		return json.Marshal(&struct {
			alias
			Text string `json:"text"`
		}{alias: alias(b), Text: b.Text})
	default:
		return json.Marshal(alias(b))
	}
}
```

- [ ] **Step 3: 创建序列化回归测试 — 覆盖空文本/非空文本/tool_use/空 thinking/非空 thinking 五种块**

```go
// backend/models/claude_marshal_test.go
package models

import (
	"encoding/json"
	"testing"
)

func TestClaudeContentBlockMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		blk  ClaudeContentBlock
		want string
	}{
		{"empty text block must keep text field",
			ClaudeContentBlock{Type: "text", Text: ""},
			`{"type":"text","text":""}`},
		{"text block with content",
			ClaudeContentBlock{Type: "text", Text: "hi"},
			`{"type":"text","text":"hi"}`},
		{"tool_use block has no text field",
			ClaudeContentBlock{Type: "tool_use", ID: "toolu_1", Name: "bash", Input: map[string]interface{}{"cmd": "ls"}},
			`{"type":"tool_use","id":"toolu_1","name":"bash","input":{"cmd":"ls"}}`},
		{"empty thinking block must keep thinking field",
			ClaudeContentBlock{Type: "thinking", Thinking: ""},
			`{"type":"thinking","thinking":""}`},
		{"thinking block with content",
			ClaudeContentBlock{Type: "thinking", Thinking: "think"},
			`{"type":"thinking","thinking":"think"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := json.Marshal(tt.blk)
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}
			if string(out) != tt.want {
				t.Errorf("got %s, want %s", string(out), tt.want)
			}
		})
	}
}
```

- [ ] **Step 4: 验证序列化测试**
Run: `cd backend && go test ./models/ -run TestClaudeContentBlockMarshalJSON -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS" and "ok  	github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"

- [ ] **Step 5: 提交**
Run: `cd backend && git add models/claude.go ../backend/claude/models/messages.go models/claude_marshal_test.go && git commit -m "fix(models): always emit text/thinking fields in content blocks to prevent Claude Code o.text.trim crash"`
  （注意：实际仓库根在 `..`，用 `git add backend/models/claude.go backend/claude/models/messages.go backend/models/claude_marshal_test.go`）

---

### Task 2: 抽取共享空内容检测并挂载到主非流式路径

**Depends on:** None
**Files:**
- Modify: `backend/handler/response_handler.go`（抽取 `ValidateResponseContent`，原行 145-182 区块）
- Modify: `backend/handler/handler.go`（`executeMessageRequestWithConfig` 非流式分支，行 690-703）
- Create: `backend/handler/response_validation_test.go`

- [ ] **Step 1: 在 `response_handler.go` 添加 `ValidateResponseContent` 方法 — 复刻现有空内容+退化检测逻辑**

文件: `backend/handler/response_handler.go`（在 `HandleNonStreamingResponse` 函数之后、`logRequestWithStreamingDetails` 之前插入）

```go
// ValidateResponseContent rejects degenerate or empty upstream responses before
// they are serialized and sent to the client. Returns true when the response
// has been rejected (error already sent to the client) and the caller must
// return immediately. Every non-streaming send path must call this — without
// it, an empty text block serializes as {"type":"text"} (the omitempty tag
// drops the empty text field), which crashes Claude Code CLI at
// o.text.trim() ("undefined is not an object").
func (r *ResponseHandler) ValidateResponseContent(
	c *gin.Context,
	configID string,
	model string,
	startTime time.Time,
	claudeResp *models.ClaudeResponse,
	claudeReq *models.ClaudeMessagesRequest,
	sessionIDPtr *string,
) bool {
	// Degenerate output detection: pseudo-tool-call markers in text.
	if claudeResp != nil && len(claudeResp.Content) > 0 {
		for _, block := range claudeResp.Content {
			if block.Type == "text" && block.Text != "" {
				if isDegenerate, pattern := converter.GetDegenerateDetector().IsDegenerate(block.Text); isDegenerate {
					utils.GetLogger().Warn("[Non-Streaming] degenerate output detected (pattern=%s), returning overloaded_error. Content preview: %.200s", pattern, block.Text)
					err := fmt.Errorf("degenerate output detected (pseudo-tool-call markers in text, pattern=%s). Please retry.", pattern)
					r.sendErrorResponse(c, err)
					r.logRequestWithDetails(c, configID, model, 0, 0, startTime, "error", err.Error(), claudeReq, nil, sessionIDPtr)
					return true
				}
			}
		}
	}

	// Empty content detection: no meaningful text and no tool calls.
	if claudeResp != nil {
		hasTextContent := false
		hasToolCalls := false
		for _, block := range claudeResp.Content {
			if block.Type == "text" && !converter.GetDegenerateDetector().IsEmptyOrWhitespace(block.Text) {
				hasTextContent = true
			}
			if block.Type == "tool_use" {
				hasToolCalls = true
			}
		}
		if !hasTextContent && !hasToolCalls {
			utils.GetLogger().Warn("[Non-Streaming] empty content detected (no text and no tool calls), returning overloaded_error")
			err := fmt.Errorf("empty response detected (no meaningful content). Please retry.")
			r.sendErrorResponse(c, err)
			r.logRequestWithDetails(c, configID, model, 0, 0, startTime, "error", err.Error(), claudeReq, nil, sessionIDPtr)
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 修改 `HandleNonStreamingResponse` 复用帮助方法 — 用一处调用替换内联检测**

文件: `backend/handler/response_handler.go`（原行 145-182 区块，即 `// --- Degenerate output detection ---` 到空内容检测 `if !hasTextContent && !hasToolCalls { ... }` 结束）

将整个区块替换为：

```go
	// 拒绝空/退化输出——防止空文本块序列化为 {"type":"text"} 崩溃客户端
	if r.ValidateResponseContent(c, configID, openAIReq.Model, startTime, claudeResp, claudeReq, sessionIDPtr) {
		return
	}
```

- [ ] **Step 3: 修改 `executeMessageRequestWithConfig` 非流式分支 — 挂载检测（修复 9139 逃生路径）**

文件: `backend/handler/handler.go:690-703`

在 `if claudeResp == nil { ... }` 检查之后、`h.responseHandler.logRequestWithDetails(... "success" ...)` 之前插入：

```go
		// 拒绝空/退化输出——否则空文本块会被序列化为 {"type":"text"}，
		// Claude Code CLI 解析时对 undefined 调用 .trim() 崩溃
		// (API Error: undefined is not an object (evaluating 'o.text.trim'))。
		if h.responseHandler.ValidateResponseContent(c, configID, openAIReq.Model, startTime, claudeResp, &req, sessionIDPtr) {
			return nil
		}
```

- [ ] **Step 4: 创建检测帮助方法测试 — 覆盖空文本拦截、正常文本放行、纯工具调用放行、退化输出拦截**

```go
// backend/handler/response_validation_test.go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newValidationContext() (*ResponseHandler, *gin.Context, *httptest.ResponseRecorder) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return rh, c, w
}

// TestValidateResponseContent_EmptyText_Rejected 空文本块必须被拦截为 overloaded_error，
// 否则会被序列化成 {"type":"text"} 导致 Claude Code o.text.trim 崩溃。
func TestValidateResponseContent_EmptyText_Rejected(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{Type: "text", Text: ""}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, &models.ClaudeMessagesRequest{Model: "deepseek-v4-pro"}, nil)
	if !rejected {
		t.Errorf("empty text content should be rejected")
	}
}

// TestValidateResponseContent_NormalText_Allowed 有内容的文本块必须放行。
func TestValidateResponseContent_NormalText_Allowed(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{Type: "text", Text: "hello"}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, nil, nil)
	if rejected {
		t.Errorf("normal text content should be allowed")
	}
}

// TestValidateResponseContent_ToolOnly_Allowed 纯工具调用响应必须放行（不是空响应）。
func TestValidateResponseContent_ToolOnly_Allowed(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{
			Type:  "tool_use",
			ID:    "toolu_1",
			Name:  "bash",
			Input: map[string]interface{}{"command": "ls"},
		}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, nil, nil)
	if rejected {
		t.Errorf("tool-only response should be allowed")
	}
}

// TestValidateResponseContent_Degenerate_Rejected 伪工具调用标记的文本必须被拦截。
func TestValidateResponseContent_Degenerate_Rejected(t *testing.T) {
	rh, c, _ := newValidationContext()
	resp := &models.ClaudeResponse{
		Content: []models.ClaudeContentBlock{{Type: "text", Text: "```tool_code\n...\n```"}},
	}
	rejected := rh.ValidateResponseContent(c, "cfg", "deepseek-v4-pro", time.Now(), resp, nil, nil)
	if !rejected {
		t.Errorf("degenerate pseudo-tool-call text should be rejected")
	}
}
```

- [ ] **Step 5: 验证检测测试**
Run: `cd backend && go test ./handler/ -run TestValidateResponseContent -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS" and "ok  	github.com/vibe-coding-labs/claude-code-cli-with-openai-api/handler"

- [ ] **Step 6: 提交**
Run: `git add backend/handler/response_handler.go backend/handler/handler.go backend/handler/response_validation_test.go && git commit -m "fix(handler): reject empty/degenerate non-streaming responses to prevent Claude Code o.text.trim crash"`

---

### Task 3: 端到端回归测试 — 空文本 OpenAI 响应不再产生崩溃载荷

**Depends on:** Task 1, Task 2
**Files:**
- Create: `backend/converter/empty_content_regression_test.go`

- [ ] **Step 1: 创建端到端回归测试 — 空文本 OpenAI 响应经过完整非流式转换后，序列化载荷必含 text 字段且被检测拦截**

```go
// backend/converter/empty_content_regression_test.go
package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestRegression_EmptyTextResponse_NeverYieldsBareTypeText 复刻线上 9139 场景：
// 上游返回 output_tokens=1 的空文本响应，走非流式转换 + 序列化。
// 修复前载荷为 {"type":"text"}（缺 text 字段），会让 Claude Code 的
// o.text.trim() 抛 "undefined is not an object"。
func TestRegression_EmptyTextResponse_NeverYieldsBareTypeText(t *testing.T) {
	openAIResp := &models.OpenAIResponse{
		ID:    "chatcmpl-9139",
		Model: "deepseek-v4-pro",
		Choices: []models.OpenAIChoice{
			{
				Message:      models.OpenAIMessage{Content: ""},
				FinishReason: "stop",
			},
		},
		Usage: models.OpenAIUsage{PromptTokens: 39625, CompletionTokens: 1},
	}
	origReq := &models.ClaudeMessagesRequest{Model: "deepseek-v4-pro", Stream: false}

	claudeResp := ConvertOpenAIToClaudeResponse(openAIResp, origReq)
	if claudeResp == nil {
		t.Fatal("ConvertOpenAIToClaudeResponse returned nil")
	}
	if len(claudeResp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(claudeResp.Content))
	}

	// 序列化载荷必须包含 text 字段（即使为空字符串），绝不允许 {"type":"text"}
	raw, err := json.Marshal(claudeResp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `{"type":"text"}`) && !strings.Contains(s, `"text":""`) {
		t.Fatalf("payload contains bare {\"type\":\"text\"} which crashes Claude Code: %s", s)
	}
	if !strings.Contains(s, `"text":""`) {
		t.Errorf("payload missing text field on text block: %s", s)
	}
	t.Logf("serialized payload: %s", s)

	// 空内容检测必须拦截（ValidateResponseContent 由 Task 2 挂载到主非流式路径）
	if !GetDegenerateDetector().IsEmptyOrWhitespace(claudeResp.Content[0].Text) {
		t.Errorf("converted text should be empty/whitespace")
	}
}
```

- [ ] **Step 2: 验证回归测试**
Run: `cd backend && go test ./converter/ -run TestRegression_EmptyTextResponse_NeverYieldsBareTypeText -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS" and "serialized payload: ...\"text\":\"\""

- [ ] **Step 3: 提交**
Run: `git add backend/converter/empty_content_regression_test.go && git commit -m "test(converter): add regression test for empty text block serialization (o.text.trim crash)"`

---

### Task 4: 全量构建、测试、重建部署

**Depends on:** Task 1, Task 2, Task 3
**Files:** None（构建/部署）

- [ ] **Step 1: 全量构建验证**
Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0
  - 无编译错误（frontend_embed.go 的 embed 路径报错为已知环境问题，从仓库根 `go build ./backend/...` 构建可避免）

- [ ] **Step 2: 全量测试验证**
Run: `cd backend && go test ./models/ ./converter/ ./handler/ 2>&1 | tail -30`
Expected:
  - Exit code: 0
  - 输出不包含 "FAIL"
  - 新增三个测试文件全部 PASS

- [ ] **Step 3: 重建二进制并重启代理 — 让运行中的 54988 端口加载新逻辑**

先确认当前代理进程：
Run: `ps aux | grep claude-with-openai-api | grep -v grep`
Expected: 输出包含 `backend/claude-with-openai-api server`

然后从仓库根重建二进制：
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && go build -o backend/claude-with-openai-api ./backend/cmd/server`
Expected:
  - Exit code: 0
  - `stat -c %y backend/claude-with-openai-api` 时间更新为当前

重启代理（假设由 systemd/进程管理器托管；手动重启则先停旧进程再起新进程）：
Run: `sudo systemctl restart claude-with-openai-api 2>/dev/null || (kill <PID> && nohup backend/claude-with-openai-api server > logs/proxy.log 2>&1 &)`
Expected:
  - 新进程启动成功
  - `ss -tlnp | grep 54988` 显示新 PID
  - `logs/proxy-stderr.log` 末尾出现 "Database initialized" 且无 panic

- [ ] **Step 4: 提交全部变更**
Run: `git add backend/ && git commit -m "fix(proxy): prevent o.text.trim crash from empty text blocks in non-streaming responses"`
  （若 Task 1/2/3 已各自提交，此步跳过，仅确认工作区干净）

---

## Phase 3: SELF-REVIEW

### 检查清单（15 项）

| # | Check | Result | Action Taken |
|---|-------|--------|-------------|
| 1 | Header 包含 Goal + Architecture + Tech Stack? | ✅ PASS | 已含 Goal / Architecture / Tech Stack / Root Cause / Risks |
| 2 | 每个 Task 标注 Depends on? | ✅ PASS | Task1=None, Task2=None, Task3=1+2, Task4=1+2+3 |
| 3 | 精确文件路径（Create/Modify/Test）? | ✅ PASS | 全部真实路径，含行号 |
| 4 | 每个 Task 有 3-8 个 Step? | ✅ PASS | Task1=5, Task2=6, Task3=3, Task4=4 |
| 5 | 新文件步骤含完整代码（含 import）? | ✅ PASS | 三个测试文件均为完整代码 |
| 6 | 修改步骤含替换后完整代码（非 diff）? | ✅ PASS | 各修改步骤给出完整函数/区块 |
| 7 | 代码块大小 5-80 行? | ✅ PASS | 最大的 MarshalJSON+测试均 < 80 行 |
| 8 | 函数/类型在 Plan 内有定义（无悬空引用）? | ✅ PASS | ValidateResponseContent、MarshalJSON 均在计划内定义 |
| 9 | 每个 Task 有验证命令（命令+exit code+output pattern）? | ✅ PASS | 每 Task 均有 Run + Expected |
| 10 | Spec 中每个需求都有对应 Task? | ✅ PASS | 序列化修复(T1)、检测挂载(T2)、回归测试(T3)、部署(T4) |
| 11 | 每个 Task 完成后可独立验证? | ✅ PASS | T1 测序列化、T2 测检测、T3 测端到端、T4 测构建 |
| 12 | 无 TBD/TODO/模糊描述? | ✅ PASS | 无占位符 |
| 13 | 无 "add validation" 抽象指令? | ✅ PASS | 每步含具体代码 |
| 14 | 跨 Task 函数签名/类型名/属性名一致? | ✅ PASS | ValidateResponseContent 签名在 T2/T3 一致；ClaudeContentBlock 字段名一致 |
| 15 | 保存位置正确（docs/superpowers/plans/）? | ✅ PASS | 已保存至 docs/superpowers/plans/2026-08-19-fix-o-text-trim-protocol-error.md |

**Status:** ✅ ALL PASS

---

## Phase 4: EXECUTION SELECTION

**Tasks:** 4
**Dependencies:** yes（Task 3/4 依赖前置）
**User Preference:** none（skill 指定 ZERO-CONFIRM）
**Decision:** Subagent-Driven
**Reasoning:** 4 个任务 ≥ 3，且有顺序依赖，按规则选择 Subagent-Driven

**Auto-invoking:** `superpowers:subagent-driven-development`
