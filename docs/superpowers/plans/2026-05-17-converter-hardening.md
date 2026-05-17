# Converter Protocol Hardening — 参考claude-code-proxy改进

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 参考 claude-code-proxy 项目，对我们的 Claude↔OpenAI 协议转换层进行5项加固：Tool Result 降级兜底、Max Tokens 封顶、Gemini Schema 递归清理、Gemini Provider 识别、流式 Ping 保活增强。

**Architecture:** Claude Code CLI → /v1/messages → ClaudeConverter 解析请求 → OpenAIConverter 构建请求 → 上游 OpenAI/Gemini API → ResponseConverter 转回 Claude 格式 → CLI。本次改动集中在 Converter 层，不涉及 Handler 或 Database。

**Tech Stack:** Go 1.23, Gin, 自研 Converter (backend/converter/)

**Risks:**
- Task 1 修改 tool_result 处理逻辑，可能影响现有 tool 消息转换 → 缓解：仅在 tool message 转换失败时降级为文本，不影响正常路径
- Task 2 修改 max_tokens 逻辑 → 缓解：仅对非 Anthropic provider 生效，Anthropic 直连不受影响
- Task 4 扩展 provider_quirks → 缓解：新增枚举值和判断分支，不修改已有逻辑

---

### Task 1: Tool Result 降级兜底 — 当 OpenAI tool message 被拒绝时转为描述性文本

**Depends on:** None
**Files:**
- Modify: `backend/converter/openai.go:340-348` (tool_result → tool message 转换逻辑)

当 OpenAI 不接受 tool message（如 tool_call_id 不匹配、provider 不支持 function calling）时，将 tool_result 降级为 user 角色的描述性文本，保持对话连贯。

- [ ] **Step 1: 修改 BuildRequest 中 tool_result 处理逻辑 — 添加降级文本转换函数**

文件: `backend/converter/openai.go` (在文件末尾添加新函数)

```go
// toolResultToFallbackText 将 tool_result 转为描述性文本（参考 claude-code-proxy 模式）
// 当 provider 不支持 function calling 或 tool_call_id 不匹配时使用
func toolResultToFallbackText(tr ContentBlock) string {
	name := tr.Name
	if name == "" {
		name = tr.ToolUseID
	}
	content := tr.Content
	if content == "" {
		content = "(empty result)"
	}
	return fmt.Sprintf("[Tool Result for %s]: %s", name, content)
}
```

- [ ] **Step 2: 修改 BuildRequest 中 tool_result 转换 — 添加 provider 不支持 function calling 时的降级路径**

文件: `backend/converter/openai.go:340-348` (替换 tool_result 循环块)

```go
			// tool messages must immediately follow assistant(tool_calls)
			// OpenAI spec: assistant(tool_calls) -> tool -> tool -> user
			for _, tr := range toolResults {
				// 检测 provider 是否支持 function calling
				provider := DetectProvider(o.cfg.GetBaseURL())
				if !SupportsFunctionCalling(provider) {
					// Provider 不支持 function calling，降级为描述性文本
					fallbackMsg := models.OpenAIMessage{
						Role:    "user",
						Content: toolResultToFallbackText(tr),
					}
					openAIReq.Messages = append(openAIReq.Messages, fallbackMsg)
				} else {
					toolMsg := models.OpenAIMessage{
						Role:       "tool",
						ToolCallID: tr.ToolUseID,
						Content:    tr.Content,
					}
					openAIReq.Messages = append(openAIReq.Messages, toolMsg)
				}
			}
```

- [ ] **Step 3: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "cannot" or "undefined" or "not used"

- [ ] **Step 4: 提交**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/openai.go && git commit -m "feat(converter): add tool_result fallback to descriptive text for non-function-calling providers"`

---

### Task 2: Max Tokens 按 Provider 封顶 — 防止超出 OpenAI/Gemini 的 token 上限

**Depends on:** None
**Files:**
- Modify: `backend/converter/openai.go:290-293` (max_tokens 设置逻辑)

参考 claude-code-proxy 对 OpenAI 模型封顶 16384 tokens 的模式，在我们的 converter 中也添加按 provider 类型的封顶逻辑。

- [ ] **Step 1: 在 provider_quirks.go 中添加 GetMaxTokensCap 函数 — 返回各 provider 的 token 上限**

文件: `backend/converter/provider_quirks.go` (在文件末尾追加)

```go
// GetMaxTokensCap returns the maximum tokens allowed for the given provider.
// Returns 0 if no cap applies (e.g. Anthropic direct proxy).
func GetMaxTokensCap(provider ProviderType) int {
	switch provider {
	case ProviderOpenAI, ProviderAzureOpenAI:
		return 16384
	case ProviderDeepSeek:
		return 16384
	default:
		return 0 // no cap
	}
}
```

- [ ] **Step 2: 修改 BuildRequest 中 max_tokens 设置 — 应用封顶**

文件: `backend/converter/openai.go:290-293` (替换 max_tokens 设置块)

```go
		// Set max tokens with provider-specific capping (参考 claude-code-proxy)
		if req.MaxTokens > 0 {
			capTokens := req.MaxTokens
			if o.cfg != nil {
				provider := DetectProvider(o.cfg.GetBaseURL())
				if maxCap := GetMaxTokensCap(provider); maxCap > 0 && capTokens > maxCap {
					utils.GetLogger().Debug("[BuildRequest] Capping max_tokens %d -> %d for provider %v", capTokens, maxCap, provider)
					capTokens = maxCap
				}
			}
			openAIReq.MaxTokens = capTokens
		}
