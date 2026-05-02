# Protocol Compatibility Hardening — 极致兼容性工程

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 通过分析 GitHub 上 7 个同类项目（1rgs/claude-code-proxy 3529⭐、fuergaosi233 2535⭐、nielspeter/OpenRouter、UniClaudeProxy/ReAct、antomix/双向桥接、AIDotNet/C#、c2js/Rust Responses API），将关键兼容性技术移植到我们的项目，使其支持所有主流 LLM 提供商和 Claude Code 各版本的协议差异。

**Architecture:** 请求流入 → Claude Code 协议解析 → ReAct XML Fallback 检测（模型不支持原生 tool calling 则自动降级为 XML 注入）→ Tool Call ID 格式标准化 → 参数名称规范化（camelCase↔snake_case）→ 停止原因完整映射 → 流式响应 SSE 标准化。核心思路：每个兼容层独立，可按需启用，不互相耦合。

**Tech Stack:** Go 1.22, 无新外部依赖，纯标准库实现

**Risks:**
- Task 1 ReAct XML 注入会修改 system prompt → 缓解：仅在检测到模型不支持 function calling 时启用，保持原有路径不变
- Task 2 修改 openai.go 核心转换逻辑 → 缓解：只在现有函数中增加 case 分支，不改已有逻辑
- Task 3 停止原因映射扩展 → 缓解：default 分支保持现有行为

---

### Task 1: ReAct XML Tool Calling Fallback

**Depends on:** None
**Files:**
- Create: `converter/react.go`（XML 工具描述构建器 + XML 响应解析器）
- Create: `converter/react_test.go`（覆盖注入、解析、流式场景）

**Why:** UniClaudeProxy 验证了这是兼容不支持原生 function calling 模型（Ollama、开源模型、某些商业模型）的关键技术。Claude Code 严重依赖 tool calling，没有这个 fallback 就无法在这些模型上工作。

- [ ] **Step 1: 创建 ReAct XML 转换器 — 支持 XML 工具描述注入和响应解析**

文件: `converter/react.go`

```go
package converter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ReactConverter handles ReAct XML tool calling fallback for models
// that do not support native function calling.
type ReactConverter struct{}

// reactSystemTemplate is injected into the system prompt to teach the model
// how to call tools using XML format.
const reactSystemTemplate = `

TOOL CALLING — YOU MUST USE THIS EXACT XML FORMAT

To call a tool, you MUST output this EXACT XML block — nothing else works:

<tool>
<name>TOOL_NAME</name>
<parameters>
{"param1": "value1"}
</parameters>
</tool>

You may call multiple tools by outputting multiple XML blocks.
You MUST use valid JSON for the parameters.
After receiving tool results, continue your response normally.

Available tools:
%s

