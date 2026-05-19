# Protocol Compatibility Enhancement Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 参考 JoyCodeProxy 的协议转换实现，修复和增强 Claude↔OpenAI 协议转换的兼容性问题，使其能正确处理 Mistral、Gemini 等严格上游供应商。

**Architecture:** Claude 请求 → ClaudeConverter.ParseRequest → InternalRequest → OpenAIConverter.BuildRequest → 上游 OpenAI API。改进点分布在三个关键节点：(1) tool call ID 空值处理，(2) tool_result content 提取鲁棒性，(3) 工具名截断，(4) 流式 tool call 参数完整发送，(5) 空响应兜底。

**Tech Stack:** Go 1.24, Gin, SQLite, crypto/sha256

**Risks:**
- Task 2 修改 BuildRequest 消息构建逻辑，可能影响正常流程 → 缓解：现有回归测试覆盖 + 新增单元测试
- Task 3 工具名截断是破坏性变更（上游看到的名称不同） → 缓解：仅当名称超 64 字符时截断，保持前缀可辨识
- Task 4 流式处理修改可能引入竞态 → 缓解：不修改 mutex 逻辑，仅修改数据内容

---

### Task 1: Tool Call ID 空值自动生成

**Depends on:** None
**Files:**
- Modify: `converter/openai.go:217-231` (convertOpenAIMessageToInternal 中 tool_calls 处理)
- Modify: `converter/openai.go:534-547` (convertInternalMessageToOpenAI 中 tool_use 处理)
- Test: `converter/openai_test.go`

JoyCodeProxy 在 `convertAssistantBlocks` 中对空 ID 自动生成 `"call_" + newID()`，我们的实现在收到上游 tool_use 块没有 ID 时会传空字符串，导致上游报错。

- [ ] **Step 1: 修改 convertOpenAIMessageToOpenAI — tool_use 块缺少 ID 时自动生成**

文件: `converter/openai.go:534-547`（convertInternalMessageToOpenAI 函数中 `case "tool_use":` 分支）

```go
// 替换 converter/openai.go 中 tool_use case 分支
case "tool_use":
	hasToolUse = true
	// Auto-generate ID if missing (JoyCodeProxy pattern)
	toolID := cb.ID
	if toolID == "" {
		toolID = "call_" + generateShortID()
	}
	args := ""
	if cb.Input != nil {
		argsBytes, _ := json.Marshal(cb.Input)
		args = string(argsBytes)
	}
	toolCalls = append(toolCalls, models.OpenAIToolCall{
		ID:   toolID,
		Type: "function",
		Function: models.OpenAIFunctionCall{
			Name:      cb.Name,
			Arguments: args,
		},
	})
```

- [ ] **Step 2: 修改 convertOpenAIMessageToInternal — 上游返回 tool_calls 时 ID 空值处理**

文件: `converter/openai.go:217-231`（convertOpenAIMessageToInternal 函数中 tool_calls 处理）

```go
// 替换 converter/openai.go:217-231 的 tool_calls 处理
if len(msg.ToolCalls) > 0 {
	for _, tc := range msg.ToolCalls {
		var input map[string]interface{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		}
		// Preserve original ID, auto-generate if missing
		toolID := tc.ID
		if toolID == "" {
			toolID = "toolu_" + generateShortID()
		}
		internalMsg.Content = append(internalMsg.Content, ContentBlock{
			Type:  "tool_use",
			ID:    toolID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
}
```

- [ ] **Step 3: 添加 generateShortID 工具函数 — 在 openai.go 文件末尾**

```go
// generateShortID generates a random 24-char hex string for tool call IDs
func generateShortID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}
```

同时在文件顶部 import 中添加 `"crypto/rand"` 和 `"encoding/hex"`。