```

- [ ] **Step 3: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "cannot" or "undefined"

- [ ] **Step 4: 提交**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/openai.go backend/converter/provider_quirks.go && git commit -m "feat(converter): cap max_tokens per provider to prevent exceeding upstream limits"`

---

### Task 3: Gemini JSON Schema 递归清理 — 移除 Gemini 不支持的 schema 字段

**Depends on:** None
**Files:**
- Modify: `backend/converter/provider_quirks.go` (添加 schema 清理函数)
- Modify: `backend/converter/openai.go:364-377` (tools 构建时应用清理)

参考 claude-code-proxy 的 `clean_schema` 函数，在向 Gemini 发送请求前递归清理 JSON Schema 中不支持的字段（`additionalProperties`, `default`, 不支持的 `format` 等）。

- [ ] **Step 1: 在 provider_quirks.go 中添加 CleanSchemaForGemini 函数 — 递归清理不兼容字段**

文件: `backend/converter/provider_quirks.go` (在文件末尾追加)

```go
// geminiUnsupportedSchemaFields 列出 Gemini 不支持的 JSON Schema 字段
var geminiUnsupportedSchemaFields = []string{
	"additionalProperties",
	"default",
	"$schema",
	"deprecated",
	"examples",
}

// CleanSchemaForGemini 递归清理 JSON Schema 中 Gemini 不支持的字段
// 参考 claude-code-proxy 的 clean_schema 实现
func CleanSchemaForGemini(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	cleaned := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		// 跳过不支持的字段
		if isUnsupportedField(k) {
			continue
		}
		cleaned[k] = cleanSchemaValue(v)
	}
	// 如果有 properties，递归清理每个属性
	if props, ok := cleaned["properties"].(map[string]interface{}); ok {
		for propName, propVal := range props {
			if propMap, ok := propVal.(map[string]interface{}); ok {
				props[propName] = CleanSchemaForGemini(propMap)
			}
		}
	}
	// 如果有 items（数组类型），递归清理
	if items, ok := cleaned["items"].(map[string]interface{}); ok {
		cleaned["items"] = CleanSchemaForGemini(items)
	}
	// 如果有 anyOf/allOf/oneOf，递归清理每个子 schema
	for _, key := range []string{"anyOf", "allOf", "oneOf"} {
		if arr, ok := cleaned[key].([]interface{}); ok {
			for i, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arr[i] = CleanSchemaForGemini(itemMap)
				}
			}
		}
	}
	return cleaned
}

func isUnsupportedField(field string) bool {
	for _, f := range geminiUnsupportedSchemaFields {
		if field == f {
			return true
		}
	}
	return false
}

func cleanSchemaValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return CleanSchemaForGemini(val)
	case []interface{}:
		cleaned := make([]interface{}, len(val))
		for i, item := range val {
			cleaned[i] = cleanSchemaValue(item)
		}
		return cleaned
	default:
		return v
	}
}
```

- [ ] **Step 2: 修改 BuildRequest 中 tools 构建 — 对 Gemini provider 应用 schema 清理**

文件: `backend/converter/openai.go:364-377` (替换 tools 构建块)

```go
		// 构建 tools
		isGemini := false
		if o.cfg != nil {
			isGemini = IsGeminiProvider(o.cfg.GetBaseURL())
		}
		for _, tool := range req.Tools {
			toolName := truncateToolName(tool.Name)
			if toolName != tool.Name {
				utils.GetLogger().Debug("[BuildRequest] Tool name truncated: %q -> %q", tool.Name, toolName)
			}
			parameters := tool.Parameters
			if isGemini && parameters != nil {
				if paramMap, ok := parameters.(map[string]interface{}); ok {
					parameters = CleanSchemaForGemini(paramMap)
				}
			}
			openAIReq.Tools = append(openAIReq.Tools, models.OpenAITool{
				Type: "function",
				Function: models.OpenAIFunction{
					Name:        toolName,
					Description: tool.Description,
					Parameters:  parameters,
				},
			})
		}
```

- [ ] **Step 3: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "cannot" or "undefined"