IMPORTANT: When you need to call a tool, output the XML block directly. Do NOT wrap it in markdown or code fences.`

// ToolCall represents a parsed tool call from XML response.
type ParsedToolCall struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

var (
	// toolCallRe matches complete XML tool call blocks
	toolCallRe = regexp.MustCompile(
		`(?s)<tool>\s*<name>\s*(.*?)\s*</name>\s*<parameters>\s*(.*?)\s*</parameters>\s*</tool>`,
	)
	// partialToolCallRe matches incomplete XML tool call blocks (for streaming)
	partialToolCallRe = regexp.MustCompile(
		`(?s)<tool>\s*<name>\s*(.*?)\s*<parameters>\s*(.*?)\s*$`,
	)
)

// BuildReactSystemPrompt injects XML tool descriptions into the system prompt.
func (r *ReactConverter) BuildReactSystemPrompt(tools []ToolDefinition, originalSystem string) string {
	var toolDescs []string
	for _, tool := range tools {
		schemaJSON, _ := json.Marshal(tool.Parameters)
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s\n  Schema: %s", tool.Name, tool.Description, string(schemaJSON)))
	}
	toolSection := strings.Join(toolDescs, "\n")
	xmlInstruction := fmt.Sprintf(reactSystemTemplate, toolSection)

	if originalSystem == "" {
		return xmlInstruction
	}
	return originalSystem + xmlInstruction
}

// ParseToolCallsFromResponse extracts XML tool calls from model response text.
// Returns: parsed tool calls, remaining text (non-tool content), whether any tools were found.
func (r *ReactConverter) ParseToolCallsFromResponse(text string) ([]ParsedToolCall, string, bool) {
	matches := toolCallRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, text, false
	}

	var calls []ParsedToolCall
	for _, match := range matches {
		name := strings.TrimSpace(match[1])
		paramsStr := strings.TrimSpace(match[2])

		var params map[string]interface{}
		if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
			params = map[string]interface{}{"raw": paramsStr}
		}

		calls = append(calls, ParsedToolCall{
			Name:       name,
			Parameters: params,
		})
	}

	// Remove all XML blocks from text to get remaining content
	remaining := toolCallRe.ReplaceAllString(text, "")
	remaining = strings.TrimSpace(remaining)

	return calls, remaining, true
}

// HasPartialToolCall checks if text contains an incomplete XML tool call (for streaming).
func (r *ReactConverter) HasPartialToolCall(text string) bool {
	// Check if there's an opening <tool> without a closing </tool>
	return strings.Contains(text, "<tool>") && !strings.Contains(text, "</tool>")
}

// ConvertToolCallsToContentBlocks converts parsed tool calls to Claude content blocks.
func (r *ReactConverter) ConvertToolCallsToContentBlocks(calls []ParsedToolCall) []ContentBlock {
	var blocks []ContentBlock
	for i, call := range calls {
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    fmt.Sprintf("toolu_%016x", i),
			Name:  call.Name,
			Input: call.Parameters,
		})
	}
	return blocks
}

// ConvertToolResultsToXML converts Claude tool_result messages to XML format for request injection.
func (r *ReactConverter) ConvertToolResultsToXML(toolUseID string, content string) string {
	return fmt.Sprintf("[Tool result for %s]: %s", toolUseID, content)
}

// NeedsReactFallback determines if the model needs ReAct XML fallback.
// Models that support native function calling do NOT need fallback.
func NeedsReactFallback(modelName string) bool {
	// Models known to NOT support function calling
	nonFunctionModels := []string{
		"ollama", "llama", "mistral:7b", "mistral:13b",
		"qwen", "yi-", "deepseek-coder", "starcoder",
		"codellama", "phi-", "gemma",
	}

	lower := strings.ToLower(modelName)
	for _, prefix := range nonFunctionModels {
		if strings.Contains(lower, prefix) {
			return true
		}
	}

	// Ollama local models always need fallback (no function calling support)
	if strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1") {
		return true
	}

	return false
}
```

- [ ] **Step 2: 创建 ReAct XML 转换器测试**

文件: `converter/react_test.go`

```go
package converter

import (
	"strings"
	"testing"
)

func TestReactConverter_BuildReactSystemPrompt(t *testing.T) {
	tools := []ToolDefinition{
		{Name: "Bash", Description: "Run a bash command", Parameters: map[string]interface{}{"command": "string"}},
		{Name: "Read", Description: "Read a file", Parameters: map[string]interface{}{"path": "string"}},
	}
	rc := &ReactConverter{}
	prompt := rc.BuildReactSystemPrompt(tools, "You are helpful.")

	if !strings.Contains(prompt, "You are helpful.") {
		t.Error("should preserve original system prompt")
	}
	if !strings.Contains(prompt, "<tool>") {
		t.Error("should contain XML format instruction")
	}
	if !strings.Contains(prompt, "Bash") || !strings.Contains(prompt, "Read") {
		t.Error("should list all tools")
	}
}

func TestReactConverter_BuildReactSystemPrompt_EmptySystem(t *testing.T) {
	rc := &ReactConverter{}
	prompt := rc.BuildReactSystemPrompt([]ToolDefinition{}, "")
	if strings.HasPrefix(prompt, "\n\n") {
		t.Error("should not start with double newline for empty system")
	}
	if !strings.Contains(prompt, "TOOL CALLING") {
		t.Error("should contain tool calling instruction")
	}
}

func TestReactConverter_ParseToolCallsFromResponse_SingleTool(t *testing.T) {
	rc := &ReactConverter{}
	text := `I'll read the file for you.
