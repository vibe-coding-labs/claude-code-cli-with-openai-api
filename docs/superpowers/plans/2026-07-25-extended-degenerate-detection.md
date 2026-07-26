# 扩展无效输出检测 - 多类型无效输出的统一检测与重试机制 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 扩展现有的伪工具调用标记检测器，增加"空内容"和"纯空白内容"的检测，当检测到这些无效输出时触发 `overloaded_error` 让客户端自动重试，减少"对话停住"的问题。

**Architecture:** 扩展 `DegenerateOutputDetector` → 添加 `IsEmptyOrWhitespace` 方法 → 在流式/非流式路径中检测空内容（无文本且无工具调用）和纯空白内容 → 触发 `overloaded_error` → 复用现有的重试机制。

**Tech Stack:** Go 1.22, Gin, 正则表达式, strings 包

**Risks:**
- 空内容 + 工具调用的场景不触发重试 → 这是正常的工具执行场景，用户可能只要求执行操作而不需要文本响应
- 截断输出不触发重试 → 这是用户配置问题（`max_tokens` 过小），自动重试可能导致无限循环，由客户端根据 `stop_reason=max_tokens` 自行处理
- `content_filter` 不触发重试 → 这是安全检查结果，重试可能绕过安全限制

---

### Task 1: 扩展 DegenerateOutputDetector 添加空内容和纯空白检测

**Depends on:** None
**Files:**
- Modify: `backend/converter/degenerate_detector.go:86-115`（IsDegenerate 方法后添加新方法）
- Modify: `backend/converter/degenerate_detector_test.go`（添加新测试用例）

- [ ] **Step 1: 扩展 degenerate_detector.go — 添加 IsEmptyOrWhitespace 方法**

文件: `backend/converter/degenerate_detector.go`
在 `IsDegenerate` 方法之后（约第 102 行之后），添加以下方法：

```go
// IsEmptyOrWhitespace checks whether the given text is empty or contains only
// whitespace characters. This detects another form of degenerate output where
// the model produces no meaningful content.
//
// This is separate from IsDegenerate because:
// 1. Empty content might be normal when there are tool calls (tool-only response)
// 2. The caller needs to check context (e.g., presence of tool calls) before deciding
//
// Returns true if text is empty or contains only whitespace (spaces, tabs, newlines).
func (d *DegenerateOutputDetector) IsEmptyOrWhitespace(text string) bool {
	return strings.TrimSpace(text) == ""
}

// IsEmptyContent checks if a response has no meaningful content.
// This is the comprehensive check that considers both text content and tool calls.
//
// Parameters:
//   - textContent: the collected text content (may be empty)
//   - hasToolCalls: whether the response contains any tool calls
//
// Returns true if:
//   - textContent is empty or whitespace AND there are no tool calls
//
// This distinguishes between:
//   - Empty response (no content at all) → degenerate, should retry
//   - Tool-only response (no text but has tool calls) → normal, should not retry
func (d *DegenerateOutputDetector) IsEmptyContent(textContent string, hasToolCalls bool) bool {
	// Tool-only responses are valid — user may just want to execute a tool
	if hasToolCalls {
		return false
	}
	// Empty or whitespace-only text with no tool calls is degenerate
	return d.IsEmptyOrWhitespace(textContent)
}
```

同时，需要确保 `strings` 包已被导入。检查文件头部，确认 `import` 块包含 `"strings"`。

- [ ] **Step 2: 添加测试用例 — 验证空内容和纯空白检测**

文件: `backend/converter/degenerate_detector_test.go`
在现有测试函数之后，添加以下测试：