- [ ] **Step 4: 验证**
Run: `go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0
  - No compilation errors

- [ ] **Step 5: 提交**
Run: `git add converter/openai.go && git commit -m "fix(converter): auto-generate tool call IDs when missing"`

---

### Task 2: Tool Result Content 提取鲁棒性增强

**Depends on:** Task 1
**Files:**
- Modify: `converter/openai.go:298-333` (BuildRequest 中 tool_result 处理)
- Modify: `converter/claude.go:125-155` (ParseRequest 中 tool_result content 提取)

JoyCodeProxy 的 `extractToolResultContent` 函数处理了 3 种格式：(1) 空/缺失，(2) 纯字符串，(3) 数组形式的 text 块。我们的 BuildRequest 中 tool_result → OpenAI tool 消息时，Content 字段可能为空或包含数组，需要确保总是输出 string。

- [ ] **Step 1: 修改 BuildRequest 中 tool_result 的 Content 处理 — 确保总是 string**

文件: `converter/openai.go:317-324`（BuildRequest 函数中 tool_result → tool message 转换）

```go
// 替换 converter/openai.go:317-324 的 tool message 构建逻辑
for _, tr := range toolResults {
	// Ensure content is always a string for OpenAI tool messages
	toolContent := tr.Content
	if toolContent == "" {
		toolContent = ""
	}
	toolMsg := models.OpenAIMessage{
		Role:       "tool",
		ToolCallID: tr.ToolUseID,
		Content:    toolContent,
	}
	openAIReq.Messages = append(openAIReq.Messages, toolMsg)
}
```

- [ ] **Step 2: 修改 convertOpenAIMessageToInternal 中 tool 消息的 Content 提取 — 支持数组格式**

文件: `converter/openai.go:233-246`（convertOpenAIMessageToInternal 函数中 tool_call_id 处理）

JoyCodeProxy 的 `extractToolResultContent` 能处理数组和嵌套 text 块。我们的实现只处理 string，需要增加数组支持。

```go
// 替换 converter/openai.go:233-246 的 tool_call_id 处理
if msg.ToolCallID != "" {
	content := ""
	if msg.Content != nil {
		switch c := msg.Content.(type) {
		case string:
			content = c
		case []interface{}:
			// Extract text from content block array (JoyCodeProxy pattern)
			var parts []string
			for _, item := range c {
				if str, ok := item.(string); ok {
					parts = append(parts, str)
				} else if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["text"].(string); ok {
						parts = append(parts, t)
					}
				}
			}
			content = strings.Join(parts, "")
		}
	}
	// tool 消息转换为 user 角色的 tool_result
	internalMsg.Role = "user"
	internalMsg.Content = []ContentBlock{
		{Type: "tool_result", ToolUseID: msg.ToolCallID, Content: content},
	}
}
```

- [ ] **Step 3: 验证**
Run: `go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add converter/openai.go && git commit -m "fix(converter): robust tool_result content extraction for array and nested formats"`

---

### Task 3: Tool Name Truncation（工具名超长截断）

**Depends on:** Task 2
**Files:**
- Modify: `converter/openai.go:339-348` (BuildRequest 中 tools 构建)

litellm 和 JoyCodeProxy 在转换工具名时，如果名称超过 64 字符，会截断并用 deterministic hash 替代后缀。Mistral 等供应商限制 function name 长度为 64 字符。

- [ ] **Step 1: 添加 truncateToolName 函数 — 在 openai.go 文件末尾**

```go
const maxToolNameLength = 64