<tool>
<name>Read</name>
<parameters>
{"path": "/tmp/test.txt"}
</parameters>
</tool>`

	calls, remaining, found := rc.ParseToolCallsFromResponse(text)
	if !found {
		t.Fatal("should find tool call")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "Read" {
		t.Errorf("expected Read, got %s", calls[0].Name)
	}
	if calls[0].Parameters["path"] != "/tmp/test.txt" {
		t.Errorf("unexpected parameters: %v", calls[0].Parameters)
	}
	if !strings.Contains(remaining, "I'll read the file") {
		t.Error("should preserve non-tool text")
	}
}

func TestReactConverter_ParseToolCallsFromResponse_MultipleTools(t *testing.T) {
	rc := &ReactConverter{}
	text := `<tool>
<name>Bash</name>
<parameters>
{"command": "ls -la"}
</parameters>
</tool>
<tool>
<name>Read</name>
<parameters>
{"path": "main.go"}
</parameters>
</tool>`

	calls, _, found := rc.ParseToolCallsFromResponse(text)
	if !found {
		t.Fatal("should find tool calls")
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "Bash" || calls[1].Name != "Read" {
		t.Errorf("unexpected call names: %s, %s", calls[0].Name, calls[1].Name)
	}
}

func TestReactConverter_ParseToolCallsFromResponse_NoTools(t *testing.T) {
	rc := &ReactConverter{}
	text := "Just a regular response without any tool calls."

	calls, remaining, found := rc.ParseToolCallsFromResponse(text)
	if found {
		t.Error("should not find tools in plain text")
	}
	if calls != nil {
		t.Error("calls should be nil")
	}
	if remaining != text {
		t.Error("remaining text should be unchanged")
	}
}

func TestReactConverter_ParseToolCallsFromResponse_InvalidJSON(t *testing.T) {
	rc := &ReactConverter{}
	text := `<tool>
<name>Bash</name>
<parameters>
{invalid json}
</parameters>
</tool>`

	calls, _, found := rc.ParseToolCallsFromResponse(text)
	if !found {
		t.Fatal("should find tool call even with invalid JSON")
	}
	if calls[0].Name != "Bash" {
		t.Errorf("expected Bash, got %s", calls[0].Name)
	}
	if calls[0].Parameters["raw"] == nil {
		t.Error("should fallback to raw string for invalid JSON")
	}
}

func TestReactConverter_HasPartialToolCall(t *testing.T) {
	rc := &ReactConverter{}
	if rc.HasPartialToolCall("no tool here") {
		t.Error("should not detect partial in plain text")
	}
	if !rc.HasPartialToolCall("<tool>\n<name>Bash") {
		t.Error("should detect partial tool call")
	}
	if rc.HasPartialToolCall("<tool>\n</tool>") {
		t.Error("should not detect partial for complete block")
	}
}

func TestReactConverter_ConvertToolCallsToContentBlocks(t *testing.T) {
	rc := &ReactConverter{}
	calls := []ParsedToolCall{
		{Name: "Bash", Parameters: map[string]interface{}{"command": "ls"}},
		{Name: "Read", Parameters: map[string]interface{}{"path": "main.go"}},
	}
	blocks := rc.ConvertToolCallsToContentBlocks(calls)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "tool_use" || blocks[0].Name != "Bash" {
		t.Errorf("unexpected first block: %+v", blocks[0])
	}
	if !strings.HasPrefix(blocks[0].ID, "toolu_") {
		t.Errorf("ID should start with toolu_, got %s", blocks[0].ID)
	}
}

func TestNeedsReactFallback(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"ollama/llama3", true},
		{"gpt-4o", false},
		{"claude-3-opus", false},
		{"deepseek-coder-6.7b", true},
		{"qwen2.5-72b", true},
		{"gpt-4o-mini", false},
	}
	for _, tt := range tests {
		if got := NeedsReactFallback(tt.model); got != tt.expected {
			t.Errorf("NeedsReactFallback(%q) = %v, want %v", tt.model, got, tt.expected)
		}
	}
}
```

- [ ] **Step 3: 验证 ReAct 转换器**
Run: `go test ./converter/ -v -count=1 -run "TestReactConverter|TestNeedsReactFallback" 2>&1 | tail -20`
Expected:
  - Exit code: 0
  - Output contains: "PASS" for all tests

- [ ] **Step 4: 提交**
Run: `git add converter/react.go converter/react_test.go && git commit -m "feat(converter): add ReAct XML tool calling fallback for non-function-calling models"`

---

### Task 2: Tool Call ID & Parameter Normalization

