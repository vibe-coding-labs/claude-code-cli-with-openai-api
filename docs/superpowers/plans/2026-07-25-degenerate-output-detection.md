# 检测并处理模型伪工具调用无效输出 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 检测上游模型在文本内容中输出的伪工具调用标记（如 `</｜DSML｜invoke>`、`</｜DSML｜tool_calls>` 等），将其识别为无效/退化输出，触发重试或错误处理，避免这些伪标记原样透传给 Claude Code CLI 导致解析混乱。

**Architecture:** 上游模型输出 → 流式/非流式转换器收集文本内容 → 在流结束（finish_reason）或非流式响应返回前，检测文本中是否包含伪工具调用标记模式 → 如果检测到，将 stop_reason 改为 `overloaded_error`（流式）或返回 503 错误（非流式），触发客户端自动重试。复用现有的 `overloaded_error` 重试机制，不引入新的错误类型。

**Tech Stack:** Go 1.22, Gin, 正则表达式

**Risks:**
- 伪标记模式可能随模型版本变化而变化 → 缓解：使用可配置的正则模式列表，支持环境变量动态添加
- 误判风险：正常文本中可能偶然包含类似标记 → 缓解：模式设计为高特异性（必须包含 `｜DSML｜` 或类似的结构化伪标记特征），且只检测闭合标签（`</...>`），不检测开放标签
- 流式场景下内容已经逐块发送给客户端，无法"撤回" → 缓解：流式场景在检测到伪标记后，立即发送 `overloaded_error` SSE 事件终止流，客户端会自动重试整个请求

---

### Task 1: 创建伪工具调用标记检测器

**Depends on:** None
**Files:**
- Create: `backend/converter/degenerate_detector.go`
- Create: `backend/converter/degenerate_detector_test.go`

- [ ] **Step 1: 创建 degenerate_detector.go — 定义伪工具调用标记检测逻辑**

```go
// backend/converter/degenerate_detector.go
package converter

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

// DegenerateOutputDetector detects pseudo-tool-call markers and other
// degenerate output patterns that indicate the upstream model produced
// invalid output instead of a proper response.
//
// Some models (e.g., certain DeepSeek variants) emit structured pseudo-tags
// like </｜DSML｜invoke> or </｜DSML｜tool_calls> in their text output when
// they attempt to call tools but the API doesn't support structured tool_calls.
// These markers are meaningless to Claude Code CLI and should be treated as
// degenerate output that warrants a retry.
type DegenerateOutputDetector struct {
	patterns []*regexp.Regexp
	mu       sync.RWMutex
}

// defaultDegeneratePatterns are the built-in patterns for known degenerate output.
// Each pattern is designed for HIGH SPECIFICITY to avoid false positives:
// - Must contain the distinctive ｜DSML｜ or similar structured pseudo-tag markers
// - Only matches closing tags (</...>), not opening tags that might be legitimate XML
// - The full-width pipe ｜ (U+FF5C) is a strong signal — it's never used in legitimate
//   XML/HTML but is common in these model-generated pseudo-tags
var defaultDegeneratePatterns = []string{
	// DSML pseudo-tool-call closing tags (DeepSeek pattern)
	// The full-width pipe ｜ (U+FF5C) is the key discriminant
	`</｜DSML｜\w+>`,
	// DSML pseudo-tool-call opening tags — less common but also degenerate
	`<｜DSML｜\w+>`,
	// Variant with half-width pipe (some models use this)
	`</\|DSML\|\w+>`,
	`<\|DSML\|\w+>`,
}

// globalDetector is the singleton detector instance, initialized once.
var globalDetector *DegenerateOutputDetector
var globalDetectorOnce sync.Once

// GetDegenerateDetector returns the global detector instance.
func GetDegenerateDetector() *DegenerateOutputDetector {
	globalDetectorOnce.Do(func() {
		globalDetector = newDegenerateDetector()
	})
	return globalDetector
}

// newDegenerateDetector creates a detector with default + env-configured patterns.
func newDegenerateDetector() *DegenerateOutputDetector {
	d := &DegenerateOutputDetector{}

	// Compile default patterns
	for _, p := range defaultDegeneratePatterns {
		re, err := regexp.Compile(p)
		if err == nil {
			d.patterns = append(d.patterns, re)
		}
	}

	// Load additional patterns from environment variable
	// PROXY_DEGENERATE_PATTERNS="pattern1;pattern2;pattern3"
	// Patterns are separated by semicolons
	if extra := os.Getenv("PROXY_DEGENERATE_PATTERNS"); extra != "" {
		for _, p := range strings.Split(extra, ";") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			re, err := regexp.Compile(p)
			if err != nil {
				// Log but don't fail — bad pattern is skipped
				continue
			}
			d.patterns = append(d.patterns, re)
		}
	}

	return d
}

// IsDegenerate checks whether the given text contains degenerate output patterns.
// Returns true if any configured pattern matches, along with the first matching pattern.
func (d *DegenerateOutputDetector) IsDegenerate(text string) (bool, string) {
	if text == "" {
		return false, ""
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, re := range d.patterns {
		if re.MatchString(text) {
			return true, re.String()
		}
	}
	return false, ""
}

// AddPattern adds a new detection pattern at runtime (for testing).
func (d *DegenerateOutputDetector) AddPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.patterns = append(d.patterns, re)
	return nil
}
```

