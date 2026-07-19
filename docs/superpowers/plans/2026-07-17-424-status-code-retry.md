# 424 状态码重试兼容修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 让代理对上游返回的 `424 Service temporarily unavailable` 等临时过载状态码执行重试(代理内部重试 + 兜底转 `overloaded_error`),使 Claude Code 客户端不再因该错误码直接中断。

## Root Cause（根因分析）

用户的直觉正确:这个代理工具的核心价值之一就是"把上游的临时错误消化掉,客户端无感"。但 `424` 在**三道防线全部漏网**,完整因果链如下:

```
上游返回 HTTP 424 "Service temporarily unavailable"
        │
        ▼
① client 层 isRetryableHTTPStatus(424) → false
   （openai_client.go:250-262 白名单只有 408/406/429/500-511/≥500，无 424）
   → client 不重试，直接返回 error："OpenAI API error (status 424): Service temporarily unavailable"
        │
        ▼
② handler 层 retry.NewEngine().Execute（handler.go:528）
   → retry.ClassifyError 把 "status 424" 判为 CategoryUnknown
   （retry.go:251-259 isServerErrorStatus 只认 500/502/503/504/506/507/508，无 424）
   → engine 不换 config 节点重试
        │
        ▼
③ response_handler.SendErrorResponse（response_handler.go:347-432）
   → 提取状态码 424，既不是 model routing 也不是 timeout
   → 走默认分支：c.JSON(424, {type: "api_error", ...})  ← 不是 overloaded_error
        │
        ▼
④ Claude Code 客户端收到 424 + api_error
   （客户端只对 overloaded_error / 429 / 5xx 自动重试）
   → 不重试 → 中断报错："API Error: 424 Service temporarily unavailable"
```

**为什么 424 漏网:** 424 在 HTTP 标准里是 "Failed Dependency"(WebDAV),但许多第三方 OpenAI 兼容网关/聚合中转(以及部分上游 LLM 服务)用 `424` 表示"上游过载/暂时不可用"。代理的判断逻辑按"标准 5xx 才是过载"设计,424 这个 4xx 段的过载码被当成不可重试的客户端错误处理了。

**修复策略(三层纵深防御):**
- **Task 1:** retry 包识别 `status 424` 为可重试服务端错误 → 让 `retry.Engine` 换节点重试生效(防线②)
- **Task 2:** client 层 `isRetryableHTTPStatus` 加入 424 → 让 client 逐 attempt 重试覆盖 424(防线①)
- **Task 3:** response_handler 把 424 兜底转成 `overloaded_error`+503 → 代理重试全耗尽时,客户端仍能自己退避重试,不中断(防线③)
- **Task 4:** 同步死代码保持一致 + 全量回归 + 重建二进制

**Architecture:** 上游 424 错误 → client 层白名单拦截重试(①)→ 耗尽则 error 传到 handler → retry engine 按 CategoryServerError 换节点重试(②)→ 仍失败则 SendErrorResponse 转 overloaded_error 返回(③)→ Claude 客户端识别 overloaded_error 自动重试。三层任一层命中即消化掉错误。

**Tech Stack:** Go 1.x, gin-gonic, net/http, testing 标准库

**Risks:**
- `424` 是非标准过载码,不同上游语义可能不同 → 缓解:仅在错误消息含 "temporarily unavailable"/"overload" 这类临时性语义,或状态码本身就是 424 时才重试,避免把真正的 Failed Dependency(如 WebDAV 依赖失败)也重试。但 LLM 网关场景下 424 几乎都是过载,重试是安全默认。
- Task 3 改动 `SendErrorResponse` 是所有错误响应的必经路径 → 缓解:新增分支只针对 424/529,在现有 model routing / timeout 检测之后、默认分支之前插入,不影响其他状态码;并补充正反对照单测(424→overloaded,401→保持 401)。
- Task 4 触及 `retry_handler.go` 的 exported 死代码 `IsRetryableHTTPStatus` → 缓解:仅扩展白名单,不改签名,零行为风险。
- 用户线上实例(如 54988 端口)可能跑旧二进制 → 缓解:Task 4 强制 `go build` 重建,执行后需替换线上二进制并重启才生效(见 [[st-cc-proxy-binary-stale]] 的相关教训)。

---

### Task 1: retry 包识别 424 为可重试服务端错误