**Depends on:** None
**Files:**
- Modify: `converter/openai.go:583-586`（tool call ID 生成标准化）
- Modify: `converter/openai.go:717-731`（tool call ID 传入保持原样）
- Modify: `converter/openai.go`（新增参数名称规范化函数）
- Create: `converter/normalizer.go`（ID 前缀标准化 + 参数名称规范化）
- Create: `converter/normalizer_test.go`

**Why:** 不同提供商使用不同 tool call ID 前缀（OpenAI: `call_`、Claude: `toolu_`、其他: `fc_`）。Claude Code 要求稳定的 ID 映射（tool_use → tool_result 必须用同一个 ID）。UniClaudeProxy 发现模型返回 camelCase 参数名但 Claude Code 期望 snake_case，导致工具调用失败。

- [ ] **Step 1: 创建 normalizer.go — Tool Call ID 标准化和参数名称规范化**

文件: `converter/normalizer.go`

```go
package converter

import (
	"regexp"
	"strings"
)

// Tool call ID prefixes from different providers
const (
	IDPrefixClaude  = "toolu_" // Anthropic Claude native prefix
	IDPrefixOpenAI  = "call_"  // OpenAI native prefix
	IDPrefixGeneric = "fc_"    // Generic function call prefix
)

// NormalizeToolCallID standardizes tool call IDs to use the Claude prefix.
// Claude Code requires consistent IDs between tool_use and tool_result.
func NormalizeToolCallID(id string) string {
	if id == "" {
		return ""
	}
	// If already has Claude prefix, keep as-is
	if strings.HasPrefix(id, IDPrefixClaude) {
		return id
	}
	// Strip known prefixes and add Claude prefix
	for _, prefix := range []string{IDPrefixOpenAI, IDPrefixGeneric} {
		if strings.HasPrefix(id, prefix) {
			return IDPrefixClaude + strings.TrimPrefix(id, prefix)
		}
	}
	// Unknown prefix — prepend Claude prefix
	return IDPrefixClaude + id
}

// PreserveToolCallID keeps the original tool call ID from the upstream provider.
// This is used when we need to send back tool results matching the original IDs.
func PreserveToolCallID(id string) string {
	return id
}

var camelRe = regexp.MustCompile(`(?=[A-Z])`)

// CamelToSnake converts camelCase or PascalCase to snake_case.
func CamelToSnake(name string) string {
	result := camelRe.Split(name, -1)
	for i, part := range result {
		result[i] = strings.ToLower(part)
	}
	return strings.Join(result, "_")
}

// SnakeToCamel converts snake_case to camelCase.
func SnakeToCamel(name string) string {
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// NormalizeToolParameters normalizes parameter names to match expected casing.
// Claude Code tools use snake_case parameter names (e.g., file_path, old_string).
// Some models (Gemini, certain OpenAI models) may return camelCase.
func NormalizeToolParameters(toolName string, params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}

	// Known Claude Code tools and their expected parameter names (all snake_case)
	knownSnakeParams := map[string]bool{
		"command": true, "file_path": true, "old_string": true, "new_string": true,
		"search_text": true, "replace_text": true, "pattern": true, "path": true,
		"content": true, "offset": true, "limit": true, "name": true,
		"description": true, "query": true, "glob": true, "timeout": true,
	}

	normalized := make(map[string]interface{}, len(params))
	for key, value := range params {
		// Check if this is already a known parameter
		if knownSnakeParams[key] {
			normalized[key] = value
			continue
		}
		// Try converting from camelCase to snake_case
		snakeKey := CamelToSnake(key)
		if knownSnakeParams[snakeKey] {
			normalized[snakeKey] = value
		} else {
			// Keep original if no known mapping
			normalized[key] = value
		}
	}

	return normalized
}
```

- [ ] **Step 2: 创建 normalizer_test.go**

文件: `converter/normalizer_test.go`