- [ ] **Step 2: 创建 degenerate_detector_test.go — 测试伪标记检测的各类场景**

```go
// backend/converter/degenerate_detector_test.go
package converter

import (
	"testing"
)

func TestDegenerateDetector_DSMLClosingTags(t *testing.T) {
	d := GetDegenerateDetector()

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "DSML invoke closing tag with full-width pipe",
			text:     "  </｜DSML｜invoke>",
			expected: true,
		},
		{
			name:     "DSML tool_calls closing tag with full-width pipe",
			text:     "  </｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "DSML invoke opening tag with full-width pipe",
			text:     "  <｜DSML｜invoke>",
			expected: true,
		},
		{
			name:     "DSML tool_calls opening tag with full-width pipe",
			text:     "  <｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "DSML closing tag with half-width pipe",
			text:     "  </|DSML|invoke>",
			expected: true,
		},
		{
			name:     "DSML opening tag with half-width pipe",
			text:     "  <|DSML|tool_calls>",
			expected: true,
		},
		{
			name:     "DSML tags embedded in longer output",
			text:     "Here is the result:\n  </｜DSML｜invoke>\n  </｜DSML｜tool_calls>",
			expected: true,
		},
		{
			name:     "normal XML should not match",
			text:     "<tool><name>read_file</name></tool>",
			expected: false,
		},
		{
			name:     "normal HTML should not match",
			text:     "<div class=\"container\">Hello</div>",
			expected: false,
		},
		{
			name:     "empty string should not match",
			text:     "",
			expected: false,
		},
		{
			name:     "plain text should not match",
			text:     "This is a normal response from the model.",
			expected: false,
		},
		{
			name:     "XML with pipe character should not match",
			text:     "<data|format>some content</data|format>",
			expected: false,
		},
		{
			name:     "real user example from issue",
			text:     "● ARGUMENTS: 继续\n  </｜DSML｜invoke>\n  </｜DSML｜tool_calls>\n\n✻ Sautéed for 8s",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := d.IsDegenerate(tt.text)
			if got != tt.expected {
				t.Errorf("IsDegenerate(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestDegenerateDetector_AddPattern(t *testing.T) {
	d := newDegenerateDetector()

	// Before adding custom pattern
	got, _ := d.IsDegenerate("  </CUSTOM_TAG>test")
	if got {
		t.Error("should not match before adding custom pattern")
	}

	// Add custom pattern
	err := d.AddPattern(`</CUSTOM_\w+>`)
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	// After adding custom pattern
	got, _ = d.IsDegenerate("  </CUSTOM_TAG>test")
	if !got {
		t.Error("should match after adding custom pattern")
	}
}

func TestDegenerateDetector_InvalidPattern(t *testing.T) {
	d := newDegenerateDetector()

	err := d.AddPattern(`[invalid regex`)
	if err == nil {
		t.Error("AddPattern should return error for invalid regex")
	}
}
```

- [ ] **Step 3: 验证 DegenerateOutputDetector**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/ -run TestDegenerateDetector -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/degenerate_detector.go backend/converter/degenerate_detector_test.go && git commit -m "feat(converter): add degenerate output detector for pseudo-tool-call markers"`

---

### Task 2: 流式场景集成伪标记检测

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/response_converter.go:496-525`（流正常完成后的内容验证区块）

- [ ] **Step 1: 修改流式转换器 — 在流正常完成后检测伪标记并触发 overloaded_error**
文件: `backend/converter/response_converter.go:496-525`（流正常完成后的处理区块，在 heartbeat.Stop() 之后、emitContentBlockStop 之前）

在 `heartbeat.Stop()` 之后、`if !state.sentContentBlockFinish` 之前，插入伪标记检测逻辑：

```go
// 文件: backend/converter/response_converter.go
// 在 heartbeat.Stop() 之后（约第 503 行），在 "If content_block_finish was never sent" 之前插入

		// --- Degenerate output detection ---
		// Check if the collected text content contains pseudo-tool-call markers
		// (e.g., </｜DSML｜invoke>). These indicate the upstream model tried to
		// call tools via text output instead of the structured tool_calls field,
		// producing invalid output that Claude Code CLI cannot parse.
		// Treat as overloaded_error so the client auto-retries.
		collectedText := collectedContent.String()
		if isDegenerate, pattern := GetDegenerateDetector().IsDegenerate(collectedText); isDegenerate {
			logger.Warn("[stream] degenerate output detected (pattern=%s), emitting overloaded_error for auto-retry. Content preview: %.200s", pattern, collectedText)
			sendSSEError(c, "overloaded_error", "Degenerate output detected (pseudo-tool-call markers in text). Please retry.")
			return &StreamingResult{
				Content:      collectedText,
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
				StopReason:   "overloaded_error",
				ToolCalls:    nil,
			}
		}
```

**注意：** 此检测在流式内容已经逐块发送给客户端之后执行。虽然无法撤回已发送的文本块，但发送 `overloaded_error` 事件会触发 Claude Code CLI 的自动重试机制，客户端会丢弃当前响应并重新发起请求。这是与现有 stall/timeout 处理一致的模式。

- [ ] **Step 2: 验证流式检测集成**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "undefined"

- [ ] **Step 3: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/response_converter.go && git commit -m "feat(converter): integrate degenerate output detection in streaming path"`

---

### Task 3: 非流式场景集成伪标记检测

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/response_handler.go:123-131`（非流式响应转换后的验证区块）

- [ ] **Step 1: 修改非流式响应处理器 — 在转换后检测伪标记并返回 503 错误**
文件: `backend/handler/response_handler.go:123-131`（`ConvertOpenAIToClaudeResponse` 调用后的验证区块）

在现有的 `claudeResp == nil` 检查之后，添加伪标记检测：

```go
// 文件: backend/handler/response_handler.go
// 在 claudeResp == nil 检查之后（约第 131 行之后）插入

		// --- Degenerate output detection ---
		// Check if the response text contains pseudo-tool-call markers
		// (e.g., </｜DSML｜invoke>). These indicate invalid model output.
		if claudeResp != nil && len(claudeResp.Content) > 0 {
			for _, block := range claudeResp.Content {
				if block.Type == "text" && block.Text != "" {
					if isDegenerate, pattern := converter.GetDegenerateDetector().IsDegenerate(block.Text); isDegenerate {
						utils.GetLogger().Warn("[Non-Streaming] degenerate output detected (pattern=%s), returning overloaded_error. Content preview: %.200s", pattern, block.Text)
						err := fmt.Errorf("degenerate output detected (pseudo-tool-call markers in text, pattern=%s). Please retry.", pattern)
						r.sendErrorResponse(c, err)
						r.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), claudeReq, nil)
						return
					}
				}
			}
		}
```

- [ ] **Step 2: 验证非流式检测集成**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "undefined"

- [ ] **Step 3: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/handler/response_handler.go && git commit -m "feat(handler): integrate degenerate output detection in non-streaming path"`

---

### Task 4: 重试引擎集成伪标记错误分类

**Depends on:** Task 2, Task 3
**Files:**
- Modify: `backend/retry/retry.go:55-104`（ClassifyError 函数）
- Modify: `backend/database/proxy_errors.go`（错误类型常量，如已存在则复用）

- [ ] **Step 1: 修改 ClassifyError — 将 degenerate output 分类为可重试的服务端错误**
文件: `backend/retry/retry.go:55-104`（ClassifyError 函数）

在现有的错误分类逻辑中，添加对 "degenerate output" 的识别：

```go
// 文件: backend/retry/retry.go
// 在 ClassifyError 函数中，在现有的 "empty choices" 和 "decode response" 检查之后添加

	// Degenerate output — model produced pseudo-tool-call markers instead of valid response
	if strings.Contains(errMsg, "degenerate output") {
		return CategoryServerError
	}
```

- [ ] **Step 2: 验证重试分类集成**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./retry/ -run TestClassifyError -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

- [ ] **Step 3: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/retry/retry.go && git commit -m "feat(retry): classify degenerate output as retryable server error"`

---

### Task 5: 端到端集成测试

**Depends on:** Task 2, Task 3, Task 4
**Files:**
- Create: `backend/converter/degenerate_integration_test.go`

- [ ] **Step 1: 创建集成测试 — 验证流式和非流式路径的伪标记检测端到端工作**

```go
// backend/converter/degenerate_integration_test.go
package converter

import (
	"strings"
	"testing"
)

func TestDegenerateDetector_StreamContentDetection(t *testing.T) {
	d := GetDegenerateDetector()

	// Simulate content that would be collected during streaming
	testCases := []struct {
		name        string
		content     string
		isDegenerate bool
	}{
		{
			name:        "real issue example - DSML invoke and tool_calls",
			content:     "● ARGUMENTS: 继续\n  </｜DSML｜invoke>\n  </｜DSML｜tool_calls>\n\n✻ Sautéed for 8s",
			isDegenerate: true,
		},
		{
			name:        "only DSML invoke closing",
			content:     "Some text before\n  </｜DSML｜invoke>\nSome text after",
			isDegenerate: true,
		},
		{
			name:        "only DSML tool_calls closing",
			content:     "  </｜DSML｜tool_calls>",
			isDegenerate: true,
		},
		{
			name:        "half-width pipe variant",
			content:     "  </|DSML|invoke>\n  </|DSML|tool_calls>",
			isDegenerate: true,
		},
		{
			name:        "normal tool use XML - not degenerate",
			content:     "<tool><name>read_file</name><parameters><path>/tmp/test</path></parameters></tool>",
			isDegenerate: false,
		},
		{
			name:        "normal markdown response - not degenerate",
			content:     "Here's the implementation:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```",
			isDegenerate: false,
		},
		{
			name:        "empty content - not degenerate",
			content:     "",
			isDegenerate: false,
		},
		{
			name:        "legitimate XML with pipes - not degenerate",
			content:     "<data|separator>value</data|separator>",
			isDegenerate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, pattern := d.IsDegenerate(tc.content)
			if got != tc.isDegenerate {
				t.Errorf("IsDegenerate(%q) = %v (pattern=%s), want %v",
					tc.content, got, pattern, tc.isDegenerate)
			}
		})
	}
}