**Depends on:** None
**Files:**
- Modify: `backend/retry/retry.go:251-259`(`isServerErrorStatus` 函数)
- Modify: `backend/retry/retry_test.go:35-43`(`TestClassifyError_ServerError`)
- Modify: `backend/retry/retry_test.go:222-241`(`TestIsRetryable`)

- [ ] **Step 1: 修改 isServerErrorStatus 以识别 status 424**

文件: `backend/retry/retry.go:251-259`(替换整个 `isServerErrorStatus` 函数)

```go
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

说明:client 返回的错误字符串是 `"OpenAI API error (status 424): ..."`，包含子串 `"status 424"`，会被命中并归为 `CategoryServerError`，从而使 `retry.IsRetryable` 返回 true、`retry.Engine` 换节点重试。

- [ ] **Step 2: 扩展 TestClassifyError_ServerError 覆盖 424**

文件: `backend/retry/retry_test.go:35-43`(替换整个 `TestClassifyError_ServerError` 函数)

```go
func TestClassifyError_ServerError(t *testing.T) {
	codes := []string{"424", "500", "502", "503", "504"}
	for _, code := range codes {
		err := fmt.Errorf("status %s error", code)
		if got := ClassifyError(err); got != CategoryServerError {
			t.Errorf("status %s: got %v, want CategoryServerError", code, got)
		}
	}
}
```

- [ ] **Step 3: 扩展 TestIsRetryable 增加 424 用例**

文件: `backend/retry/retry_test.go:222-241`(替换整个 `TestIsRetryable` 函数)

```go
func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
		{fmt.Errorf("status 424: service temporarily unavailable"), true},
		{fmt.Errorf("status 429: rate limit"), true},
		{fmt.Errorf("status 500: internal error"), true},
		{fmt.Errorf("connection refused"), true},
		{fmt.Errorf("status 400: bad request"), false},
		{fmt.Errorf("status 401: unauthorized"), false},
		{fmt.Errorf("insufficient_quota exceeded"), false},
		{context.Canceled, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := IsRetryable(tt.err); got != tt.retryable {
			t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
		}
	}
}
```

- [ ] **Step 4: 验证 retry 包**
Run: `cd backend && go test ./retry/... -run 'TestClassifyError_ServerError|TestIsRetryable' -v`
Expected:
  - Exit code: 0
  - Output contains: "ok" 且无 "FAIL"
  - Output does NOT contain: "want CategoryServerError" 或 "want true" 的失败行

- [ ] **Step 5: 提交**
Run: `git add backend/retry/retry.go backend/retry/retry_test.go && git commit -m "fix(retry): classify HTTP 424 upstream overload as retryable server error"`

---

### Task 2: client 层 isRetryableHTTPStatus 加入 424

**Depends on:** Task 1
**Files:**
- Modify: `backend/client/openai_client.go:250-262`(`isRetryableHTTPStatus`)
- Create: `backend/client/openai_client_retry_test.go`

- [ ] **Step 1: 修改 isRetryableHTTPStatus 以重试 424**

文件: `backend/client/openai_client.go:250-262`(替换整个 `isRetryableHTTPStatus` 函数)

```go
func isRetryableHTTPStatus(statusCode int, errorBody string) bool {
	if statusCode == 429 {
		cat := retry.ClassifyError(fmt.Errorf("status 429: %s", errorBody))
		return cat == retry.CategoryRateLimit
	}

	switch statusCode {
	case 424, // 上游过载（第三方网关非标准码，等同于 503）
		408, 406, 502, 503, 504, 506, 507, 508, 509, 510, 511:
		return true
	}

	return statusCode >= 500
}
```

说明:424 原本落在 `switch` 之外且 `< 500`,被判为不可重试。加入 case 后 client 内部逐 attempt 重试循环(`CreateChatCompletion`/`CreateChatCompletionStream`)会在遇到 424 时按指数退避重试,而非立即向 handler 抛错。

- [ ] **Step 2: 创建 client 层重试状态码单测**

```go
// backend/client/openai_client_retry_test.go
package client

import "testing"