```go
package converter

import (
	"testing"
)

func TestNormalizeToolCallID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"toolu_abc123", "toolu_abc123"},
		{"call_abc123", "toolu_abc123"},
		{"fc_abc123", "toolu_abc123"},
		{"abc123", "toolu_abc123"},
		{"", ""},
		{"toolu_", "toolu_"},
	}
	for _, tt := range tests {
		if got := NormalizeToolCallID(tt.input); got != tt.expected {
			t.Errorf("NormalizeToolCallID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"filePath", "file_path"},
		{"oldString", "old_string"},
		{"newString", "new_string"},
		{"searchText", "search_text"},
		{"already_snake", "already_snake"},
		{"simple", "simple"},
		{"HTMLParser", "h_t_m_l_parser"},
	}
	for _, tt := range tests {
		if got := CamelToSnake(tt.input); got != tt.expected {
			t.Errorf("CamelToSnake(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSnakeToCamel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"file_path", "filePath"},
		{"old_string", "oldString"},
		{"already", "already"},
	}
	for _, tt := range tests {
		if got := SnakeToCamel(tt.input); got != tt.expected {
			t.Errorf("SnakeToCamel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeToolParameters(t *testing.T) {
	// Test camelCase → snake_case conversion
	params := map[string]interface{}{
		"filePath":  "/tmp/test.txt",
		"oldString": "hello",
		"command":   "ls",
	}
	normalized := NormalizeToolParameters("Edit", params)

	if _, ok := normalized["file_path"]; !ok {
		t.Error("expected filePath to be converted to file_path")
	}
	if _, ok := normalized["old_string"]; !ok {
		t.Error("expected oldString to be converted to old_string")
	}
	if _, ok := normalized["command"]; !ok {
		t.Error("command should be preserved as-is")
	}
}

func TestNormalizeToolParameters_NilInput(t *testing.T) {
	if result := NormalizeToolParameters("Bash", nil); result != nil {
		t.Error("nil input should return nil")
	}
}
```

- [ ] **Step 3: 集成参数规范化到 openai.go — 在 ParseResponse 中规范化 tool call 参数**

文件: `converter/openai.go:717-731`（修改 ParseResponse 中的 tool_calls 解析）

在 tool_calls 解析循环中，对 input 应用参数规范化。在现有 `json.Unmarshal` 后添加一行：

```go
// 在 converter/openai.go ParseResponse 函数中，约 line 719-730
// 修改前:
// var input map[string]interface{}
// if tc.Function.Arguments != "" {
//     _ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
// }

// 修改后:
var input map[string]interface{}
if tc.Function.Arguments != "" {
    _ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
}
input = NormalizeToolParameters(tc.Function.Name, input)
```

具体操作：在 `converter/openai.go` 中 `json.Unmarshal` 那行之后添加一行 `input = NormalizeToolParameters(tc.Function.Name, input)`

- [ ] **Step 4: 验证 normalizer 和集成**
Run: `go test ./converter/ -v -count=1 -run "TestNormalize|TestCamel|TestSnake" 2>&1 | tail -15`
Expected:
  - Exit code: 0
  - Output contains: "PASS" for all tests

Run: `go build ./converter/ 2>&1 | head -5`
Expected:
  - Exit code: 0
  - No output (clean build)

- [ ] **Step 5: 提交**
Run: `git add converter/normalizer.go converter/normalizer_test.go converter/openai.go && git commit -m "feat(converter): add tool call ID normalization and parameter name auto-fix"`

---

### Task 3: Stop Reason & Streaming Event Compatibility

**Depends on:** Task 2
**Files:**
- Modify: `converter/openai.go:694-704`（完善 stop reason 映射）
- Modify: `converter/openai.go:794-800`（完善反向 stop reason 映射）
- Modify: `converter/openai.go:943-953`（完善流式 stop reason 映射）
- Modify: `converter/response_converter.go:400-403`（完善流式 stop reason 映射）

**Why:** 不同提供商返回不同的 finish_reason 值。OpenAI 可能返回 `content_filter`、`recitation`；Gemini 返回 `SAFETY`、`RECITATION`。如果不映射这些，Claude Code 会收到未知的 stop_reason 导致行为异常。

- [ ] **Step 1: 完善 ParseResponse 中的 stop reason 映射**

文件: `converter/openai.go:694-704`（替换 stop reason 映射代码块）

```go
// 替换 converter/openai.go 中约 line 694-704 的 switch
switch choice.FinishReason {
case "stop":
	resp.StopReason = "end_turn"
case "length":
	resp.StopReason = "max_tokens"
case "tool_calls", "function_call":
	resp.StopReason = "tool_use"
case "content_filter":
	resp.StopReason = "end_turn"
case "interrupt":
	resp.StopReason = "end_turn"
default:
	resp.StopReason = "end_turn"
}
```

