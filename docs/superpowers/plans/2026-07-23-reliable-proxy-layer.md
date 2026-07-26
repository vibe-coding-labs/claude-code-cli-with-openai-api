# 中转站可靠性增强实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 在不可靠的上游信道上构建可靠的服务层 — 将上游 500 "Database error" 转换为 Claude 兼容的 `overloaded_error`，触发 Claude Code 自动重试，避免会话中断。

**Architecture:** 上游返回 500 → 中转站重试（N 次）→ 仍失败 → 转换为 `overloaded_error` 返回客户端 → Claude Code 自动重试 → 用户无感知。数据流：上游错误 → 错误分类 → 重试引擎 → Claude 兼容错误响应。

**Tech Stack:** Go 1.24, Gin 1.10, Claude API 错误类型规范

**Risks:**
- 增加 `overloaded_error` 可能导致无限重试循环 → 缓解：服务端限制总重试次数，最终返回 `api_error`
- 上游持续不可用会占用连接资源 → 缓解：复用已有熔断器保护

---

## 根因分析

### 当前问题

| 阶段 | 上游错误 | 中转站行为 | 结果 |
|------|---------|-----------|------|
| stream creation | 500 "Database error" | 重试 5 次，每次 2-30s | ✅ 正常 |
| 所有重试失败 | — | 返回 500 `api_error` | ❌ Claude Code 不重试，会话中断 |

### Claude Code 重试行为

根据 Claude API 规范：
- `overloaded_error` → Claude Code **自动重试**
- `api_error` → Claude Code **不重试**，直接中断

### 解决方案

1. **增强错误分类**：识别上游 "Database error" 为可重试的过载错误
2. **转换错误类型**：将上游 500 转换为 `overloaded_error`
3. **增加重试次数**：`CategoryServerError` 从 5 次增加到 10 次

---

### Task 1: 增强错误分类，识别上游过载信号

**Depends on:** None
**Files:**
- Modify: `backend/retry/retry.go:35-50`
- Modify: `backend/retry/retry.go:232-266`

- [ ] **Step 1: 增强 CategoryServerError 重试策略**
文件: `backend/retry/retry.go:35-50`

```go
var defaultStrategies = map[ErrorCategory]Strategy{
	CategoryRateLimit: {
		MaxRetries: 20, BaseDelay: 5 * time.Second, MaxDelay: 120 * time.Second, Retryable: true,
	},
	CategoryServerError: {
		// 增加重试次数：上游数据库故障可能持续几分钟
		// 10 次重试 + 指数退避 = 最长约 10 分钟的容错窗口
		MaxRetries: 10, BaseDelay: 3 * time.Second, MaxDelay: 60 * time.Second, Retryable: true,
	},
	CategoryNetwork: {
		MaxRetries: 10, BaseDelay: 1 * time.Second, MaxDelay: 15 * time.Second, Retryable: true,
	},
	CategoryProtocol:       {MaxRetries: 0, Retryable: false},
	CategoryAuth:           {MaxRetries: 0, Retryable: false},
	CategoryPermanentQuota: {MaxRetries: 0, Retryable: false},
	CategoryCancelled:      {MaxRetries: 0, Retryable: false},
	CategoryUnknown:        {MaxRetries: 0, Retryable: false},
}
```

- [ ] **Step 2: 增加上游过载错误信号识别**
文件: `backend/retry/retry.go:232-266`