// TestIsRetryableHTTPStatus 锁定可重试状态码白名单，特别覆盖 424 上游过载码。
// 429 走专属分支（按 errorBody 区分配额耗尽 vs 临时限流），其余状态码查白名单。
func TestIsRetryableHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorBody  string
		want       bool
	}{
		{"424 upstream overload retryable", 424, "Service temporarily unavailable", true},
		{"408 timeout retryable", 408, "", true},
		{"429 transient rate limit retryable", 429, "rate limit", true},
		{"429 quota exhausted not retryable", 429, "insufficient_quota", false},
		{"500 retryable", 500, "", true},
		{"502 retryable", 502, "", true},
		{"503 retryable", 503, "", true},
		{"400 not retryable", 400, "", false},
		{"401 not retryable", 401, "", false},
		{"404 not retryable", 404, "", false},
		{"422 not retryable", 422, "", false},
	}
	for _, tt := range tests {
		got := isRetryableHTTPStatus(tt.statusCode, tt.errorBody)
		if got != tt.want {
			t.Errorf("%s: isRetryableHTTPStatus(%d, %q) = %v, want %v",
				tt.name, tt.statusCode, tt.errorBody, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: 验证 client 层**
Run: `cd backend && go test ./client/... -run TestIsRetryableHTTPStatus -v`
Expected:
  - Exit code: 0
  - Output contains: "ok" 和 "PASS"
  - Output does NOT contain: "want true" 或 "want false" 的失败行

- [ ] **Step 4: 提交**
Run: `git add backend/client/openai_client.go backend/client/openai_client_retry_test.go && git commit -m "fix(client): retry HTTP 424 upstream overload instead of surfacing to client"`

---

### Task 3: response_handler 兜底将 424 转为 overloaded_error

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/response_handler.go:401-420`(替换整个 timeout 检测分支到 classifiedError 行)
- Create: `backend/handler/response_handler_error_test.go`

- [ ] **Step 1: 修改 SendErrorResponse 以将 424/529 转为 overloaded_error**

文件: `backend/handler/response_handler.go:401-420`(替换整个 timeout 检测分支及其后的 classifiedError 行,用"重写的 timeout 分支 + 新增 overload 分支 + classifiedError 行"覆盖)

**注意:** 第 399 行是 model routing 分支的闭合 `}`、第 400 行是空行 —— 这两行**不要动**。替换从第 401 行(timeout 注释)开始,到第 420 行(`classifiedError := ...`)结束,替换后的内容末尾自带 classifiedError 行以保证衔接:

```go
	// Detect timeout errors and return overloaded_error.
	// Claude Code auto-retries on overloaded_error with backoff,
	// so timeout errors don't immediately surface to the user.
	isTimeout := strings.Contains(strings.ToLower(errorMsg), "timeout") ||
		strings.Contains(strings.ToLower(err.Error()), "context deadline")
	if isTimeout {
		statusCode = http.StatusServiceUnavailable // 503
		logger.Warn("← [SendErrorResponse] Timeout error, returning overloaded_error: %s", errorMsg)
		c.JSON(statusCode, gin.H{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "overloaded_error",
				"message": "Upstream provider timeout. Please retry.",
			},
		})
		return
	}

	// Detect upstream overload status codes and return overloaded_error.
	// 424 "Service temporarily unavailable" is returned by some OpenAI-compatible
	// gateways to mean upstream overload; 529 is the canonical overloaded code.
	// Claude Code auto-retries on overloaded_error with backoff, so when the
	// proxy's own retries (client layer + retry engine) are exhausted, the
	// client still retries gracefully instead of surfacing a hard 424 error.
	if statusCode == 424 || statusCode == 529 {
		logger.Warn("← [SendErrorResponse] Upstream overload (status %d), returning overloaded_error: %s", statusCode, errorMsg)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "overloaded_error",
				"message": "Upstream provider temporarily overloaded. Please retry.",
			},
		})
		return
	}

	// 分类并格式化错误消息
	classifiedError := client.ClassifyOpenAIError(errorMsg)
```

说明:`statusCode` 在函数前段(line 354-361)已从 `"OpenAI API error (status 424): ..."` 提取出 424,此处直接复用。新分支只在代理自身两层重试(client 层 + retry engine)都耗尽、错误最终到达 `SendErrorResponse` 时触发,作为最后一道防线让客户端自己重试。

- [ ] **Step 2: 创建 response_handler 错误响应单测（正反对照）**

```go
// backend/handler/response_handler_error_test.go
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// overloadedBody 是 SendErrorResponse 返回的 overloaded_error 响应体结构。
type overloadedBody struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// TestSendErrorResponse_424_ReturnsOverloadedError 验证 424 被兜底转成
// overloaded_error + 503，使 Claude Code 客户端能自动重试而不中断。
func TestSendErrorResponse_424_ReturnsOverloadedError(t *testing.T) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	rh.SendErrorResponse(c, fmt.Errorf("OpenAI API error (status 424): Service temporarily unavailable"))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	var body overloadedBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v (body=%s)", err, w.Body.String())
	}
	if body.Error.Type != "overloaded_error" {
		t.Errorf("error.type = %q, want overloaded_error", body.Error.Type)
	}
}