```go
func TestDegenerateDetector_IsEmptyOrWhitespace(t *testing.T) {
	d := GetDegenerateDetector()

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "empty string",
			text:     "",
			expected: true,
		},
		{
			name:     "only spaces",
			text:     "   ",
			expected: true,
		},
		{
			name:     "only tabs",
			text:     "\t\t\t",
			expected: true,
		},
		{
			name:     "only newlines",
			text:     "\n\n\n",
			expected: true,
		},
		{
			name:     "mixed whitespace",
			text:     " \t\n \r\n  ",
			expected: true,
		},
		{
			name:     "normal text with leading/trailing whitespace",
			text:     "  hello world  ",
			expected: false,
		},
		{
			name:     "normal text",
			text:     "This is a normal response.",
			expected: false,
		},
		{
			name:     "single character",
			text:     "a",
			expected: false,
		},
		{
			name:     "chinese characters",
			text:     "你好世界",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.IsEmptyOrWhitespace(tt.text)
			if got != tt.expected {
				t.Errorf("IsEmptyOrWhitespace(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestDegenerateDetector_IsEmptyContent(t *testing.T) {
	d := GetDegenerateDetector()

	tests := []struct {
		name         string
		textContent  string
		hasToolCalls bool
		expected     bool
	}{
		{
			name:         "empty text with no tool calls",
			textContent:  "",
			hasToolCalls: false,
			expected:     true,
		},
		{
			name:         "whitespace-only text with no tool calls",
			textContent:  "   \n\t  ",
			hasToolCalls: false,
			expected:     true,
		},
		{
			name:         "empty text with tool calls — normal tool-only response",
			textContent:  "",
			hasToolCalls: true,
			expected:     false,
		},
		{
			name:         "whitespace text with tool calls — normal tool-only response",
			textContent:  "  ",
			hasToolCalls: true,
			expected:     false,
		},
		{
			name:         "normal text with no tool calls",
			textContent:  "Hello, world!",
			hasToolCalls: false,
			expected:     false,
		},
		{
			name:         "normal text with tool calls",
			textContent:  "I'll help you with that.",
			hasToolCalls: true,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.IsEmptyContent(tt.textContent, tt.hasToolCalls)
			if got != tt.expected {
				t.Errorf("IsEmptyContent(%q, %v) = %v, want %v",
					tt.textContent, tt.hasToolCalls, got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 3: 验证扩展后的检测器**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/ -run TestDegenerateDetector -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/degenerate_detector.go backend/converter/degenerate_detector_test.go && git commit -m "feat(converter): add empty content and whitespace detection to DegenerateOutputDetector"`

---

### Task 2: 流式场景集成空内容检测

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/response_converter.go:505-525`（退化检测区块之后添加空内容检测）

- [ ] **Step 1: 修改流式转换器 — 在退化检测后添加空内容检测**
文件: `backend/converter/response_converter.go:505-525`

在现有的 degenerate output 检测之后（第 525 行 `}` 之后），添加空内容检测：

```go
// 文件: backend/converter/response_converter.go
// 在第 525 行的 `}` 之后添加

		// --- Empty content detection ---
		// Check if the response has no meaningful content (empty text and no tool calls).
		// This indicates the model produced a degenerate response, possibly due to
		// service issues, and should be retried.
		//
		// Note: Tool-only responses (has tool calls but no text) are NOT considered
		// degenerate — the user may have requested just tool execution.
		hasToolCalls := len(state.toolCalls) > 0
		if GetDegenerateDetector().IsEmptyContent(collectedText, hasToolCalls) {
			logger.Warn("[stream] empty content detected (no text and no tool calls), emitting overloaded_error for auto-retry")
			sendSSEError(c, "overloaded_error", "Empty response detected (no meaningful content). Please retry.")
			return &StreamingResult{
				Content:      collectedText,
				InputTokens:  degenUsage.InputTokens,
				OutputTokens: degenUsage.OutputTokens,
				StopReason:   "overloaded_error",
				ToolCalls:    nil,
			}
		}
```

**重要：** 此检测使用的是 `degenUsage` 变量（已在退化检测前从 `state.usage` 读取），确保一致性。

- [ ] **Step 2: 验证流式检测集成**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./converter/...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "undefined"

- [ ] **Step 3: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/response_converter.go && git commit -m "feat(converter): integrate empty content detection in streaming path"`

---

### Task 3: 非流式场景集成空内容检测

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/response_handler.go:133-148`（退化检测区块之后添加空内容检测）

- [ ] **Step 1: 修改非流式响应处理器 — 在退化检测后添加空内容检测**
文件: `backend/handler/response_handler.go:133-148`

在现有的 degenerate output 检测之后（第 148 行 `}` 之后），添加空内容检测：

```go
// 文件: backend/handler/response_handler.go
// 在第 148 行的 `}` 之后添加

		// --- Empty content detection ---
		// Check if the response has no meaningful content (empty text and no tool calls).
		// Tool-only responses are NOT considered degenerate.
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
				r.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
				return
			}
		}
```

- [ ] **Step 2: 验证非流式检测集成**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./handler/...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "undefined"

- [ ] **Step 3: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/handler/response_handler.go && git commit -m "feat(handler): integrate empty content detection in non-streaming path"`

---

### Task 4: 重试引擎集成空内容错误分类

**Depends on:** Task 2, Task 3
**Files:**
- Modify: `backend/retry/retry.go:55-109`（ClassifyError 函数）

- [ ] **Step 1: 修改 ClassifyError — 将 empty content 错误分类为可重试的服务端错误**
文件: `backend/retry/retry.go:55-109`

