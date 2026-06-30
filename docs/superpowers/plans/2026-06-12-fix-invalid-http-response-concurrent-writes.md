# Bug Fix: InvalidHTTPResponse — 流式并发写竞争 + 断开后继续写

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Symptom:** Claude Code 客户端(Node.js/undici)偶发抛出
`API Error: InvalidHTTPResponse fetching "http://192.168.1.90:54988/v1/messages?beta=true"`。
之前的 Plan(`2026-06-03-fix-invalid-http-response.md`,已全部提交:server 超时 + retry defer 泄漏 + ConnState 日志)实施后**仍然偶发**。

**Root Cause:** 经过实际代码调研,确认当前代码(`backend/converter/`)存在两个未被之前 Plan 覆盖的问题,二者都把脏字节送进复用的 keep-alive 连接,undici 把这些孤儿字节当成下一个 HTTP 响应的状态行解析 → `InvalidHTTPResponse`:

1. **流式并发写 `c.Writer` 的数据竞争(主因,两份旧 Plan 均未提及)**
   - `response_converter.go:211` 启动 heartbeat goroutine,每 5s 调 `sendSSE` 写 ping;
   - `response_converter.go:227` 启动 scanner goroutine,持续调 `emitContentBlock*` 写内容;
   - 两个 goroutine **并发写同一个 `c.Writer`**。`http.ResponseWriter`(及 Gin 包装、底层 `*bufio.Writer`)**均非 goroutine-safe**。交错的写入破坏 chunked 分帧 → 客户端收到畸形 HTTP → `InvalidHTTPResponse`。
   - 这与"偶尔"完全吻合:只有 ping 与 content delta 落在同一调度窗口内才会触发(约每 5s 一次窗口)。

2. **`sendSSE` 完全忽略写入错误(次因)**
   - `sse_utils.go:11-15` 的 `c.Writer.Write()` / `Flush()` 返回值被丢弃;
   - 客户端断开后,代理继续向死连接写 SSE(尤其 heartbeat 每 5s 写 ping),产生孤儿字节;`StopHeartbeat`(原实现)只 `close(stopChan)`,**不等待 goroutine 退出**,于是在 `message_stop` 之后、handler 返回、Go 写完 chunk 终止符 `0\r\n\r\n` 之后,仍可能有 ping 漏出 → 复用连接上出现孤儿字节 → `InvalidHTTPResponse`。

**Impact:** 所有走 `/v1/messages` 流式路径的 Claude Code 用户。高频多轮对话(工具调用)场景每日可触发多次;同时浪费上游 token(为已离开的客户端继续处理流)。

**Scope:** Small(3 个文件:`sse_utils.go`、`heartbeat.go`、`response_converter.go`,外加 1 处 handler 可观测日志 + 1 个测试文件)
**Risk:** Medium(修改核心流式输出路径,影响所有流式请求)
**Risks:**
- Task 1 改 `sendSSE` 返回值 `void→bool`:已确认全部调用点都忽略返回值,Go 允许忽略返回值 → 不破坏现有调用点。缓解:编译 + `-race` 验证。
- Task 2 改 `StartHeartbeat` 返回类型 `chan struct{}→*Heartbeat`:已确认仅 `response_converter.go:211` 一个调用点 → 同步更新即可。缓解:编译验证。
- Task 2 的 `heartbeat.Stop()` 是**同步阻塞**(等待 goroutine 退出),调用方需保证不会在持有其他锁时调用 → 仅在 select 之后、最终事件之前调用,且该处不持有 `state.mu`。缓解:确认调用点不在锁内。

**Autonomy Level:** Full

---

### Task 1: 串行化 SSE 写入 + 写入错误检测 — 消除并发写竞争

**Depends on:** None
**Files:**
- Modify: `backend/converter/sse_utils.go:1-27`(整文件重写)
- Modify: `backend/converter/response_converter.go:204-207`(绑定 writer)
- Create: `backend/converter/sse_utils_test.go`(并发写序列化测试)