- [ ] **Step 4: 提交**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/provider_quirks.go backend/converter/openai.go && git commit -m "feat(converter): add recursive Gemini JSON Schema cleaning for tool parameters"`

---

### Task 4: Gemini Provider 识别与扩展 — 完善 provider 检测覆盖

**Depends on:** None
**Files:**
- Modify: `backend/converter/provider_quirks.go:6-37` (添加 Gemini 枚举值和检测)

当前 provider_quirks.go 缺少 Gemini 和 Google 的 provider 检测，导致无法识别 Gemini 请求。

- [ ] **Step 1: 在 provider 枚举中添加 ProviderGemini 和 ProviderGoogle**

文件: `backend/converter/provider_quirks.go:6-16` (替换枚举定义块)

```go
const (
	ProviderOpenAI      ProviderType = iota
	ProviderAzureOpenAI
	ProviderDeepSeek
	ProviderOllama
	ProviderOpenRouter
	ProviderMistral
	ProviderGemini
	ProviderGoogle
	ProviderAnthropic
	ProviderUnknown
)
```

- [ ] **Step 2: 在 DetectProvider 中添加 Gemini/Google/Anthropic 检测分支**

文件: `backend/converter/provider_quirks.go:18-37` (替换整个 DetectProvider 函数)

```go
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
	case strings.Contains(lower, "generativelanguage.googleapis.com") || strings.Contains(lower, "gemini"):
		return ProviderGemini
	case strings.Contains(lower, "googleapis.com"):
		return ProviderGoogle
	case strings.Contains(lower, "anthropic.com") || strings.Contains(lower, "api.anthropic"):
		return ProviderAnthropic
	case strings.Contains(lower, "openai.com") || strings.Contains(lower, "api.openai"):
		return ProviderOpenAI
	default:
		return ProviderUnknown
	}
}
```

- [ ] **Step 3: 添加 IsGeminiProvider 便捷函数 — 供其他模块判断是否为 Gemini**

文件: `backend/converter/provider_quirks.go` (在 GetAuthHeader 函数之后追加)

```go
// IsGeminiProvider returns true if the base URL points to a Gemini/Google AI endpoint.
func IsGeminiProvider(baseURL string) bool {
	return DetectProvider(baseURL) == ProviderGemini || DetectProvider(baseURL) == ProviderGoogle
}

// IsAnthropicProvider returns true if the base URL points to Anthropic directly.
func IsAnthropicProvider(baseURL string) bool {
	return DetectProvider(baseURL) == ProviderAnthropic
}
```

- [ ] **Step 4: 更新 SupportsFunctionCalling — Gemini 支持 function calling**

文件: `backend/converter/provider_quirks.go` (替换 SupportsFunctionCalling 函数)

```go
// SupportsFunctionCalling returns true if the provider supports native function calling.
func SupportsFunctionCalling(provider ProviderType) bool {
	switch provider {
	case ProviderOllama:
		return false
	default:
		return true
	}
}
```

- [ ] **Step 5: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "cannot" or "undefined"

- [ ] **Step 6: 提交**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/provider_quirks.go && git commit -m "feat(converter): add Gemini/Google/Anthropic provider detection with helper functions"`

---

### Task 5: 流式 Ping 保活增强 — 确保长时间 tool call 期间连接不断开

**Depends on:** None
**Files:**
- Modify: `backend/converter/heartbeat.go` (检查现有 heartbeat 实现)
- Modify: `backend/converter/response_converter.go` (在流式转换中集成 ping)

参考 claude-code-proxy 在流式响应中定期发送 `event: ping` 保活的模式，确保 Claude Code CLI 在等待长时间 tool call 结果时不会因超时断开。

- [ ] **Step 1: 检查现有 heartbeat.go 实现，确认是否需要修改**

Run: `wc -l /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend/converter/heartbeat.go`
Expected:
  - Exit code: 0
  - Output shows line count

- [ ] **Step 2: 在 heartbeat.go 中添加 WritePingEvent 函数 — 发送 Anthropic 格式的 ping 事件**

文件: `backend/converter/heartbeat.go` (追加 ping 事件函数)

```go
// WritePingEvent sends an Anthropic-compatible SSE ping event to keep the connection alive.
// Format: event: ping\ndata: {"type": "ping"}\n\n
func WritePingEvent(w io.Writer) error {
	_, err := fmt.Fprintf(w, "event: ping\ndata: {\"type\": \"ping\"}\n\n")
	if err != nil {
		return fmt.Errorf("failed to write ping event: %w", err)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
```

- [ ] **Step 3: 在 response_converter.go 流式循环中集成定期 ping — 每 15 秒发送一次**

文件: `backend/converter/response_converter.go` (在 `ConvertOpenAIStreamingToClaudeWithMapping` 函数的流式循环中，找到 scanner 扫描循环，添加 ping 计时器)

在流式循环开始前添加:
```go
	lastPing := time.Now()
	pingInterval := 15 * time.Second
```

在 scanner 循环体内（处理每行 SSE 数据之后）添加:
```go
				// Send periodic ping events to keep connection alive
				if time.Since(lastPing) >= pingInterval {
					if err := WritePingEvent(writer); err != nil {
						utils.GetLogger().Error("[Stream] Ping write failed: %v", err)
						return
					}
					lastPing = time.Now()
				}
```

- [ ] **Step 4: 验证编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "cannot" or "undefined"

- [ ] **Step 5: 提交**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/converter/heartbeat.go backend/converter/response_converter.go && git commit -m "feat(converter): add periodic SSE ping events to prevent connection timeout during streaming"`