- [ ] **Step 2: 完善 BuildResponse 中的反向 stop reason 映射**

文件: `converter/openai.go:794-800`（替换反向 stop reason 映射）

```go
// 替换 converter/openai.go 中约 line 794-800 的 switch
finishReason := "stop"
switch resp.StopReason {
case "max_tokens":
	finishReason = "length"
case "tool_use":
	finishReason = "tool_calls"
case "end_turn":
	finishReason = "stop"
case "stop_sequence":
	finishReason = "stop"
default:
	finishReason = "stop"
}
```

- [ ] **Step 3: 完善 BuildStreamEvent 中的流式 stop reason 映射**

文件: `converter/openai.go:943-953`（替换流式 finish reason 映射）

```go
// 替换 converter/openai.go 中约 line 943-953 的 switch
finishReason := event.Delta.StopReason
switch finishReason {
case "end_turn":
	finishReason = "stop"
case "max_tokens":
	finishReason = "length"
case "tool_use":
	finishReason = "tool_calls"
case "stop_sequence":
	finishReason = "stop"
case "content_filter":
	finishReason = "stop"
}
```

- [ ] **Step 4: 完善 response_converter.go 中的流式 stop reason 映射**

文件: `converter/response_converter.go:400-403`（替换流式 stop reason default 分支区域）

在 response_converter.go 中找到 finish_reason 的 switch 语句，确保包含以下完整映射：

```go
switch finishReason {
case "stop":
	state.finalStopReason = models.StopEndTurn
case "length":
	state.finalStopReason = models.StopMaxTokens
case "tool_calls", "function_call":
	state.finalStopReason = models.StopToolUse
case "content_filter":
	state.finalStopReason = models.StopEndTurn
default:
	state.finalStopReason = models.StopEndTurn
}
```

- [ ] **Step 5: 验证编译和测试**
Run: `go test ./converter/ -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "ok"

- [ ] **Step 6: 提交**
Run: `git add converter/openai.go converter/response_converter.go && git commit -m "fix(converter): complete stop reason mapping for all provider finish reasons"`

---

### Task 4: Reasoning/Thinking Block Hardening

**Depends on:** Task 2
**Files:**
- Modify: `converter/openai.go:628-631`（增强 thinking block 提取）
- Modify: `converter/openai.go:62-67`（增强 max_tokens/reasoning_effort 处理）
- Modify: `converter/openai.go:38-44`（支持 thinking_mode 参数）

**Why:** nielspeter/claude-code-proxy 发现 OpenAI 返回 `reasoning_details` 数组（含 reasoning.text、reasoning.summary、reasoning.encrypted），需要完整映射为 Claude 的 thinking content blocks。Rust 实现还展示了 budget_tokens → effort level 的映射逻辑。

- [ ] **Step 1: 增强 thinking block 提取 — 支持 reasoning_details 数组**

文件: `converter/openai.go:628-631`（替换 thinking block 提取逻辑）

在现有的 thinking block 处理（约 line 628-631），增强以支持 OpenAI `reasoning_details` 数组。在当前代码后面添加处理逻辑：

```go
// 在现有 thinkingParts 收集逻辑之后，约 line 631 之后添加:

// Handle reasoning_details array (OpenAI GPT-5.x / o1-pro format)
if reasoningDetails, ok := message.ReasoningDetails.([]interface{}); ok {
	for _, detail := range reasoningDetails {
		if detailMap, ok := detail.(map[string]interface{}); ok {
			if text, ok := detailMap["text"].(string); ok && text != "" {
				thinkingParts = append(thinkingParts, text)
			} else if summary, ok := detailMap["summary"].(string); ok && summary != "" {
				thinkingParts = append(thinkingParts, summary)
			}
			// Skip reasoning.encrypted type — cannot display
		}
	}
}
```

注意：这需要 `models.OpenAIMessage` 有 `ReasoningDetails` 字段。如果没有，需要在 `models/` 包中添加。

- [ ] **Step 2: 增强 reasoning_effort 参数处理 — 支持 budget_tokens 转换**

文件: `converter/openai.go:62-67`（在 max_tokens 处理区域之后添加 reasoning_effort 映射）

在现有的 reasoning_effort 透传之后，添加 budget_tokens → effort level 映射（参考 Rust 实现）：

```go
// 在 converter/openai.go ParseRequest 中，reasoning_effort 处理区域添加:
// Support thinking.budget_tokens → reasoning_effort conversion
if thinking, ok := raw["thinking"].(map[string]interface{}); ok {
	if budgetTokens, ok := thinking["budget_tokens"].(float64); ok {
		bt := int(budgetTokens)
		switch {
		case bt <= 2048:
			req.ReasoningEffort = ptrString("low")
		case bt <= 8192:
			req.ReasoningEffort = ptrString("medium")
		case bt <= 32768:
			req.ReasoningEffort = ptrString("high")
		default:
			req.ReasoningEffort = ptrString("xhigh")
		}
	}
}
```

需要在文件顶部添加辅助函数：

```go
func ptrString(s string) *string { return &s }
```

- [ ] **Step 3: 验证编译**
Run: `go build ./converter/ 2>&1 | head -5`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add converter/openai.go && git commit -m "feat(converter): harden reasoning/thinking block support with reasoning_details and budget_tokens mapping"`

---

### Task 5: Provider-Specific Protocol Quirks

**Depends on:** Task 3
**Files:**
- Modify: `converter/openai.go`（新增 Azure OpenAI 检测和兼容函数）
- Modify: `converter/normalizer.go`（新增 stop sequence 标准化）
- Create: `converter/provider_quirks.go`（提供商特定兼容层）
- Create: `converter/provider_quirks_test.go`

**Why:** Azure OpenAI 使用 `api-key` header 而非 `Authorization: Bearer`；不同提供商对 `max_tokens` vs `max_completion_tokens` 的支持不同；某些提供商不支持 `stop_sequences`。这些差异会导致请求失败。

- [ ] **Step 1: 创建 provider_quirks.go — 提供商特定兼容层**

文件: `converter/provider_quirks.go`

```go
package converter

import "strings"

// ProviderType identifies the upstream API provider.
type ProviderType int

const (
	ProviderOpenAI      ProviderType = iota
	ProviderAzureOpenAI
	ProviderDeepSeek
	ProviderOllama
	ProviderOpenRouter
	ProviderMistral
	ProviderUnknown
)

// DetectProvider determines the provider type from the base URL.
func DetectProvider(baseURL string) ProviderType {
	lower := strings.ToLower(baseURL)
	switch {
	case strings.Contains(lower, "azure.com") || strings.Contains(lower, "openai.azure"):
		return ProviderAzureOpenAI
	case strings.Contains(lower, "deepseek"):
		return ProviderDeepSeek
	case strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1"):
		return ProviderOllama
	case strings.Contains(lower, "openrouter"):
		return ProviderOpenRouter
	case strings.Contains(lower, "mistral"):
		return ProviderMistral
	case strings.Contains(lower, "openai.com") || strings.Contains(lower, "api.openai"):
		return ProviderOpenAI
	default:
		return ProviderUnknown
	}
}

// SupportsFunctionCalling returns true if the provider supports native function calling.
func SupportsFunctionCalling(provider ProviderType) bool {
	switch provider {
	case ProviderOllama:
		return false
	default:
		return true
	}
}

// SupportsStopSequences returns true if the provider supports stop sequences.
func SupportsStopSequences(provider ProviderType) bool {
	switch provider {
	case ProviderAzureOpenAI:
		return true
	case ProviderDeepSeek:
		return true
	case ProviderOllama:
		return false
	default:
		return true
	}
}

// SupportsMaxTokens returns the correct field name for token limit.
// Returns "max_completion_tokens" for newer OpenAI models, "max_tokens" otherwise.
func SupportsMaxTokens(provider ProviderType, model string) string {
	if provider == ProviderOpenAI || provider == ProviderAzureOpenAI {
		lower := strings.ToLower(model)
		if strings.Contains(lower, "o1") || strings.Contains(lower, "o3") ||
			strings.Contains(lower, "gpt-5") || strings.Contains(lower, "gpt-4.1") {
			return "max_completion_tokens"
		}
	}
	return "max_tokens"
}

// SupportsStreaming returns true if the provider supports SSE streaming.
func SupportsStreaming(provider ProviderType) bool {
	// All known providers support streaming
	return true
}

// GetAuthHeader returns the auth header name and format for the provider.
func GetAuthHeader(provider ProviderType) (headerName string, format string) {
	switch provider {
	case ProviderAzureOpenAI:
		return "api-key", "%s"
	default:
		return "Authorization", "Bearer %s"
	}
}
```