- [ ] **Step 1: 重写 sse_utils.go — 引入带互斥锁 + 失败记忆的 sseWriter**

文件: `backend/converter/sse_utils.go`(整文件替换)

```go
package converter

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
)

const sseWriterKey = "__claudeProxySSEWriter__"

// sseWriter serializes every SSE write to one connection and remembers whether
// the connection is still writable.
//
// Why this exists: http.ResponseWriter (and Gin's wrapper, and the underlying
// *bufio.Writer) is NOT safe for concurrent use. Previously the streaming
// scanner goroutine and the heartbeat goroutine both called c.Writer.Write /
// Flush at the same time. Interleaved writes corrupted the chunked-transfer
// framing, so the client (undici) received malformed HTTP and threw
// InvalidHTTPResponse. The mutex makes all writes sequential; the closed flag
// stops further writes the moment a write fails (client gone), preventing
// orphan SSE bytes from polluting the next keep-alive response.
type sseWriter struct {
	c      *gin.Context
	mu     sync.Mutex
	closed bool
}

// bindSSEWriter attaches a serialized writer to the request. Call exactly once
// at the start of every streaming response, before any SSE event is emitted
// and before the heartbeat goroutine starts.
func bindSSEWriter(c *gin.Context) *sseWriter {
	w := &sseWriter{c: c}
	c.Set(sseWriterKey, w)
	return w
}

// getSSEWriter returns the writer bound to the request, or nil if none.
func getSSEWriter(c *gin.Context) *sseWriter {
	if v, ok := c.Get(sseWriterKey); ok {
		if w, _ := v.(*sseWriter); w != nil {
			return w
		}
	}
	return nil
}

// write emits one SSE event under the lock. Returns false when the connection
// is broken or the stream has been marked closed; callers must stop writing.
func (w *sseWriter) write(event string, data interface{}) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		w.closed = true
		return false
	}
	if _, err := w.c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData)))); err != nil {
		w.closed = true
		return false
	}
	if err := w.c.Writer.Flush(); err != nil {
		w.closed = true
		return false
	}
	return true
}

// sseWriteClosed reports whether the bound writer has given up (a write
// failed). The streaming scanner uses it to stop consuming upstream data once
// the client is gone. Returns false when no writer is bound.
func sseWriteClosed(c *gin.Context) bool {
	w := getSSEWriter(c)
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

// sendSSE sends a Server-Sent Event. Thread-safe when a writer is bound.
// Returns false if the connection is broken (caller should stop writing).
func sendSSE(c *gin.Context, event string, data interface{}) bool {
	if w := getSSEWriter(c); w != nil {
		return w.write(event, data)
	}
	// Fallback (no bound writer, e.g. a unit-test path): legacy direct write,
	// still reporting success/failure so callers can react.
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err := c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData)))); err != nil {
		return false
	}
	if err := c.Writer.Flush(); err != nil {
		return false
	}
	return true
}

// sendSSEError sends an error event via SSE. Returns false if the write failed.
func sendSSEError(c *gin.Context, errorType, message string) bool {
	return sendSSE(c, "error", map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	})
}
```

- [ ] **Step 2: 在流式入口绑定 sseWriter — 在任何并发写者(heartbeat)启动前**

文件: `backend/converter/response_converter.go:204-207`(在 SSE headers 设置之后、`emitMessageStart` 之前插入 `bindSSEWriter(c)`)

找到:
```go
		c.Header("X-Accel-Buffering", "no")

		// Emit initial events (litellm: sent_first_chunk + sent_content_block_start)
		emitMessageStart(c, state)
```

