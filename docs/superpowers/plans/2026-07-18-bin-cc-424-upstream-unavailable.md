# bin-cc 启动报错 424 Upstream service temporarily unavailable 修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修复 `bin-cc` 快捷指令启动的 Claude Code 因上游 picpi 返回 `424 Upstream service temporarily unavailable` 而中断的问题,让代理对 424 执行三层重试/兜底,客户端无感或自动退避重试。

## Root Cause（根因分析 — bin-cc 场景）

`bin-cc` 脚本(`~/.local/bin/bin-cc`)注释已直接点明问题本质:

```
链路: claude(Anthropic /v1/messages) -> 本地代理 :54988 (Anthropic<->OpenAI) -> cn2.picpi.top (gpt-5.5)
上游 picpi 仅 gpt-5.5 实测可用 (5.2/5.4/5.6 上游 424)
```

即 **picpi 上游对"不支持/过载"的请求会返回 HTTP 424**。脚本已把 big/middle/small 三档全映射到 gpt-5.5 规避了"模型不支持"的 424,但 **gpt-5.5 自身偶发过载也会返回 424**,这个 424 被代理透传,最终让 Claude Code 客户端中断。

完整因果链(已用 `strings` 反编译 + 源码 + 进程状态三重确认):

```
picpi 上游返回 HTTP 424 "Upstream service temporarily unavailable"
        │
   ① client 层 isRetryableHTTPStatus(424) → false
      （openai_client.go:250-262 白名单 408/406/429/500-511/≥500，无 424）
      → client 不重试，返回 "OpenAI API error (status 424): Upstream service temporarily unavailable"
        │
   ② handler 层 retry.NewEngine() → ClassifyError 判为 CategoryUnknown
      （retry.go:251-259 isServerErrorStatus 只认 500/502/503/504/506/507/508，无 424）
      → 不换 config 节点重试（即便配了多 picpi 节点也不切换）
        │
   ③ response_handler.SendErrorResponse 提取出 424
      （response_handler.go:347-432，424 既非 model routing 也非 timeout）
      → 走默认分支：c.JSON(424, {type:"api_error", ...})  ← 不是 overloaded_error
        │
   ④ Claude Code 客户端收到 424 + api_error
      （客户端只对 overloaded_error / 429 / 5xx 自动重试）
      → 不重试 → 中断："API Error: 424 Upstream service temporarily unavailable"
```

**三个已确认的客观事实:**
1. `strings backend/claude-with-openai-api | grep` 显示旧二进制含 `overloaded_error` 但**不含 `status 424`** → 旧二进制无 424 处理。
2. 二进制文件时间 `2026-06-21`,systemd 进程 `2026-07-17` 启动 → **跑的是 6 月旧二进制**。
3. `git diff` 为空 → 上一份 424 Plan(`2026-07-17-424-status-code-retry.md`)**尚未执行**,源码原样。

**与上一份 Plan 的关系:** 上一份 `2026-07-17-424-status-code-retry.md` 已写好三层代码修复(retry/client/response_handler),但未执行。本 Plan 是其 **bin-cc 场景增强版**:代码修复 Task 与上一份一致(此处重写以保持自包含、可直接执行),并**新增 Task 5:重建二进制 + 重启 systemd 服务** —— 这是 bin-cc 场景下"改了源码也无效"的根因,也是 [[st-cc-proxy-binary-stale]] 记录的同类教训。

**修复策略(三层纵深防御 + 部署):**
- **Task 1:** retry 包识别 `status 424` → 换节点重试生效(防线②)
- **Task 2:** client 层 `isRetryableHTTPStatus` 加 424 → client 逐 attempt 重试(防线①,客户端无感)
- **Task 3:** response_handler 把 424 兜底转 `overloaded_error`+503 → 客户端自动退避重试(防线③)
- **Task 4:** 同步死代码白名单 + 全量回归
- **Task 5:** 重建二进制 + 重启 54988 systemd 服务(部署,bin-cc 场景必需)