在现有的 "degenerate output" 检查之后，添加 "empty content" 检查：

```go
// 文件: backend/retry/retry.go
// 在 "degenerate output" 检查之后添加

		// Empty content — model produced no meaningful output (no text and no tool calls)
		if strings.Contains(errStr, "empty content") || strings.Contains(errStr, "empty response") {
			return CategoryServerError
		}
```

- [ ] **Step 2: 验证重试分类集成**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./retry/...`
Expected:
  - Exit code: 0

- [ ] **Step 3: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/retry/retry.go && git commit -m "feat(retry): classify empty content as retryable server error"`

---

### Task 5: 端到端测试

**Depends on:** Task 2, Task 3, Task 4
**Files:**
- Create: `backend/converter/degenerate_edge_cases_test.go`

- [ ] **Step 1: 创建端到端测试 — 验证空内容检测的边界情况**

```go
// backend/converter/degenerate_edge_cases_test.go
package converter

import (
	"testing"
)

// TestEdgeCases_EmptyContentVsToolOnly tests the critical distinction between:
// - Empty content (degenerate, should retry)
// - Tool-only response (normal, should NOT retry)
func TestEdgeCases_EmptyContentVsToolOnly(t *testing.T) {
	d := GetDegenerateDetector()

	// Case 1: Pure empty — should be detected as degenerate
	if !d.IsEmptyContent("", false) {
		t.Error("empty text with no tools should be degenerate")
	}

	// Case 2: Whitespace only — should be detected as degenerate
	if !d.IsEmptyContent("   \n\t  ", false) {
		t.Error("whitespace-only text with no tools should be degenerate")
	}

	// Case 3: Empty text but has tool calls — should NOT be degenerate
	// This is the key distinction: tool-only responses are valid
	if d.IsEmptyContent("", true) {
		t.Error("empty text with tool calls should NOT be degenerate (valid tool-only response)")
	}

	// Case 4: Whitespace text but has tool calls — should NOT be degenerate
	if d.IsEmptyContent("  ", true) {
		t.Error("whitespace text with tool calls should NOT be degenerate")
	}

	// Case 5: Normal text, no tools — should NOT be degenerate
	if d.IsEmptyContent("Hello", false) {
		t.Error("normal text should NOT be degenerate")
	}

	// Case 6: Normal text with tools — should NOT be degenerate
	if d.IsEmptyContent("I'll help you", true) {
		t.Error("normal text with tools should NOT be degenerate")
	}
}

// TestEdgeCases_DSMLOnly tests that DSML markers are detected even with surrounding whitespace
func TestEdgeCases_DSMLOnly(t *testing.T) {
	d := GetDegenerateDetector()

	testCases := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "DSML with surrounding whitespace",
			text:     "   \n  </｜DSML｜invoke>  \n  ",
			expected: true,
		},
		{
			name:     "only DSML marker",
			text:     "</｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "DSML in middle of text",
			text:     "Some text\n</｜DSML｜invoke>\nMore text",
			expected: true,
		},
		{
			name:     "legitimate XML should not match",
			text:     "<response><status>ok</status></response>",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := d.IsDegenerate(tc.text)
			if got != tc.expected {
				t.Errorf("IsDegenerate(%q) = %v, want %v", tc.text, got, tc.expected)
			}
		})
	}
}

// TestEdgeCases_WhitespaceVariants tests various whitespace patterns
func TestEdgeCases_WhitespaceVariants(t *testing.T) {
	d := GetDegenerateDetector()

	whitespaceVariants := []string{
		"",
		" ",
		"  ",
		"\t",
		"\n",
		"\r\n",
		" \t\n\r\n ",
		"     \n\n\n     ",
		strings.Repeat(" ", 100),
		strings.Repeat("\n", 50),
	}

	for i, text := range whitespaceVariants {
		t.Run("whitespace_variant_"+string(rune('0'+i)), func(t *testing.T) {
			// With no tool calls, whitespace should be detected as empty content
			if !d.IsEmptyContent(text, false) {
				t.Errorf("whitespace variant %d should be detected as empty content", i)
			}
			// With tool calls, whitespace should NOT be detected as empty content
			if d.IsEmptyContent(text, true) {
				t.Errorf("whitespace variant %d with tools should NOT be detected as empty content", i)
			}
		})
	}
}
```

- [ ] **Step 2: 运行所有测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/ -run "TestDegenerate|TestEdgeCases" -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

- [ ] **Step 3: 运行完整项目构建和测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test ./... 2>&1 | tail -20`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/degenerate_edge_cases_test.go && git commit -m "test(converter): add edge case tests for degenerate output detection"`