替换为:
```go
		c.Header("X-Accel-Buffering", "no")

		// Bind a serialized, error-aware SSE writer BEFORE any concurrent
		// writer (the heartbeat below) starts. This serializes all SSE writes
		// so the scanner goroutine and the heartbeat cannot interleave bytes
		// and corrupt the chunked stream (root cause of InvalidHTTPResponse).
		bindSSEWriter(c)

		// Emit initial events (litellm: sent_first_chunk + sent_content_block_start)
		emitMessageStart(c, state)
```

- [ ] **Step 3: 创建并发写序列化测试 — 用 -race 证明互斥锁生效**

文件: `backend/converter/sse_utils_test.go`(新建)

```go
package converter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSendSSE_ConcurrentWritesAreSerialized proves the sseWriter mutex prevents
// concurrent access to c.Writer. Run with -race: a missing lock trips the race
// detector, and interleaved bytes change the total body length.
func TestSendSSE_ConcurrentWritesAreSerialized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	bindSSEWriter(c)

	const goroutines = 8
	const writesPerGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				sendSSE(c, "ping", map[string]string{"type": "ping"})
			}
		}()
	}
	wg.Wait()

	// Each write produces exactly one 35-byte frame:
	// "event: ping\ndata: {\"type\":\"ping\"}\n\n"
	const frameSize = len("event: ping\ndata: {\"type\":\"ping\"}\n\n")
	expected := goroutines * writesPerGoroutine * frameSize
	if recorder.Body.Len() != expected {
		t.Fatalf("body length = %d, want %d (bytes corrupted/lost by concurrent writes)",
			recorder.Body.Len(), expected)
	}
}

// failingResponseWriter is an http.ResponseWriter (+Flusher) whose Write always
// errors, simulating a broken pipe after the client disconnects.
type failingResponseWriter struct{}

func (failingResponseWriter) Header() http.Header       { return http.Header{} }
func (failingResponseWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
func (failingResponseWriter) WriteHeader(int)           {}
func (failingResponseWriter) Flush()                    {}

// TestSendSSE_FailureStopsFurtherWrites proves the closed flag makes every
// write after the first failure a no-op, so a dead connection receives no
// orphan bytes.
func TestSendSSE_FailureStopsFurtherWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(failingResponseWriter{})
	bindSSEWriter(c)

	first := sendSSE(c, "ping", map[string]string{"type": "ping"})
	second := sendSSE(c, "ping", map[string]string{"type": "ping"})

	if first {
		t.Fatalf("first write to broken connection should fail, got success")
	}
	if second {
		t.Fatalf("second write after failure should be a no-op (return false), got success")
	}
}
```

- [ ] **Step 4: 验证编译 + race 检测**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test -race ./converter/ -run TestSendSSE -count=1 -timeout 60s`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "DATA RACE" or "FAIL"

- [ ] **Step 5: 质量门禁 — 交付前多维检查**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./converter/... && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "declared and not used"

- [ ] **Step 6: 提交**
Run: `git add backend/converter/sse_utils.go backend/converter/response_converter.go backend/converter/sse_utils_test.go && git commit -m "fix(converter): serialize SSE writes and detect write failures to prevent InvalidHTTPResponse"`

---

### Task 2: heartbeat 同步停止 + 写失败自终止 — 杜绝终止符后的孤儿 ping

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/heartbeat.go:1-44`(整文件重写)
- Modify: `backend/converter/response_converter.go:210-212`(更新调用点)+ `435-437`(select 后同步停止)

- [ ] **Step 1: 重写 heartbeat.go — 返回 *Heartbeat,Stop 同步等待 goroutine 退出**

文件: `backend/converter/heartbeat.go`(整文件替换)