**Architecture:** 上游 424 → client 白名单拦截重试(①)→ 耗尽则 retry engine 换节点重试(②)→ 仍失败则 SendErrorResponse 转 overloaded_error(③)→ Claude 客户端自动重试。三层任一命中即消化错误。Task 5 把修复落到跑在 :54988 的 systemd 实例上。

**Tech Stack:** Go 1.x, gin-gonic, net/http, testing 标准库, systemd user service

**Risks:**
- 424 是非标准过载码,不同上游语义可能不同 → 缓解:LLM 网关场景下 424 几乎都是过载,重试是安全默认;且仅当状态码=424 时触发,不影响真正的 4xx 客户端错误(401/400/422 等保持原样,见 Task 3 反例测试)。
- Task 3 改 `SendErrorResponse` 是所有错误响应必经路径 → 缓解:新增分支只针对 424/529,在 model routing / timeout 之后、默认分支之前插入;补正反对照单测。
- Task 5 重启 systemd 服务会**短暂中断 54988 代理(数秒)** → 缓解:重启前确认无正在进行的 Claude 会话;systemd `restart` 秒级完成。重启后 bin-cc 下次启动即用新二进制。
- 旧二进制覆盖风险:Task 5 直接 `go build -o` 覆盖 `backend/claude-with-openai-api`,若构建失败会留下损坏二进制 → 缓解:先 `go build -o /tmp/xxx` 验证成功再覆盖,或构建失败时 systemd 仍跑旧二进制(进程已加载到内存,文件覆盖不影响已运行进程,直到 restart)。
- 代理 DB(`./data/proxy.db`,193MB)在运行时被读写 → 缓解:Task 5 仅 restart 服务,不触碰 DB 文件。

---

### Task 1: retry 包识别 424 为可重试服务端错误

**Depends on:** None
**Files:**
- Modify: `backend/retry/retry.go:251-259`(`isServerErrorStatus`)
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
- Modify: `backend/handler/response_handler.go:401-420`(替换 timeout 分支到 classifiedError 行)
- Create: `backend/handler/response_handler_error_test.go`

- [ ] **Step 1: 修改 SendErrorResponse 以将 424/529 转为 overloaded_error**

文件: `backend/handler/response_handler.go:401-420`(替换整个 timeout 检测分支及其后的 classifiedError 行,用"重写的 timeout 分支 + 新增 overload 分支 + classifiedError 行"覆盖)

**注意:** 第 399 行是 model routing 分支的闭合 `}`、第 400 行是空行 —— 这两行**不要动**。替换从第 401 行(timeout 注释)开始,到第 420 行(`classifiedError := ...`)结束,替换后内容末尾自带 classifiedError 行以保证衔接:

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
	// 424 "Upstream service temporarily unavailable" is returned by picpi and other
	// OpenAI-compatible gateways to mean upstream overload; 529 is the canonical
	// overloaded code. Claude Code auto-retries on overloaded_error with backoff, so
	// when the proxy's own retries (client layer + retry engine) are exhausted, the
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

	rh.SendErrorResponse(c, fmt.Errorf("OpenAI API error (status 424): Upstream service temporarily unavailable"))

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

### Task 4: 同步死代码白名单 + 全量回归

**Depends on:** Task 1, Task 2, Task 3
**Files:**
- Modify: `backend/handler/retry_handler.go:154-165`(`IsRetryableHTTPStatus`)

- [ ] **Step 1: 同步 retry_handler.go 的 IsRetryableHTTPStatus 加入 424**

文件: `backend/handler/retry_handler.go:153-165`(替换整个 `IsRetryableHTTPStatus` 函数,含注释)

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

- [ ] **Step 2: 全量回归测试**
Run: `cd backend && go test ./retry/... ./client/... ./handler/... -count=1`
Expected:
  - Exit code: 0
  - Output contains: "ok" 开头的三个包行
  - Output does NOT contain: "FAIL" 或 "build failed"

- [ ] **Step 3: 提交**
Run: `git add backend/handler/retry_handler.go && git commit -m "fix(handler): sync IsRetryableHTTPStatus whitelist with 424 for consistency"`