// truncateToolName truncates tool names exceeding maxToolNameLength using
// a deterministic hash suffix to maintain uniqueness (litellm pattern).
func truncateToolName(name string) string {
	if len(name) <= maxToolNameLength {
		return name
	}
	hash := sha256.Sum256([]byte(name))
	hashSuffix := hex.EncodeToString(hash[:])[:8]
	prefix := name[:maxToolNameLength-9]
	return prefix + "_" + hashSuffix
}
```

同时在文件顶部 import 中添加 `"crypto/sha256"`。

- [ ] **Step 2: 修改 BuildRequest 中 tools 构建 — 应用名称截断**

文件: `converter/openai.go:339-348`（BuildRequest 函数中 tools 构建循环）

```go
// 替换 converter/openai.go:339-348 的 tools 构建
for _, tool := range req.Tools {
	toolName := truncateToolName(tool.Name)
	if toolName != tool.Name {
		utils.GetLogger().Debug("[BuildRequest] Tool name truncated: %q -> %q", tool.Name, toolName)
	}
	openAIReq.Tools = append(openAIReq.Tools, models.OpenAITool{
		Type: "function",
		Function: models.OpenAIFunction{
			Name:        toolName,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		},
	})
}
```

- [ ] **Step 3: 验证**
Run: `go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add converter/openai.go && git commit -m "fix(converter): truncate tool names exceeding 64 chars with deterministic hash"`

---

### Task 4: Streaming Tool Call Arguments 完整发送

**Depends on:** Task 3
**Files:**
- Modify: `converter/response_converter.go:522-543` (processToolCallDelta 函数)

当前 `processToolCallDelta` 只在 JSON 完整解析成功时发送 `partial_json` 事件，但 Claude CLI 期望的是增量式 JSON delta。如果上游一次性返回完整 arguments（如 Gemini），当前逻辑只在第一次解析成功时发送一次，后续增量被忽略。需要确保所有 arguments chunk 都以 `partial_json` 发送。

- [ ] **Step 1: 修改 processToolCallDelta — 始终发送 partial_json 而非仅完整 JSON 时**

文件: `converter/response_converter.go:522-543`（processToolCallDelta 函数中 arguments 处理部分）

```go
// 替换 converter/response_converter.go:522-543 的 arguments 处理逻辑
// Process function arguments (outside of lock)
if shouldProcessArgs {
	// Always send incremental partial_json delta (Claude expects streaming JSON)
	// Even if the JSON is incomplete, send what we have
	state.mu.Lock()
	state.currentToolCalls[tcIndex].JSONSent = true
	state.mu.Unlock()

	sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
		"type":  models.EventContentBlockDelta,
		"index": claudeIndex,
		"delta": map[string]interface{}{
			"type":         models.DeltaInputJSON,
			"partial_json": tcDelta.Function.Arguments,
		},
	})
}
```

注意：关键变化是 (1) 不再检查 `json.Unmarshal` 是否成功，(2) 发送当前 delta 的 Arguments 而非整个缓冲区，(3) 移除 `jsonSent` 判断逻辑。

- [ ] **Step 2: 同步修改 StreamingState 结构 — 移除不再需要的 JSONSent 字段**

文件: `converter/streaming_state.go`

```go
// 替换 converter/streaming_state.go 全部内容
package converter

// ToolCallState tracks the state of a single tool call during streaming
type ToolCallState struct {
	ID          string
	Name        string
	ArgsBuffer  string
	ClaudeIndex int
	Started     bool
}
```

注意：移除了 `JSONSent bool` 字段，因为现在始终发送 partial_json。

- [ ] **Step 3: 验证**
Run: `go build -tags dev -o ccr-server .`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add converter/response_converter.go converter/streaming_state.go && git commit -m "fix(converter): always send streaming tool call arguments as partial_json delta"`

---

### Task 5: Empty Response 兜底和 Converter 单元测试

**Depends on:** Task 4
**Files:**
- Modify: `converter/response_converter.go:59-67` (legacy 转换空 choices 处理)
- Create: `converter/compat_test.go`

JoyCodeProxy 在 `TranslateResponse` 中对空 choices 返回空文本块而非 nil。我们的 legacy 转换在空 choices 时返回 nil，可能导致 handler panic。

- [ ] **Step 1: 修改 legacyConvertOpenAIToClaude — 空 choices 返回空文本块**

文件: `converter/response_converter.go:59-67`

```go
// 替换 converter/response_converter.go:59-67 的空 choices 检查
func legacyConvertOpenAIToClaude(openAIResp *models.OpenAIResponse, originalReq *models.ClaudeMessagesRequest) *models.ClaudeResponse {
	if openAIResp == nil {
		return &models.ClaudeResponse{
			ID:      "msg_empty",
			Type:    "message",
			Role:    models.RoleAssistant,
			Model:   originalReq.Model,
			Content: []models.ClaudeContentBlock{{Type: "text", Text: ""}},
			StopReason: func() *string { s := "end_turn"; return &s }(),
			Usage:   models.ClaudeUsage{},
		}
	}

	if len(openAIResp.Choices) == 0 {
		return &models.ClaudeResponse{
			ID:      openAIResp.ID,
			Type:    "message",
			Role:    models.RoleAssistant,
			Model:   originalReq.Model,
			Content: []models.ClaudeContentBlock{{Type: "text", Text: ""}},
			StopReason: func() *string { s := "end_turn"; return &s }(),
			Usage: models.ClaudeUsage{
				InputTokens:  openAIResp.Usage.PromptTokens,
				OutputTokens: openAIResp.Usage.CompletionTokens,
			},
		}
	}
```

