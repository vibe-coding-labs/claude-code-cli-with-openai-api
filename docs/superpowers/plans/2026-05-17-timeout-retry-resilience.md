# Timeout Retry Resilience — 修复超时错误的分类和重试机制

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修复 "timeout awaiting response headers" 超时错误被分类为 CategoryUnknown（不可重试）的 bug，确保超时错误被正确归类为网络错误并触发自动重试，同时让 Claude Code CLI 在代理超时时收到 overloaded_error 自动重试。

**Architecture:** 错误发生链路：`httpClient.Do()` → `http.Transport.ResponseHeaderTimeout` 触发 `net.Error(timeout=true)` → `CreateChatCompletionStream` 内部重试 3 次后包装为 `fmt.Errorf("all retry attempts failed: %w", err)` → 包装后的错误丢失 `net.Error` 接口 → `retry.ClassifyError()` 无法识别为网络错误 → `CategoryUnknown(Retryable:false)` → 外层 `retry.Engine` 不重试 → 直接返回错误 → `SendErrorResponse` 返回 `api_error` 而非 `overloaded_error`。修复点：1) retry 网络信号列表添加 "timeout" 关键词；2) SendErrorResponse 将超时错误归类为 overloaded_error；3) 数据库默认 retry_count 从 3 提高到 10。

**Tech Stack:** Go 1.22, SQLite 3, Gin HTTP framework

**Risks:**
- Task 1 添加 "timeout" 到 networkSignals 可能匹配到非网络超时 → 缓解：用精确模式 "timeout awaiting response" 而非单独 "timeout"
- Task 2 将超时返回为 overloaded_error 可能导致 Claude Code CLI 过度重试 → 缓解：Claude Code CLI 自带指数退避，不会无限重试
- Task 3 修改默认值只影响新创建的 config，已有 config 需手动更新 → 缓解：提供 SQL 更新语句

---

### Task 1: 修复 retry.ClassifyError 超时错误分类 — 添加 "timeout awaiting response" 到网络信号列表

**Depends on:** None
**Files:**
- Modify: `backend/retry/retry.go:261-266`（networkSignals 变量）
- Modify: `backend/retry/retry_test.go`（添加超时分类测试）

- [ ] **Step 1: 修改 networkSignals 添加超时相关信号**

文件: `backend/retry/retry.go:261-266`（替换整个 networkSignals 变量）

```go
var networkSignals = []string{
	"connection refused", "connection reset", "connection timeout",
	"no such host", "network is unreachable", "broken pipe",
	"i/o timeout", "temporary failure", "dns error",
	"tls handshake timeout", "temporary error",
	"timeout awaiting response headers",
	"timeout awaiting response",
	"context deadline exceeded",
}
```

- [ ] **Step 2: 添加超时错误分类测试**

文件: `backend/retry/retry_test.go`（在 `TestClassifyError_Network` 函数中追加测试用例）

在 `TestClassifyError_Network` 函数的 `signals` 切片中追加：

```go
signals := []string{"connection refused", "connection reset", "i/o timeout", "broken pipe",
	"timeout awaiting response headers", "timeout awaiting response",
	"context deadline exceeded"}
```

同时在文件末尾添加新的测试函数：

```go
func TestClassifyError_WrappedTimeout(t *testing.T) {
	innerErr := fmt.Errorf("net/http: timeout awaiting response headers")
	wrappedErr := fmt.Errorf("all retry attempts failed, last error: %w", innerErr)
	if got := ClassifyError(wrappedErr); got != CategoryNetwork {
		t.Errorf("wrapped timeout error: got %v, want CategoryNetwork", got)
	}
}

func TestClassifyError_WrappedContextDeadline(t *testing.T) {
	innerErr := fmt.Errorf("context deadline exceeded")
	wrappedErr := fmt.Errorf("all retry attempts failed, last error: %w", innerErr)
	if got := ClassifyError(wrappedErr); got != CategoryNetwork {
		t.Errorf("wrapped context deadline: got %v, want CategoryNetwork", got)
	}
}
```

- [ ] **Step 3: 验证测试通过**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./retry/ -run "TestClassifyError" -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

- [ ] **Step 4: 提交**

Run: `git add backend/retry/retry.go backend/retry/retry_test.go && git commit -m "fix(retry): classify wrapped timeout errors as network (retryable)"`

---

### Task 2: 将超时错误返回为 overloaded_error — 让 Claude Code CLI 自动重试

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/response_handler.go:346-414`（SendErrorResponse 函数）

- [ ] **Step 1: 在 SendErrorResponse 中添加超时错误检测，返回 overloaded_error**

文件: `backend/handler/response_handler.go:398`（在 `isModelError` 检查之后，`classifiedError` 赋值之前添加）

在 `if isModelError { ... return }` 代码块之后，`classifiedError := client.ClassifyOpenAIError(errorMsg)` 之前，添加：

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
```

- [ ] **Step 2: 验证编译通过**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "cannot"

- [ ] **Step 3: 提交**

Run: `git add backend/handler/response_handler.go && git commit -m "feat(handler): return overloaded_error on timeout so Claude Code auto-retries"`

---

### Task 3: 提高默认重试次数并更新现有 config — 从 3 次提升到 10 次

**Depends on:** Task 1
**Files:**
- Modify: `backend/handler/config_manager.go:108-109`（默认值设置）
- Modify: `backend/handler/retry_handler.go:169`（DefaultMaxRetries 常量）

- [ ] **Step 1: 修改配置默认值 — 将 request_timeout 从 180s 提升到 300s（如未设置），retry_count 默认值从 3 提升到 10**

文件: `backend/handler/config_manager.go:108-112`（替换整个 if 块）

```go
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 300
	}
	if config.StreamStallTimeout == 0 {
		config.StreamStallTimeout = 60
	}
	if config.RetryCount == 0 {
		config.RetryCount = 10
	}
```

- [ ] **Step 2: 修改 DefaultMaxRetries 常量**

文件: `backend/handler/retry_handler.go:169`（替换 DefaultMaxRetries 常量）

```go
	DefaultMaxRetries = 10 // 默认重试 10 次（覆盖网络抖动+上游超时）
```

- [ ] **Step 3: 更新现有数据库中的低 retry_count 配置**

Run: `sqlite3 /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/data/proxy.db "UPDATE api_configs SET retry_count = 10 WHERE retry_count < 10; SELECT id, name, retry_count FROM api_configs;"`
Expected:
  - Exit code: 0
  - Output shows retry_count = 10 for all configs

- [ ] **Step 4: 验证编译通过**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "cannot"

- [ ] **Step 5: 重新构建二进制文件并重启服务**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o claude-code-cli-with-openai-api . && launchctl unload ~/Library/LaunchAgents/com.vibecoding.claude-proxy.plist && launchctl load ~/Library/LaunchAgents/com.vibecoding.claude-proxy.plist`
Expected:
  - Exit code: 0
  - `ps aux | grep claude-code-cli-with-openai-api` shows running process

- [ ] **Step 6: 提交**

Run: `git add backend/handler/config_manager.go backend/handler/retry_handler.go && git commit -m "feat(config): increase default retry count to 10 for better timeout resilience"`