func TestDegenerateDetector_MultiplePatternsInContent(t *testing.T) {
	d := GetDegenerateDetector()

	// Content with multiple degenerate markers
	content := "Starting analysis...\n<｜DSML｜invoke>\n  </｜DSML｜tool_calls>\nDone."
	got, _ := d.IsDegenerate(content)
	if !got {
		t.Error("content with multiple DSML markers should be detected as degenerate")
	}
}

func TestDegenerateDetector_StreamingIncrementalContent(t *testing.T) {
	d := GetDegenerateDetector()

	// Simulate how content accumulates during streaming
	chunks := []string{
		"● ARGUMENTS: 继续\n",
		"  </｜DSML｜invoke>\n",
		"  </｜DSML｜tool_calls>\n",
		"\n✻ Sautéed for 8s",
	}

	var full strings.Builder
	detectedAtChunk := -1
	for i, chunk := range chunks {
		full.WriteString(chunk)
		if isDegenerate, _ := d.IsDegenerate(full.String()); isDegenerate {
			detectedAtChunk = i
			break
		}
	}

	if detectedAtChunk < 0 {
		t.Error("degenerate content should be detected at some point during accumulation")
	}
	if detectedAtChunk != 1 {
		t.Errorf("expected detection at chunk 1 (first DSML marker), got chunk %d", detectedAtChunk)
	}
}
```

- [ ] **Step 2: 运行所有测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./converter/ -run TestDegenerateDetector -v && go test ./converter/ -run TestDegenerate -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

- [ ] **Step 3: 运行完整项目构建和测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test ./... 2>&1 | tail -20`
Expected:
  - Exit code: 0
  - Output does NOT contain: "FAIL" or "build error"

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/degenerate_integration_test.go && git commit -m "test(converter): add integration tests for degenerate output detection"`