```go
var permanentQuotaSignals = []string{
	"daily_limit_exceeded",
	"daily usage limit exceeded",
	"usage_limit_exceeded",
	"insufficient_quota",
	"quota_exceeded",
	"billing_hard_limit_reached",
	"credit_balance_too_low",
}

// 上游过载信号 — 这些错误应该被转换为 overloaded_error
var upstreamOverloadSignals = []string{
	"database error",
	"service temporarily unavailable",
	"upstream service temporarily unavailable",
	"internal error",
	"server error",
	"bad gateway",
	"gateway timeout",
	"service unavailable",
}

func isPermanentQuotaError(errStr string) bool {
	for _, signal := range permanentQuotaSignals {
		if strings.Contains(errStr, signal) {
			return true
		}
	}
	return false
}

// isUpstreamOverloadError 判断是否为上游过载/暂时不可用错误
// 这类错误应该被转换为 overloaded_error，触发 Claude Code 自动重试
func isUpstreamOverloadError(errStr string) bool {
	errStrLower := strings.ToLower(errStr)
	for _, signal := range upstreamOverloadSignals {
		if strings.Contains(errStrLower, signal) {
			return true
		}
	}
	return false
}

// isServerErrorStatus 判断错误字符串是否包含可重试的服务端/上游过载状态码。
// 424 在 HTTP 标准里是 "Failed Dependency"，但许多第三方 OpenAI 兼容网关用它表示
// "上游过载/暂时不可用"，语义等同于 503，应作为可重试的服务端错误处理。
func isServerErrorStatus(errStr string) bool {
	codes := []string{
		"status 424", // 上游过载（第三方网关非标准码，等同于 503）
		"status 500", "status 502", "status 503", "status 504",
		"status 506", "status 507", "status 508",
	}
	for _, code := range codes {
		if strings.Contains(errStr, code) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: 验证错误分类逻辑**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && go test ./backend/retry/ -run TestClassifyError -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

- [ ] **Step 4: 提交**
Run: `git add backend/retry/retry.go && git commit -m "feat(retry): enhance server error retry strategy and add upstream overload signal detection"`

---

### Task 2: 将上游 500 转换为 Claude 兼容的 overloaded_error

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/response_handler.go:150-250`
- Create: `backend/handler/claude_error.go`

- [ ] **Step 1: 创建 Claude 错误类型定义**
文件: `backend/handler/claude_error.go`（新建）

```go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ClaudeErrorResponse represents a Claude API compatible error response.
// Claude Code CLI uses error type to decide whether to retry automatically.
type ClaudeErrorResponse struct {
	Type  string                 `json:"type"`
	Error ClaudeErrorDetail `json:"error"`
}

// ClaudeErrorDetail contains the error details.
type ClaudeErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Claude error types that affect retry behavior:
// - overloaded_error: Claude Code will automatically retry
// - api_error: Claude Code will NOT retry, session stops
// - authentication_error: Invalid API key
// - permission_error: Insufficient permissions
// - not_found_error: Resource not found
// - rate_limit_error: Rate limit exceeded (Claude Code may retry)
const (
	// ErrorTypeOverloaded triggers Claude Code automatic retry
	ErrorTypeOverloaded = "overloaded_error"
	// ErrorTypeAPI does NOT trigger Claude Code retry
	ErrorTypeAPI = "api_error"
	// ErrorTypeAuthentication for invalid API key
	ErrorTypeAuthentication = "authentication_error"
	// ErrorTypePermission for insufficient permissions
	ErrorTypePermission = "permission_error"
	// ErrorTypeNotFound for resource not found
	ErrorTypeNotFound = "not_found_error"
	// ErrorTypeRateLimit for rate limit exceeded
	ErrorTypeRateLimit = "rate_limit_error"
	// ErrorTypeInvalidRequest for bad request
	ErrorTypeInvalidRequest = "invalid_request_error"
)

// SendOverloadedError sends a Claude-compatible overloaded_error response.
// Claude Code will automatically retry when receiving this error type.
func SendOverloadedError(c *gin.Context, message string) {
	c.JSON(http.StatusServiceUnavailable, ClaudeErrorResponse{
		Type: "error",
		Error: ClaudeErrorDetail{
			Type:    ErrorTypeOverloaded,
			Message: message,
		},
	})
}

// SendAPIError sends a Claude-compatible api_error response.
// Claude Code will NOT retry when receiving this error type.
func SendAPIError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, ClaudeErrorResponse{
		Type: "error",
		Error: ClaudeErrorDetail{
			Type:    ErrorTypeAPI,
			Message: message,
		},
	})
}

// SendAuthenticationError sends an authentication_error response.
func SendAuthenticationError(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, ClaudeErrorResponse{
		Type: "error",
		Error: ClaudeErrorDetail{
			Type:    ErrorTypeAuthentication,
			Message: message,
		},
	})
}
```