- [ ] **Step 2: 创建兼容性单元测试 — 覆盖所有新增逻辑**

```go
// converter/compat_test.go
package converter

import (
	"encoding/json"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// --- Task 1: Tool Call ID Auto-generation ---

func TestToolUseBlockWithoutID(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter()
	conv.SetConfig(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "", Name: "test_tool", Input: map[string]interface{}{"key": "value"}},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Should have system-generated tool call ID
	for _, msg := range openAIReq.Messages {
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.ID == "" {
					t.Error("Tool call ID should not be empty, expected auto-generated ID")
				}
				if tc.Function.Name != "test_tool" {
					t.Errorf("Tool name = %q, want %q", tc.Function.Name, "test_tool")
				}
			}
			return
		}
	}
	t.Error("No tool_calls found in output messages")
}

func TestToolUseBlockWithExistingID(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter()
	conv.SetConfig(cfg)

	req := &InternalRequest{
		Model: "test-model",
		Messages: []InternalMessage{
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: "toolu_abc123", Name: "my_tool", Input: nil},
				},
			},
		},
	}

	body, err := conv.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	var openAIReq models.OpenAIRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	for _, msg := range openAIReq.Messages {
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "toolu_abc123" {
					t.Errorf("Tool call ID = %q, want %q (should preserve original)", tc.ID, "toolu_abc123")
				}
			}
			return
		}
	}
}

// --- Task 3: Tool Name Truncation ---

func TestTruncateToolName_Short(t *testing.T) {
	name := "short_tool"
	if got := truncateToolName(name); got != name {
		t.Errorf("truncateToolName(%q) = %q, want %q", name, got, name)
	}
}

func TestTruncateToolName_Exactly64(t *testing.T) {
	name := ""
	for i := 0; i < 64; i++ {
		name += "x"
	}
	if got := truncateToolName(name); got != name {
		t.Errorf("truncateToolName(64 chars) = %q (len=%d), want original", got, len(got))
	}
}

func TestTruncateToolName_Over64(t *testing.T) {
	name := ""
	for i := 0; i < 100; i++ {
		name += "x"
	}
	got := truncateToolName(name)
	if len(got) > 64 {
		t.Errorf("truncateToolName(100 chars) length = %d, want <= 64", len(got))
	}
	// Same input should always produce same output (deterministic)
	got2 := truncateToolName(name)
	if got != got2 {
		t.Errorf("truncateToolName is not deterministic: %q vs %q", got, got2)
	}
}

func TestTruncateToolName_DifferentLongNames(t *testing.T) {
	name1 := ""
	name2 := ""
	for i := 0; i < 100; i++ {
		name1 += "a"
		name2 += "b"
	}
	got1 := truncateToolName(name1)
	got2 := truncateToolName(name2)
	if got1 == got2 {
		t.Errorf("Different long names should produce different truncated results: %q == %q", got1, got2)
	}
}

// --- Task 5: Empty Response Handling ---

func TestLegacyConvertOpenAIToClaude_NilResponse(t *testing.T) {
	originalReq := &models.ClaudeMessagesRequest{Model: "test-model"}
	result := legacyConvertOpenAIToClaude(nil, originalReq)
	if result == nil {
		t.Fatal("Expected non-nil response for nil input")
	}
	if result.Type != "message" {
		t.Errorf("Type = %q, want %q", result.Type, "message")
	}
	if len(result.Content) == 0 {
		t.Error("Expected at least one content block")
	}
	if result.Content[0].Type != "text" {
		t.Errorf("Content type = %q, want %q", result.Content[0].Type, "text")
	}
}

func TestLegacyConvertOpenAIToClaude_EmptyChoices(t *testing.T) {
	originalReq := &models.ClaudeMessagesRequest{Model: "test-model"}
	openAIResp := &models.OpenAIResponse{
		ID: "chatcmpl-test",
		Choices: []models.OpenAIChoice{},
		Usage: models.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}
	result := legacyConvertOpenAIToClaude(openAIResp, originalReq)
	if result == nil {
		t.Fatal("Expected non-nil response for empty choices")
	}
	if len(result.Content) == 0 {
		t.Error("Expected at least one content block for empty choices")
	}
}

// --- Tool Result Content Array Format ---

func TestToolMessageWithArrayContent(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter()
	conv.SetConfig(cfg)

	// Parse OpenAI format message with array content on tool message
	msg := &models.OpenAIMessage{
		Role:       "tool",
		ToolCallID: "call_123",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": "result text"},
		},
	}

	internalMsg := conv.convertOpenAIMessageToInternal(msg)
	if internalMsg.Role != "user" {
		t.Errorf("Role = %q, want %q", internalMsg.Role, "user")
	}
	if len(internalMsg.Content) != 1 {
		t.Fatalf("Content blocks = %d, want 1", len(internalMsg.Content))
	}
	if internalMsg.Content[0].Type != "tool_result" {
		t.Errorf("Content type = %q, want %q", internalMsg.Content[0].Type, "tool_result")
	}
	if internalMsg.Content[0].Content != "result text" {
		t.Errorf("Content = %q, want %q", internalMsg.Content[0].Content, "result text")
	}
}

func TestToolMessageWithEmptyContent(t *testing.T) {
	cfg := &config.Config{}
	conv := NewOpenAIConverter()
	conv.SetConfig(cfg)

	msg := &models.OpenAIMessage{
		Role:       "tool",
		ToolCallID: "call_456",
		Content:    nil,
	}

	internalMsg := conv.convertOpenAIMessageToInternal(msg)
	if internalMsg.Content[0].Content != "" {
		t.Errorf("Content = %q, want empty string for nil content", internalMsg.Content[0].Content)
	}
}
```