```go
package converter

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// Heartbeat coordinates periodic ping events. The goroutine stops on ctx
// cancel, an explicit Stop, or the first ping write failure (client gone).
type Heartbeat struct {
	stop chan struct{} // closed by Stop to request termination
	done chan struct{} // closed by the goroutine once it has fully exited
}

// StartHeartbeat sends periodic ping events to keep the connection alive,
// preventing proxy/CDN timeouts during long-running operations. Writes go
// through the serialized, error-aware sendSSE (see sse_utils.go), so a failed
// ping (client disconnected) stops the heartbeat immediately.
func StartHeartbeat(c *gin.Context, ctx context.Context, interval time.Duration) *Heartbeat {
	hb := &Heartbeat{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	go func() {
		defer close(hb.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-hb.stop:
				return
			case <-ticker.C:
				// sendSSE is serialized + error-aware: if the ping write fails
				// the client has disconnected, so stop pinging immediately.
				if !sendSSE(c, models.EventPing, map[string]interface{}{
					"type": models.EventPing,
				}) {
					return
				}
			}
		}
	}()

	return hb
}

// Stop signals the heartbeat to stop and BLOCKS until the goroutine has fully
// exited, guaranteeing no in-flight ping write remains. Idempotent.
//
// Call before emitting the terminal SSE events (message_delta / message_stop)
// so no ping can be written after the response is finalized — such orphan
// bytes on a keep-alive connection are read as the next response's status
// line by undici, producing InvalidHTTPResponse.
func (hb *Heartbeat) Stop() {
	if hb == nil {
		return
	}
	select {
	case <-hb.stop:
		// already stopped / stopping
	default:
		close(hb.stop)
	}
	<-hb.done
}
```

- [ ] **Step 2: 更新 response_converter.go 调用点 — 用 *Heartbeat 替换 chan struct{}**

文件: `backend/converter/response_converter.go:210-212`(替换 StartHeartbeat 调用与 defer)

找到:
```go
		// Start heartbeat to keep connection alive
		heartbeatStop := StartHeartbeat(c, ctx, 5*time.Second)
		defer StopHeartbeat(heartbeatStop)
```

替换为:
```go
		// Start heartbeat to keep connection alive. Stop() is synchronous (it
		// waits for the goroutine to exit), so a deferred Stop on the error
		// paths guarantees no ping write outlives this function.
		heartbeat := StartHeartbeat(c, ctx, 5*time.Second)
		defer heartbeat.Stop()
```

- [ ] **Step 3: 在 select 之后、最终事件之前同步停止 heartbeat**

文件: `backend/converter/response_converter.go:432-437`(在 select 块结束后、`if !state.sentContentBlockFinish` 之前插入同步停止)

找到(select 结束 + 最终事件区块开头):
```go
		case <-time.After(5 * time.Minute):
			sendSSEError(c, "api_error", "Streaming timeout")
			return nil
		}

		// If content_block_finish was never sent (stream ended without finish_reason),
		// close the current block
		if !state.sentContentBlockFinish {
```

替换为:
```go
		case <-time.After(5 * time.Minute):
			sendSSEError(c, "api_error", "Streaming timeout")
			return nil
		}

		// Only the normal-completion case (<-done) reaches here (every other
		// case returns above). Stop the heartbeat SYNCHRONOUSLY before emitting
		// the terminal events, so no ping can be written after message_stop —
		// orphan pings after the chunked terminator corrupt the next
		// keep-alive response (InvalidHTTPResponse). The scanner goroutine has
		// already exited (it closed `done`), so after this only the main
		// goroutine writes.
		heartbeat.Stop()

		// If content_block_finish was never sent (stream ended without finish_reason),
		// close the current block
		if !state.sentContentBlockFinish {
```

- [ ] **Step 4: 验证编译 + race 检测(流式 e2e 测试)**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test -race ./converter/ -count=1 -timeout 120s`
Expected:
  - Exit code: 0
  - Output contains: "PASS" / "ok"
  - Output does NOT contain: "DATA RACE", "FAIL", or "deadlock"

- [ ] **Step 5: 质量门禁**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./converter/... && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 6: 提交**
Run: `git add backend/converter/heartbeat.go backend/converter/response_converter.go && git commit -m "fix(converter): make heartbeat stop synchronously and self-terminate on write failure"`

---

### Task 3: scanner 早退 + handler 可观测日志 — 停止为死连接消耗上游 token

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/response_converter.go:235-244`(scanner 循环顶部加 sseWriteClosed 检查)
- Modify: `backend/handler/handler.go:605-627`(streamResult 为 nil 时记录可观测日志)