---

### Task 5: 重建二进制 + 重启 54988 systemd 服务（部署 — bin-cc 场景必需）

**Depends on:** Task 1, Task 2, Task 3, Task 4
**Files:**
- Build: `backend/claude-with-openai-api`(覆盖旧二进制)
- Operate: systemd user service `claude-openai-proxy.service`

- [ ] **Step 1: 先构建到临时路径验证编译通过**

先构建到 `/tmp` 验证,避免构建失败损坏线上二进制:

Run: `cd backend && go build -o /tmp/claude-with-openai-api.new .`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" 或 "undefined"
  - 文件 `/tmp/claude-with-openai-api.new` 存在

- [ ] **Step 2: 验证新二进制含 424 修复**

用 `strings` 确认新二进制已包含 424 处理(对比旧二进制不含):

Run: `strings /tmp/claude-with-openai-api.new | grep -c "status 424"`
Expected:
  - Output: `1`(新二进制含 "status 424" 字符串,来自 retry.go 的注释/代码)
  - 若为 `0` 说明构建异常,停止后续步骤

- [ ] **Step 3: 覆盖线上二进制**
Run: `cp /tmp/claude-with-openai-api.new backend/claude-with-openai-api && chmod +x backend/claude-with-openai-api`
Expected:
  - Exit code: 0
  - `ls -la backend/claude-with-openai-api` 显示新修改时间为当前时间

- [ ] **Step 4: 重启 54988 systemd 服务以加载新二进制**

重启会短暂中断 54988 代理(秒级)。重启前若有正在进行的 Claude 会话会断开。

Run: `systemctl --user restart claude-openai-proxy.service && sleep 2 && systemctl --user is-active claude-openai-proxy.service`
Expected:
  - Exit code: 0
  - Output: `active`
  - 54988 端口重新监听(进程为新启动时间)

- [ ] **Step 5: 验证新二进制已加载且 424 修复生效**
Run: `systemctl --user status claude-openai-proxy.service --no-pager | head -5 && strings /proc/$(systemctl --user show -p MainPID --value claude-openai-proxy.service)/exe 2>/dev/null | grep -c "status 424"`
Expected:
  - status 输出显示 `active (running)` 且启动时间为刚才
  - strings 计数输出: `1`(运行中进程的二进制含 424 修复)

说明:旧二进制不含 `status 424`(已用 `strings` 确认),新二进制含 → 计数从 0 变 1 即证明线上已加载修复。完成后 `bin-cc` 下次启动的 Claude 会话即受 424 三层重试保护。

- [ ] **Step 6: 提交（二进制不入库,仅记 commit 标记部署完成）**

二进制文件在 `.gitignore` 中不入库,此 Step 仅做部署完成标记,不产生 git 变更。若需留痕,在 commit message 记录:

Run: `git log --oneline -5`(确认 Task 1-4 的四个 commit 已在 HEAD)
Expected:
  - Exit code: 0
  - 最近 4 条 commit 为 Task 1-4 的 fix 提交

---

## 验收标准

修复后,`bin-cc` 启动的 Claude 遇到 picpi 上游 424 时:

1. **首选(客户端无感):** client 层遇 424 → 指数退避重试 → 成功则正常返回,Claude 无感知。
2. **次选(换节点):** client 重试耗尽 → retry engine 识别 `CategoryServerError` → 换 picpi config 节点重试(若配了多节点)。
3. **兜底(客户端可重试):** 上述都失败 → `SendErrorResponse` 返回 `overloaded_error` + 503 → Claude Code 客户端自动退避重试(`CLAUDE_CODE_MAX_RETRIES=10`),**不再中断**。
4. **部署生效:** Task 5 重启后,54988 跑新二进制,`strings` 验证含 `status 424`。

最终验证命令(全部应通过):
```bash
cd backend && go test ./retry/... ./client/... ./handler/... -count=1
systemctl --user is-active claude-openai-proxy.service
```
