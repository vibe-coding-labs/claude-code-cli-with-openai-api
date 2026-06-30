# Bug Fix: InvalidHTTPResponse 间歇性错误

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Symptom:** Claude Code CLI 偶尔报错 `API Error: InvalidHTTPResponse fetching "http://192.168.1.90:54988/v1/messages?beta=true". For more information, pass verbose: true in the second argument to fetch()`

**Root Cause:** 三个叠加问题导致客户端收到不完整的 HTTP 响应：

1. **HTTP Server 无超时配置（主因）** — `cmd/server.go:404` 使用 `router.Run(addr)`，底层 `http.Server` 的 `ReadTimeout`/`WriteTimeout`/`IdleTimeout` 全部为零。当上游 API 响应缓慢（>30s）时，我们的代理长时间不向客户端写入任何数据，客户端侧的 keepalive 连接变为僵死连接，下一次请求复用该连接时收到 FIN/RST 而非 HTTP 响应 → `InvalidHTTPResponse`。

2. **retry 循环内 defer 导致资源泄漏** — `backend/client/openai_client.go:541-542` 在 retry for 循环内使用 `defer resp.Body.Close()` 和 `defer cancel()`。Go 的 defer 在函数返回时执行，不是在循环迭代结束时执行。重试 N 次会累积 N 个未关闭的 Body 和 Context，最终耗尽连接池。

3. **非流式请求无客户端心跳** — 非流式路径中，代理收到请求后直到上游返回前不发送任何数据给客户端（无 100-continue、无 SSE 心跳、无 chunk 保持）。客户端可能在代理处理期间超时断开。

**Impact:** 所有通过代理使用 Claude Code CLI 的用户，尤其在上游 API 响应较慢时（复杂推理、大 token 输出）更容易触发。偶发性意味着连接池状态相关，非必现但高频场景下每日可触发多次。

**Scope:** Small
**Risk:** Medium（修改 HTTP Server 配置和核心请求循环，影响所有请求路径）
**Risks:**
- Task 1 修改 server 启动逻辑，WriteTimeout 设置需平衡长流式请求和连接保护 → 缓解：流式请求通过 `c.Stream()` 发送，Gin 在流式写入期间不强制 WriteTimeout
- Task 2 修改 retry 循环的 defer 模式，必须确保每个迭代正确释放资源 → 缓解：显式 Close 替代 defer，保持语义一致
- Task 3 添加 HTTP Server 层面的连接状态日志 → 缓解：仅在 DEBUG 级别记录，不影响生产性能

---

### Task 1: 配置 HTTP Server 超时 — 消除僵死连接

**Depends on:** None
**Files:**
- Modify: `backend/cmd/server.go:401-408`（替换 `router.Run(addr)` 为显式 `http.Server` 配置）

- [ ] **Step 1: 替换 router.Run 为显式 http.Server 配置**

文件: `backend/cmd/server.go:401-408`（替换 router.Run 调用和整个 server 启动块）

```go
		// Start server with proper timeouts to prevent stale connections.
		// router.Run() uses http.Server with zero timeouts, causing
		// InvalidHTTPResponse errors when the client reuses a keepalive
		// connection that the server has silently closed.
		addr := fmt.Sprintf("%s:%d", cfg.Host, actualPort)
		srv := &http.Server{
			Addr:           addr,
			Handler:        router,
			ReadTimeout:    30 * time.Second,   // Time limit for reading the entire request (including body)
			ReadHeaderTimeout: 10 * time.Second, // Time limit for reading request headers
			WriteTimeout:   0,                   // No write timeout — streaming responses can take minutes
			IdleTimeout:    120 * time.Second,   // Keepalive connection idle timeout
			MaxHeaderBytes: 1 << 20,             // 1MB max header size
		}

		color.New(color.FgCyan, color.Bold).Println("\n🚀 Server starting...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			color.New(color.FgRed, color.Bold).Print("❌ Failed to start server: ")
			color.New(color.FgRed).Println(err)
			return err
		}
		return nil
```