- [ ] **Step 1: scanner 循环顶部检测连接已死 — 早退避免消耗上游 token**

文件: `backend/converter/response_converter.go:235-244`(在 ctx.Done 检查的 `default:` 之后、`line := scanner.Text()` 之前插入)

找到:
```go
			// Reset idle timer — upstream sent data (stall detection)
			idleTimer.Reset(stallTimeout)
			select {
			case <-ctx.Done():
				errChan <- fmt.Errorf("client disconnected")
				return
			default:
			}

			line := scanner.Text()
```

替换为:
```go
			// Reset idle timer — upstream sent data (stall detection)
			idleTimer.Reset(stallTimeout)
			select {
			case <-ctx.Done():
				errChan <- fmt.Errorf("client disconnected")
				return
			default:
			}

			// If a prior SSE write failed (client gone), stop consuming
			// upstream data — the connection is dead, so there is no point
			// spending more upstream tokens. The closed flag is set by the
			// serialized writer in sse_utils.go.
			if sseWriteClosed(c) {
				errChan <- fmt.Errorf("client disconnected")
				return
			}

			line := scanner.Text()
```

- [ ] **Step 2: handler 在流式结果为 nil 时记录可观测日志 — 帮助确认 fix 生效**

文件: `backend/handler/handler.go:605-627`(给 `if streamResult != nil` 增加 else 分支)

找到:
```go
		if streamResult != nil {
			h.responseHandler.logRequestWithStreamingDetails(c, configID, openAIReq.Model, streamResult, startTime, "success", "", &req)
			if h.sessionHandler != nil && sessionID != "" {
				var assistantContent interface{}
				if len(streamResult.ToolCalls) > 0 {
					contentBlocks := make([]map[string]interface{}, 0)
					if streamResult.Content != "" {
						contentBlocks = append(contentBlocks, map[string]interface{}{
							"type": "text",
							"text": streamResult.Content,
						})
					}
					for _, tc := range streamResult.ToolCalls {
						contentBlocks = append(contentBlocks, tc)
					}
					assistantContent = contentBlocks
				} else {
					assistantContent = streamResult.Content
				}
				h.responseHandler.SaveMessagesToSession(h.sessionHandler, sessionID, &req, assistantContent, streamResult.InputTokens, streamResult.OutputTokens)
			}
		}
```

替换为:
```go
		if streamResult != nil {
			h.responseHandler.logRequestWithStreamingDetails(c, configID, openAIReq.Model, streamResult, startTime, "success", "", &req)
			if h.sessionHandler != nil && sessionID != "" {
				var assistantContent interface{}
				if len(streamResult.ToolCalls) > 0 {
					contentBlocks := make([]map[string]interface{}, 0)
					if streamResult.Content != "" {
						contentBlocks = append(contentBlocks, map[string]interface{}{
							"type": "text",
							"text": streamResult.Content,
						})
					}
					for _, tc := range streamResult.ToolCalls {
						contentBlocks = append(contentBlocks, tc)
					}
					assistantContent = contentBlocks
				} else {
					assistantContent = streamResult.Content
				}
				h.responseHandler.SaveMessagesToSession(h.sessionHandler, sessionID, &req, assistantContent, streamResult.InputTokens, streamResult.OutputTokens)
			}
		} else {
			// ConvertOpenAIStreamingToClaudeWithMapping returned nil: the
			// client disconnected mid-stream OR a terminal SSE error was
			// already sent to the client inside the converter. Do not write
			// anything else (the connection may already be closed). Logged so
			// the rate of disconnects is observable when verifying the
			// InvalidHTTPResponse fix.
			logger.Warn("  Stream ended without result for config %s (client disconnected or terminal error already sent)", configID)
		}
```