- [ ] **Step 2: 创建 provider_quirks_test.go**

文件: `converter/provider_quirks_test.go`

```go
package converter

import "testing"

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		url      string
		expected ProviderType
	}{
		{"https://api.openai.com/v1", ProviderOpenAI},
		{"https://myresource.openai.azure.com/openai/", ProviderAzureOpenAI},
		{"https://api.deepseek.com/v1", ProviderDeepSeek},
		{"http://localhost:11434/v1", ProviderOllama},
		{"https://openrouter.ai/api/v1", ProviderOpenRouter},
		{"https://api.mistral.ai/v1", ProviderMistral},
		{"https://unknown.example.com/v1", ProviderUnknown},
	}
	for _, tt := range tests {
		if got := DetectProvider(tt.url); got != tt.expected {
			t.Errorf("DetectProvider(%q) = %v, want %v", tt.url, got, tt.expected)
		}
	}
}

func TestSupportsFunctionCalling(t *testing.T) {
	if !SupportsFunctionCalling(ProviderOpenAI) {
		t.Error("OpenAI should support function calling")
	}
	if SupportsFunctionCalling(ProviderOllama) {
		t.Error("Ollama should not support function calling")
	}
}

func TestSupportsMaxTokens(t *testing.T) {
	if SupportsMaxTokens(ProviderOpenAI, "o1-preview") != "max_completion_tokens" {
		t.Error("o1 should use max_completion_tokens")
	}
	if SupportsMaxTokens(ProviderOpenAI, "gpt-4o") != "max_tokens" {
		t.Error("gpt-4o should use max_tokens")
	}
	if SupportsMaxTokens(ProviderDeepSeek, "deepseek-chat") != "max_tokens" {
		t.Error("DeepSeek should use max_tokens")
	}
}

func TestGetAuthHeader(t *testing.T) {
	header, format := GetAuthHeader(ProviderAzureOpenAI)
	if header != "api-key" || format != "%s" {
		t.Errorf("Azure should use api-key header, got %s/%s", header, format)
	}
	header, format = GetAuthHeader(ProviderOpenAI)
	if header != "Authorization" || format != "Bearer %s" {
		t.Errorf("OpenAI should use Authorization Bearer, got %s/%s", header, format)
	}
}

func TestSupportsStopSequences(t *testing.T) {
	if !SupportsStopSequences(ProviderOpenAI) {
		t.Error("OpenAI should support stop sequences")
	}
	if SupportsStopSequences(ProviderOllama) {
		t.Error("Ollama should not support stop sequences")
	}
}
```

- [ ] **Step 3: 验证 provider quirks**
Run: `go test ./converter/ -v -count=1 -run "TestDetect|TestSupports|TestGetAuth" 2>&1 | tail -15`
Expected:
  - Exit code: 0
  - Output contains: "PASS" for all tests

- [ ] **Step 4: 提交**
Run: `git add converter/provider_quirks.go converter/provider_quirks_test.go && git commit -m "feat(converter): add provider-specific protocol quirk detection layer"`

---

### Task 6: Integration, Full Test Suite, and Rebuild

**Depends on:** Task 1, Task 4, Task 5
**Files:**
- None (verification only)

- [ ] **Step 1: 运行全部 converter 包测试**
Run: `go test ./converter/ -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "ok"

- [ ] **Step 2: 运行全部 handler 包测试**
Run: `go test ./handler/ -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0

- [ ] **Step 3: 全量编译**
Run: `go build -tags dev -o claude-proxy-server . 2>&1 | head -5`
Expected:
  - Exit code: 0

- [ ] **Step 4: 重启后端服务**
Run: `lsof -i :54989 -t | xargs kill 2>/dev/null; sleep 1; ./claude-proxy-server server --port 54989 > /tmp/ccr-backend.log 2>&1 &`
Expected: Server starts on port 54989

- [ ] **Step 5: 提交**
Run: `git add -A && git commit -m "chore: rebuild with protocol compatibility hardening"`