// TestSendErrorResponse_401_NotOverloaded 验证非过载码（如 401）保持原状态码
// 与 api_error 类型，不被错误地转成 overloaded_error。
func TestSendErrorResponse_401_NotOverloaded(t *testing.T) {
	rh := NewResponseHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	rh.SendErrorResponse(c, fmt.Errorf("OpenAI API error (status 401): invalid_api_key"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	var body overloadedBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v (body=%s)", err, w.Body.String())
	}
	if body.Error.Type != "api_error" {
		t.Errorf("error.type = %q, want api_error", body.Error.Type)
	}
}
```

- [ ] **Step 3: 验证 response_handler 过载兜底**
Run: `cd backend && go test ./handler/... -run 'TestSendErrorResponse_424_ReturnsOverloadedError|TestSendErrorResponse_401_NotOverloaded' -v`
Expected:
  - Exit code: 0
  - Output contains: "ok" 和 "PASS"
  - Output does NOT contain: "want 503"、"want overloaded_error"、"want 401"、"want api_error" 的失败行

- [ ] **Step 4: 提交**
Run: `git add backend/handler/response_handler.go backend/handler/response_handler_error_test.go && git commit -m "fix(handler): convert HTTP 424/529 upstream overload to overloaded_error for client retry"`

---

### Task 4: 同步死代码白名单 + 全量回归 + 重建二进制

**Depends on:** Task 1, Task 2, Task 3
**Files:**
- Modify: `backend/handler/retry_handler.go:154-165`(`IsRetryableHTTPStatus`，保持三处一致)
- Build: 重建 `backend/claude-with-openai-api` 二进制

- [ ] **Step 1: 同步 retry_handler.go 的 IsRetryableHTTPStatus 加入 424**

文件: `backend/handler/retry_handler.go:154-165`(替换整个 `IsRetryableHTTPStatus` 函数)

```go
// IsRetryableHTTPStatus checks if an HTTP status code is retryable
func IsRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		424,                            // 上游过载（第三方网关非标准码）
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}
```

说明:此 exported 函数目前不在主请求路径(主流程走 `retry.NewEngine` + client 层判断),但为避免未来误用导致 424 再次漏网,与 Task 1/Task 2 保持白名单一致。

- [ ] **Step 2: 全量回归测试**
Run: `cd backend && go test ./retry/... ./client/... ./handler/... -count=1`
Expected:
  - Exit code: 0
  - Output contains: "ok" 开头的三个包行
  - Output does NOT contain: "FAIL" 或 "build failed"

- [ ] **Step 3: 重建二进制（关键：覆盖旧二进制）**
Run: `cd backend && go build -o claude-with-openai-api .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" 或 "undefined"
  - 文件 `backend/claude-with-openai-api` 存在且为新生成的可执行文件

说明:线上实例(如 54988 端口)若跑旧二进制,代码修复不会生效。执行者完成本 Task 后,需将新二进制部署替换线上实例并重启(部署动作不在本 Plan 范围,但必须执行)。

- [ ] **Step 4: 提交**
Run: `git add backend/handler/retry_handler.go && git commit -m "fix(handler): sync IsRetryableHTTPStatus whitelist with 424 for consistency"`

---

## 验收标准

修复后,`424 Service temporarily unavailable` 的处理路径变为:

1. **首选(客户端无感):** client 层遇 424 → 指数退避重试 → 成功则正常返回,客户端无感知。
2. **次选(换节点):** client 重试耗尽 → retry engine 识别 `CategoryServerError` → 换 config 节点重试。
3. **兜底(客户端可重试):** 上述都失败 → `SendErrorResponse` 返回 `overloaded_error` + 503 → Claude Code 客户端自动退避重试,**不再中断**。

验证命令(全部应通过):
```bash
cd backend && go test ./retry/... ./client/... ./handler/... -count=1
```