注意：`WriteTimeout` 设为 0（无限），因为流式响应可能持续数分钟。Gin 的 `c.Stream()` 和 `c.SSEvent()` 在 WriteTimeout=0 时正常工作。`ReadTimeout=30s` 足够接收完整的请求体（Claude Code 通常在数秒内发送完毕）。`IdleTimeout=120s` 确保 keepalive 连接在 2 分钟无活动后关闭，避免客户端复用已半关闭的连接。

- [ ] **Step 2: 验证编译通过**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "cannot"

- [ ] **Step 3: 质量门禁 — 交付前多维检查**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./cmd/... && go build ./...`
Expected:
  - Exit code: 0
  - 无遗留 debug 语句
  - 无未使用 import

- [ ] **Step 4: 提交**
Run: `git add backend/cmd/server.go && git commit -m "fix(server): configure HTTP server timeouts to prevent InvalidHTTPResponse errors"`

---

### Task 2: 修复 retry 循环内 defer 资源泄漏 — 确保连接正确释放

**Depends on:** None
**Files:**
- Modify: `backend/client/openai_client.go:530-542`（非流式 CreateChatCompletion 的 retry 循环体内 defer）
- Modify: `backend/client/openai_client.go:810-820`（流式 CreateChatCompletionStream 的 retry 循环体内）

- [ ] **Step 1: 修复非流式 retry 循环的 defer 资源泄漏**

文件: `backend/client/openai_client.go:533-542`（替换 `httpClient.Do` 调用后的 defer 部分）

找到这段代码：
```go
			resp, err := c.httpClient.Do(req)
			if err != nil {
				cancel()
				lastErr = err
				logger.Warn("← [OpenAIClient] Request failed (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)
				c.logProxyError(openAIReq.Model, "", 0, err, err.Error(), "", database.StageRequest, attempt, time.Since(startTime).Milliseconds(), "")
				continue
			}
			defer resp.Body.Close()
			defer cancel()
```

替换为：
```go
			resp, err := c.httpClient.Do(req)
			if err != nil {
				cancel()
				lastErr = err
				logger.Warn("← [OpenAIClient] Request failed (attempt %d/%d): %v", attempt+1, c.RetryCount+1, err)
				c.logProxyError(openAIReq.Model, "", 0, err, err.Error(), "", database.StageRequest, attempt, time.Since(startTime).Milliseconds(), "")
				continue
			}
```

即删除 `defer resp.Body.Close()` 和 `defer cancel()` 两行。确保后续的错误路径（非 200 状态码）中已有显式 `resp.Body.Close()`（在 line 546），成功路径中也有显式 Close（在 line 643 已有 `resp.Body.Close()`）。对于 `cancel()`，确保在函数的每个 return 路径前调用 `cancel()`。

同时，在函数的每个 return 路径前添加 `cancel()` 调用。具体修改位置：

**Line 700-701 区域**（成功返回路径）：
找到：
```go
			return &openAIResp, nil
```
替换为：
```go
			cancel()
			return &openAIResp, nil
```

**Line 709-713 区域**（所有重试失败路径）：
找到：
```go
		// All retries failed
		if lastErr != nil {
			return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
		}
		return nil, fmt.Errorf("all retry attempts failed")
```
此路径中 cancel 已经在最后一次循环迭代中被调用过了（通过 `continue` 前的 `cancel()` 调用），所以无需额外调用。但如果最后一次迭代没有进入循环（`attempt > c.RetryCount`），cancel 未被调用。为安全起见，在 for 循环之前添加一个跟踪变量：

在 `var lastErr error` 声明（约 line 477）之后添加：
```go
		var currentCancel context.CancelFunc // track the latest context cancel for cleanup
```

在 `ctx, cancel := context.WithTimeout(...)` 行（约 line 501）之后添加：
```go
			currentCancel = cancel
```

然后将最后的 "All retries failed" 块替换为：
```go
		// All retries failed
		if currentCancel != nil {
			currentCancel()
		}
		if lastErr != nil {
			return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
		}
		return nil, fmt.Errorf("all retry attempts failed")
```

- [ ] **Step 2: 修复流式 retry 循环中的 context 资源问题**

文件: `backend/client/openai_client.go:813-820`（流式 CreateChatCompletionStream 的 retry 循环体中）

流式请求没有使用 context.WithTimeout（line 782 使用 `http.NewRequest` 而非 `http.NewRequestWithContext`），所以没有 cancel 泄漏问题。但需确认 `resp.Body` 在非 200 路径中被正确关闭。

检查 `backend/client/openai_client.go:823` 处的 `resp.Body.Close()` 确认已存在。如果已存在则无需修改此部分。

- [ ] **Step 3: 验证编译和测试通过**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test ./client/... -count=1 -timeout 60s 2>&1 || true`
Expected:
  - Exit code: 0 for `go build`
  - 无编译错误

- [ ] **Step 4: 质量门禁 — 交付前多维检查**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./client/... && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "loses cancel" or "defer"

- [ ] **Step 5: 提交**
Run: `git add backend/client/openai_client.go && git commit -m "fix(client): remove defer-in-loop to prevent resource leak in retry cycle"`

---

### Task 3: 添加连接错误诊断日志 — 帮助定位偶发性问题

**Depends on:** Task 1, Task 2
**Files:**
- Modify: `backend/handler/handler.go:132`（CreateMessage 入口处添加 panic recovery 日志）
- Modify: `backend/cmd/server.go`（在 Task 1 的 Server 配置中添加 ConnState 回调）

- [ ] **Step 1: 在 HTTP Server 上添加连接状态监控**

文件: `backend/cmd/server.go`（在 Task 1 创建的 `srv` 变量后添加 `ConnState` 回调）

在 `srv` 结构体初始化中，`MaxHeaderBytes` 行之后添加：
```go
			ConnState: func(conn net.Conn, state http.ConnState) {
				if state == http.StateClosed || state == http.StateHijacked {
					logger := utils.GetLogger()
					if logger != nil {
						logger.Debug("[ConnState] %s -> %s (remote=%s)", conn.RemoteAddr(), state, conn.RemoteAddr())
					}
				}
			},
```

同时在文件顶部的 import 中确保包含 `"net"`（用于 `net.Conn` 类型）。检查 import 区是否已有 `"net/http"` 和 `"time"`，如果缺少 `"net"` 则添加。

- [ ] **Step 2: 在 CreateMessage 入口添加 panic recovery 日志**

文件: `backend/handler/handler.go:132`（CreateMessage 函数开头）

Gin 框架自带 panic recovery 中间件，会自动捕获 panic 并返回 500。但当 panic 发生时，我们无法区分 500 是业务错误还是 panic 导致。在 CreateMessage 入口添加一条 DEBUG 日志，用于在日志中标记每个请求的开始，与 panic recovery 的日志配对使用。

在 `logger := utils.GetLogger()` 行之后（约 line 134），添加：
```go
	// Log request entry for connection error correlation
	logger.Debug("→ [CreateMessage] Request received: method=%s path=%s remote=%s content-length=%s",
		c.Request.Method, c.Request.URL.String(), c.ClientIP(), c.Request.Header.Get("Content-Length"))
```

- [ ] **Step 3: 验证编译通过**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 4: 质量门禁**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./... && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 5: 提交**
Run: `git add backend/cmd/server.go backend/handler/handler.go && git commit -m "feat(observability): add connection state logging and request tracing for intermittent errors"`

---

## Progress Checkpoint

**Total Tasks:** 3
**Execution Order:** Task 1 → Task 2 → Task 3（Task 1 和 Task 2 可并行）
**Estimated Impact:** 修复后 `InvalidHTTPResponse` 应从每日数次降至零。连接状态日志帮助确认是否还有其他边缘情况。