- [ ] **Step 3: 验证**
Run: `go test ./converter/ -run "TestToolUseBlockWithoutID|TestToolUseBlockWithExistingID|TestTruncateToolName|TestLegacyConvertOpenAIToClaude|TestToolMessageWith" -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS" for all tests
  - Output does NOT contain: "FAIL"

- [ ] **Step 4: 提交**
Run: `git add converter/response_converter.go converter/compat_test.go && git commit -m "fix(converter): return empty text block for nil/empty responses, add compatibility tests"`

---

## Phase 3: SELF-REVIEW

| # | Check | Result | Action Taken |
|---|-------|--------|-------------|
| 1 | Header 有 Goal + Architecture + Tech Stack? | PASS | - |
| 2 | 每个 Task 标注 Depends on? | PASS | - |
| 3 | 每个 Task 列出精确文件路径? | PASS | - |
| 4 | 每个 Task 有 3-8 Steps? | PASS | Task 1: 5, Task 2: 4, Task 3: 4, Task 4: 4, Task 5: 4 |
| 5 | 新文件步骤包含完整代码? | PASS | compat_test.go 完整 |
| 6 | 修改步骤包含替换后完整函数? | PASS | 所有修改都标注了文件路径和行号 |
| 7 | 代码块大小在 5-80 行? | PASS | - |
| 8 | 所有函数/类型在 Plan 内有定义? | PASS | generateShortID, truncateToolName 都有代码 |
| 9 | 每个 Task 有验证命令? | PASS | - |
| 10 | Spec 每个需求都有对应 Task? | PASS | 5 个改进点对应 5 个 Task |
| 11 | 每个 Task 完成后可独立验证? | PASS | 每个都有 build 验证 |
| 12 | 无 TBD/TODO/模糊描述? | PASS | - |
| 13 | 无 "add validation" 等抽象指令? | PASS | - |
| 14 | 跨 Task 函数签名一致? | PASS | generateShortID, truncateToolName 定义和使用一致 |
| 15 | 保存位置正确? | PASS | docs/superpowers/plans/ |

**Status:** ALL PASS

## Phase 4: EXECUTION SELECTION

**Tasks:** 5
**Dependencies:** Yes (sequential chain)
**User Preference:** None specified
**Decision:** Subagent-Driven
**Reasoning:** 5 tasks with sequential dependencies, each modifying overlapping files — best handled sequentially by a single agent to avoid merge conflicts.