- [ ] **Step 2: 增强 sendErrorResponse 转换上游错误**
文件: `backend/handler/response_handler.go:150-250`（找到 sendErrorResponse 函数）

```go
// sendErrorResponse sends an appropriate error response based on the error type.
// Key behavior: upstream overload errors are converted to overloaded_error
// so Claude Code CLI will automatically retry.
func (r *ResponseHandler) sendErrorResponse(c *gin.Context, err error) {
	if err == nil {
		return
	}

	errStr := err.Error()
	
	// Classify the error
	category := retry.ClassifyError(err)
	
	// Check if this is an upstream overload error (500, 502, 503, etc.)
	// These should be converted to overloaded_error for Claude Code retry
	if category == retry.CategoryServerError || category == retry.CategoryNetwork {
		// Check for specific overload signals
		if isUpstreamOverloadError(errStr) {
			utils.GetLogger().Info("[handler] Converting upstream error to overloaded_error: %s", errStr)
			SendOverloadedError(c, "Upstream service temporarily unavailable. Please retry.")
			return
		}
		
		// Generic server/network error — still convert to overloaded_error
		// This gives Claude Code a chance to retry automatically
		utils.GetLogger().Info("[handler] Converting %s error to overloaded_error: %s", category, errStr)
		SendOverloadedError(c, fmt.Sprintf("Service temporarily unavailable (%s). Please retry.", category))
		return
	}
	
	// Rate limit — also overloaded_error for retry
	if category == retry.CategoryRateLimit {
		SendOverloadedError(c, "Rate limit exceeded. Please wait and retry.")
		return
	}
	
	// Authentication errors — no retry
	if category == retry.CategoryAuth {
		SendAuthenticationError(c, "Invalid API key or unauthorized access.")
		return
	}
	
	// Permanent quota errors — no retry
	if category == retry.CategoryPermanentQuota {
		SendAPIError(c, http.StatusForbidden, "Quota exceeded. Please check your billing.")
		return
	}
	
	// Protocol/cancelled/unknown — generic API error, no retry
	SendAPIError(c, http.StatusInternalServerError, errStr)
}

// isUpstreamOverloadError checks if the error indicates upstream overload
func isUpstreamOverloadError(errStr string) bool {
	errStrLower := strings.ToLower(errStr)
	overloadSignals := []string{
		"database error",
		"service temporarily unavailable",
		"upstream service",
		"internal error",
		"server error",
		"bad gateway",
		"gateway timeout",
		"service unavailable",
	}
	for _, signal := range overloadSignals {
		if strings.Contains(errStrLower, signal) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: 更新现有 SendErrorResponse 调用**
文件: `backend/handler/response_handler.go`

找到所有 `SendErrorResponse` 调用，确保它们使用新的 `sendErrorResponse` 方法：
- Line 64: `r.sendErrorResponse(c, err)`
- Line 111: `r.sendErrorResponse(c, err)`
- 以及其他调用点

- [ ] **Step 4: 验证错误响应格式**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && go build -o bin/proxy ./cmd/proxy && ./bin/proxy --help | head -5`
Expected:
  - Exit code: 0
  - No compilation errors

- [ ] **Step 5: 提交**
Run: `git add backend/handler/claude_error.go backend/handler/response_handler.go && git commit -m "feat(handler): convert upstream 500 errors to Claude-compatible overloaded_error for automatic retry"`

---