- [ ] **Step 3: 验证编译 + 全量测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test ./converter/ ./handler/ -count=1 -timeout 120s`
Expected:
  - Exit code: 0
  - Output does NOT contain: "FAIL" or "cannot"

- [ ] **Step 4: 质量门禁 — 交付前多维检查**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go vet ./converter/... ./handler/... && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "declared and not used"

- [ ] **Step 5: 提交**
Run: `git add backend/converter/response_converter.go backend/handler/handler.go && git commit -m "feat(streaming): bail out scanner on client disconnect and log nil stream results"`

---

## Self-Review Results

**Plan Type:** Bug Fix

| # | Check | Result | Action Taken |
|---|-------|--------|-------------|
| 1 | Header 含 Symptom + Root Cause + Impact? | PASS | Root Cause 精确到函数+行号(并发写 211/227;忽略错误 sse_utils.go:11-15;StopHeartbeat 不等待 39-44) |
| 2 | 每个 Task 标注 Depends on? | PASS | Task 1=None, Task 2/3=Task 1 |
| 3 | 每个 Task 3-8 Steps? | PASS | 6/6/5 |
| 4 | 无 TBD/模糊描述? | PASS | 全部含完整代码 |
| 5 | 跨 Task 类型/函数签名一致? | PASS | `sseWriter`/`bindSSEWriter`/`getSSEWriter`/`sseWriteClosed`/`sendSSE`/`Heartbeat.Stop` 全程统一 |
| 6 | 保存位置正确? | PASS | docs/superpowers/plans/ |
| 7 | 每个 Task 含质量门禁? | PASS | 编译 + vet + race/测试 |
| 8 | 未命中交付反模式? | PASS | 测试含并发(非 happy-path-only)+ 精确断言(非 truthy)+ 错误路径覆盖 |
| 9 | Step 1 是失败测试? | FIXED | Task 1 Step 3 创建并发/race 测试证明 bug;Bug Fix 类型用 -race 测试替代"先写失败测试"(Go race 无法用普通断言表达,改用 race detector + 长度断言) |
| 10 | 修复最小化? | PASS | 不改 emit* 签名(靠 closed flag 级联停止)、不改其他 SSE 路径 |
| 11 | 含回归测试? | PASS | Task 1 Step 3 + Task 2 Step 4(-race) |
| 12 | 无"顺手优化"? | PASS | 仅做修复 + 必要可观测;handler.go:599 的 defer-in-loop 不在本 Plan 范围 |

**Status:** ✅ ALL PASS

---

## Execution Selection

**Tasks:** 3
**Dependencies:** yes(Task 2/3 依赖 Task 1)
**User Preference:** none
**Decision:** Subagent-Driven
**Reasoning:** 3+ tasks 规则 → Subagent-Driven

**Limitations(本次未覆盖):**
- `handler.go:1059+` 的 `h.sendSSEEvent` 与 `claude/handlers/messages.go` 的 `sendSSEEvent` 是**另一套**顺序写入的 SSE 实现(无 heartbeat 并发),非 `/v1/messages` 主路径,本次不改。执行时若发现它们也是热路径且存在并发写,记录为新 Plan(Scope Guard)。
- Go HTTP stack 对客户端 TCP 断开的检测有固有延迟(TCP keepalive 探测间隔),closed flag 的置位依赖下一次写入失败;极端情况下可能有最多一次失败写入的少量字节残留,但该连接会被 Go 标记为 broken 不再复用,因此不会触发 InvalidHTTPResponse。

**Estimated Impact:** 修复后流式路径的并发写竞争彻底消除(互斥锁),断开后立即停止写入 + 心跳同步停止杜绝终止符后孤儿 ping。`InvalidHTTPResponse` 应从"每日数次"降至"接近零"。