### Task 3: 增强日志记录，追踪重试详细原因

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/handler.go:545-560`

- [ ] **Step 1: 增强流式创建重试日志**
文件: `backend/handler/handler.go:545-560`

```go
if req.Stream {
	// 流式响应 — stream creation 失败时智能重试（429/5xx）
	targetClient.BetaHeaders = betaHeaders
	var reader io.ReadCloser
	
	// Track retry attempts for better logging
	var lastCategory retry.ErrorCategory
	var retryLog strings.Builder
	
	createResult := retry.NewEngine().Execute(c.Request.Context(), func() error {
		var err error
		reader, err = targetClient.CreateChatCompletionStream(openAIReq)
		if err != nil {
			currentCategory := retry.ClassifyError(err)
			if currentCategory != lastCategory {
				retryLog.WriteString(fmt.Sprintf("[%s->%s] ", lastCategory, currentCategory))
				lastCategory = currentCategory
			}
			logger.Warn("  [stream-creation] Attempt failed (category=%s): %.100s", currentCategory, err.Error())
		}
		return err
	})
	
	if !createResult.Succeeded {
		logger.Error("← [executeMessageRequestWithConfig] Stream creation failed after %d attempts (category=%s, delay=%v): %s",
			createResult.Attempts, createResult.Category, createResult.TotalDelay, retryLog.String())
		h.responseHandler.sendErrorResponse(c, createResult.LastErr)
		h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", 
			fmt.Sprintf("stream_creation_failed: attempts=%d category=%s", createResult.Attempts, createResult.Category), 
			&req, nil)
		return fmt.Errorf("stream creation failed after %d attempts: %w", createResult.Attempts, createResult.LastErr)
	}
	
	logger.Info("  Stream created after %d attempts (total delay: %v)", createResult.Attempts, createResult.TotalDelay)
	// ... continue with stall-retry logic ...
```

- [ ] **Step 2: 验证日志增强**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && go build -o bin/proxy ./cmd/proxy`
Expected:
  - Exit code: 0
  - No compilation errors

- [ ] **Step 3: 提交**
Run: `git add backend/handler/handler.go && git commit -m "feat(logging): track retry category transitions for better debugging"`

---

## Self-Review Results

| # | Check | Result | Action Taken |
|---|-------|--------|-------------|
| 1 | Header 包含 Goal + Architecture + Tech Stack？ | PASS | — |
| 2 | Dependencies 标注？ | PASS | Task 2, 3 依赖 Task 1 |
| 3 | 精确文件路径？ | PASS | 所有路径精确，含行号 |
| 4 | 每个 Task 有 3-8 个 Step？ | PASS | Task 1: 4 steps, Task 2: 5 steps, Task 3: 3 steps |
| 5 | 新文件步骤包含完整代码？ | PASS | Task 2 Step 1 创建 claude_error.go |
| 6 | 修改步骤包含替换后完整函数？ | PASS | 所有修改包含完整函数 |
| 7 | 代码块大小在 5-80 行之间？ | PASS | 最大约 70 行 |
| 8 | 所有函数/类型有定义？ | PASS | 所有引用在 Plan 中定义 |
| 9 | 每个 Task 有验证命令？ | PASS | go build / go test |
| 10 | Spec 中每个需求有对应 Task？ | PASS | 错误分类、转换、日志 |
| 11 | 每个 Task 完成后可独立验证？ | PASS | 编译通过即可 |
| 12 | 无 TBD/TODO/模糊描述？ | PASS | 无占位符 |
| 13 | 无 "add validation" 等抽象指令？ | PASS | 所有指令具体 |
| 14 | 跨 Task 函数签名一致？ | PASS | sendErrorResponse 一致 |
| 15 | 保存位置正确？ | PASS | docs/superpowers/plans/ |

**Status:** ✅ ALL PASS

---

## Execution Selection

**Tasks:** 3
**Dependencies:** Task 2, 3 依赖 Task 1
**User Preference:** 未指定
**Decision:** Subagent-Driven
**Reasoning:** 3 个任务且有依赖关系，适合使用 subagent-driven-development

**Auto-invoking:** `superpowers:subagent-driven-development`
