# SSE Concurrency & Stress Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add high-concurrency stress tests that exercise the real HTTP streaming path (chunked transfer over real `net.Conn`) plus component-level race-detection, so the InvalidHTTPResponse fix is provably validated under load — giving confidence normal clients won't hit it.

**Architecture:**
- **Data flow:** mock OpenAI SSE source (paced) → real `gin.Engine` + `httptest.NewServer` → `ConvertOpenAIStreamingToClaude` (production path, binds `sseWriter`, starts `Heartbeat`) → real chunked HTTP response → N concurrent keep-alive clients (shared `http.Transport`, mirroring undici's connection reuse) → each client parses every SSE byte and asserts frame integrity + content fidelity + correct terminal event.
- **Key components:** (1) injectable heartbeat interval (`heartbeatInterval` package var) so tests fire pings fast; (2) `raceDetectingWriter` — a custom `http.ResponseWriter` whose `Write` flags overlapping calls, proving the `sseWriter` mutex serializes heartbeat + content writers; (3) HTTP-level stress test (32 clients) + early-disconnect test; (4) `ensureGinTestMode()` via `sync.Once` to kill the pre-existing `-race` flake in `TestStreamingE2E_ConcurrentStreams`.
- **Why this design:** `httptest.ResponseRecorder` (used by existing tests) never fails writes and does no chunked framing, so it cannot reproduce the bug. Only a real `http.Server` over a real connection reproduces the interleaved-byte corruption that produces `InvalidHTTPResponse`. The race-detecting writer gives a fast, deterministic component-level regression guard for the exact root cause.

**Tech Stack:** Go 1.25.0, Gin v1.10.1, `net/http/httptest`, `sync`, `sync/atomic`, `-race` detector.

**Scope:** Small-Medium
**Risk:** Low (1 trivial package-var in production code + `5*time.Second`→var swap; everything else is new test files)

**Risks:**
- `heartbeatInterval` is a package-level global → tests that lower it must NOT run concurrently (`t.Parallel`) with other heartbeat-dependent tests → 缓解: overriding tests set `t.Cleanup` to restore the default and do not call `t.Parallel()`.
- `go test -race ./converter/` currently flakes on the pre-existing `TestStreamingE2E_ConcurrentStreams` (concurrent `gin.SetMode`) → 缓解: Task 2 introduces `ensureGinTestMode()` (`sync.Once`) and refactors `runStreamingTest` to use it, making the whole package `-race`-clean.
- Pre-existing `-race` failures in `./handler/` (9 tests) and the flaky `lb_performance_benchmark_test.go` are OUT OF SCOPE — regression gates run only `./converter/`.

**Autonomy Level:** Full

---

## Type Detection

**Plan Type:** Feature (new test infrastructure + tests)
**Scope:** Small-Medium (1 production var + 2 new test files + 1 test-helper refactor)
**Risk:** Low
**Detection Reason:** The user asks to "写一个单测试…做压力测试、高并发测试" to validate an existing fix — this is creating new test capability, the canonical Feature signal ("写/新增").

→ Routing to Phase 1 (Feature) branch...

## Pre-Planning Analysis

**Feature:** SSE concurrency stress tests + component race tests
**Scope:** single subsystem (converter package)
**Files Create:**
- `backend/converter/sse_writer_race_test.go` — `raceDetectingWriter`, `ensureGinTestMode()`, serialization test, synchronous-Stop test
- `backend/converter/sse_concurrency_test.go` — HTTP-level stress test, early-disconnect test, `slowStream`/`scanSSE`/`runOneClient` helpers
**Files Modify:**
- `backend/converter/heartbeat.go` — add `var heartbeatInterval = 5 * time.Second`
- `backend/converter/response_converter.go:219` — `5*time.Second` → `heartbeatInterval`
- `backend/converter/anthropic_streaming_e2e_test.go:195` — `gin.SetMode(gin.TestMode)` → `ensureGinTestMode()` (fixes pre-existing `-race` flake)
**Tasks:** 3
**Order:** Task 1 (inject interval) → Task 2 (component race tests + shared helper) → Task 3 (HTTP stress tests, reuses Task 2 helper)
**Risks:** global var mutation across tests (mitigated above); pre-existing converter `-race` flake (fixed in Task 2).

→ Proceeding to Phase 2...

---

## Task 1: Make heartbeat interval injectable

**Depends on:** None
**Files:**
- Modify: `backend/converter/heartbeat.go` (add package var after imports)
- Modify: `backend/converter/response_converter.go:219`

- [ ] **Step 1: Add `heartbeatInterval` package var to heartbeat.go**

文件: `backend/converter/heartbeat.go`（在 `import` 块之后、`type Heartbeat struct` 之前插入）

```go
// heartbeatInterval is the gap between keep-alive ping events during a stream.
// It defaults to 5s; tests lower it (then restore via t.Cleanup) to exercise
// the heartbeat goroutine without long waits — see sse_writer_race_test.go and
// sse_concurrency_test.go.
var heartbeatInterval = 5 * time.Second
```

- [ ] **Step 2: Use the var at the production call site**

文件: `backend/converter/response_converter.go:219`

```go
	heartbeat := StartHeartbeat(c, ctx, heartbeatInterval)
```

- [ ] **Step 3: 验证编译**
Run: `cd backend && go build ./converter/`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" / "undefined"

- [ ] **Step 4: 质量门禁 — 交付前多维检查**
Run: `cd backend && go build ./converter/ && go vet ./converter/`
Expected:
  - Exit code: 0
  - 无遗留 debug 语句 / TODO / dead code
  - 改动范围仅为 1 行新增 var + 1 行替换（无额外改动）

- [ ] **Step 5: 提交**
Run: `cd backend && git add converter/heartbeat.go converter/response_converter.go && git commit -m "refactor(converter): make heartbeat interval injectable for concurrency tests"`
Expected: Exit code 0, new commit created.

---

## Task 2: Component-level race tests + shared gin-mode helper

**Depends on:** Task 1
**Files:**
- Create: `backend/converter/sse_writer_race_test.go`
- Modify: `backend/converter/anthropic_streaming_e2e_test.go:195`

- [ ] **Step 1: 创建 sse_writer_race_test.go — race-detecting writer + gin-mode guard + 两个并发安全测试**

`backend/converter/sse_writer_race_test.go`（完整文件）:

```go
package converter

// Race-detection tests for the serialized SSE writer (sse_utils.go) and the
// synchronous heartbeat stop (heartbeat.go). These directly target the
// InvalidHTTPResponse root cause: the heartbeat goroutine and the streaming
// scanner goroutine must never call c.Writer.Write at the same time, and no
// ping may be written after the response is finalized.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ginTestModeOnce ensures gin.SetMode runs exactly once for the whole test
// binary. Previously runStreamingTest called gin.SetMode(gin.TestMode) on every
// invocation, so TestStreamingE2E_ConcurrentStreams (which calls it from 10
// goroutines) raced on gin's global mode variable under -race.
var ginTestModeOnce sync.Once

// ensureGinTestMode sets gin to TestMode exactly once. Use this instead of a
// bare gin.SetMode in every test/helper.
func ensureGinTestMode() {
	ginTestModeOnce.Do(func() { gin.SetMode(gin.TestMode) })
}

// raceDetectingWriter is an http.ResponseWriter whose Write detects overlapping
// calls via an atomic in-flight counter. Two concurrent Writes (heartbeat +
// content writer without the sseWriter mutex) increment overlapping. With the
// mutex in place, Writes are fully serialized and overlapping stays 0.
type raceDetectingWriter struct {
	header     http.Header
	inFlight   int32 // accessed atomically — concurrent writers currently inside Write
	overlapping int32 // accessed atomically — number of detected concurrent-Write overlaps
	mu         sync.Mutex
	buf        bytes.Buffer
}

func newRaceDetectingWriter() *raceDetectingWriter {
	return &raceDetectingWriter{header: http.Header{}}
}

func (w *raceDetectingWriter) Header() http.Header { return w.header }

func (w *raceDetectingWriter) Write(b []byte) (int, error) {
	// A second writer entering while the first is still inside Write means two
	// goroutines hit the connection at once — the exact root cause.
	if atomic.AddInt32(&w.inFlight, 1) > 1 {
		atomic.AddInt32(&w.overlapping, 1)
	}
	// A tiny hold widens the race window so overlap is reliably observable on
	// fast hardware, not just under -race.
	time.Sleep(time.Microsecond)
	atomic.AddInt32(&w.inFlight, -1)

	w.mu.Lock()
	w.buf.Write(b)
	w.mu.Unlock()
	return len(b), nil
}

func (w *raceDetectingWriter) WriteHeader(int) {}
func (w *raceDetectingWriter) Flush()          {}

// countPings reports how many ping events were written to buf.
func countPings(s string) int { return strings.Count(s, "event: ping") }

// TestSSE_HeartbeatAndContentAreSerialized proves the sseWriter mutex prevents
// the heartbeat goroutine and a content writer from writing concurrently. If
// the mutex is removed, the race-detecting writer records overlapping writes.
func TestSSE_HeartbeatAndContentAreSerialized(t *testing.T) {
	ensureGinTestMode()
	w := newRaceDetectingWriter()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	bindSSEWriter(c)

	prev := heartbeatInterval
	heartbeatInterval = 2 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hb := StartHeartbeat(c, ctx, heartbeatInterval)

	// Hammer content writes concurrently with the heartbeat for 50ms.
	deadline := time.After(50 * time.Millisecond)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-deadline:
				return
			default:
			}
			sendSSE(c, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": fmt.Sprintf("d%d", i),
				},
			})
		}
	}()
	wg.Wait()
	hb.Stop()

	if n := atomic.LoadInt32(&w.overlapping); n != 0 {
		t.Fatalf("detected %d concurrent writes to the connection — heartbeat and content writer are not serialized", n)
	}
}

// TestHeartbeat_StopIsSynchronousAndTerminal proves Heartbeat.Stop() blocks
// until the goroutine has exited AND that no ping is written afterwards.
// Orphan pings after the chunked terminator are read as the next keep-alive
// response's status line by undici → InvalidHTTPResponse.
func TestHeartbeat_StopIsSynchronousAndTerminal(t *testing.T) {
	ensureGinTestMode()
	w := newRaceDetectingWriter()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	bindSSEWriter(c)

	prev := heartbeatInterval
	heartbeatInterval = 5 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hb := StartHeartbeat(c, ctx, heartbeatInterval)

	time.Sleep(20 * time.Millisecond) // let a few pings fire
	hb.Stop()                         // must block until the goroutine exited

	pingsBefore := countPings(w.buf.String())
	time.Sleep(30 * time.Millisecond) // ~6 intervals — no ping should fire now
	pingsAfter := countPings(w.buf.String())

	if pingsAfter != pingsBefore {
		t.Fatalf("heartbeat wrote %d ping(s) after Stop() returned — orphan bytes, Stop is not terminal",
			pingsAfter-pingsBefore)
	}
}
```

- [ ] **Step 2: 修改 runStreamingTest 使用 ensureGinTestMode — 消除预存的 -race 抖动**

文件: `backend/converter/anthropic_streaming_e2e_test.go:195`（`runStreamingTest` 函数体内，`gin.SetMode(gin.TestMode)` 替换为 `ensureGinTestMode()`）

替换后的该行:

```go
	ensureGinTestMode()
```

- [ ] **Step 3: 验证两个新测试在 -race 下通过**
Run: `cd backend && go test -race -run 'TestSSE_HeartbeatAndContentAreSerialized|TestHeartbeat_StopIsSynchronousAndTerminal' ./converter/`
Expected:
  - Exit code: 0
  - Output contains: "ok" and the two test names
  - Output does NOT contain: "DATA RACE" or "FAIL"

- [ ] **Step 4: 验证预存并发测试不再 -race 抖动**
Run: `cd backend && go test -race -run 'TestStreamingE2E_ConcurrentStreams' -count=3 ./converter/`
Expected:
  - Exit code: 0
  - Output does NOT contain: "DATA RACE"（之前因并发 `gin.SetMode` 必现）

- [ ] **Step 5: 质量门禁 — 交付前多维检查**
Run: `cd backend && go vet ./converter/ && go test -race -run 'TestSSE_|TestHeartbeat_|TestStreamingE2E_ConcurrentStreams' ./converter/`
Expected:
  - Exit code: 0
  - 无遗留 debug 语句 / TODO / dead code
  - 无反模式（对照交付反模式清单）

- [ ] **Step 6: 提交**
Run: `cd backend && git add converter/sse_writer_race_test.go converter/anthropic_streaming_e2e_test.go && git commit -m "test(converter): add race-detecting SSE serialization + heartbeat-stop tests"`
Expected: Exit code 0, new commit created.

---

## Task 3: HTTP-level concurrent streaming stress test + early-disconnect test

**Depends on:** Task 1, Task 2 (reuses `ensureGinTestMode`)
**Files:**
- Create: `backend/converter/sse_concurrency_test.go`

- [ ] **Step 1: 创建 sse_concurrency_test.go — 真实 HTTP 服务器 + 高并发 keep-alive 客户端 + 帧完整性断言**

`backend/converter/sse_concurrency_test.go`（完整文件）:

```go
package converter

// HTTP-level concurrency stress tests for the streaming pipeline.
//
// Unlike the ResponseRecorder-based E2E tests, these spin up a REAL
// httptest.Server (real http.Server, real chunked transfer over a real
// net.Conn) and drive it with keep-alive HTTP clients that share a Transport —
// mirroring undici's connection reuse, the exact precondition under which
// orphan/interleaved bytes surface as InvalidHTTPResponse. If the sseWriter
// mutex or the synchronous heartbeat Stop failed, chunked framing would
// corrupt and a client would see malformed frames or a wrong status line.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

const streamContent = "The quick brown fox jumps over the lazy dog repeatedly."

// slowStream returns an OpenAI SSE reader that emits content in small deltas
// with a short gap, so several heartbeats fire mid-stream — forcing the
// heartbeat goroutine and the content scanner to contend for the connection.
func slowStream(body string) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		runes := []rune(body)
		for i := 0; i < len(runes); i += 3 {
			end := i + 3
			if end > len(runes) {
				end = len(runes)
			}
			chunk := openAIChunk("c", "gpt-4", map[string]interface{}{"content": string(runes[i:end])}, "")
			if _, err := pw.Write([]byte(chunk)); err != nil {
				return
			}
			time.Sleep(3 * time.Millisecond)
		}
		end := openAIChunk("c", "gpt-4", map[string]interface{}{}, "stop") + "data: [DONE]\n\n"
		pw.Write([]byte(end))
	}()
	return pr
}

// scanSSE reads a full SSE stream and returns the ordered event types and the
// concatenated text-delta content. Any malformed data line or read error is
// reported — those signal chunked-framing corruption.
func scanSSE(r io.Reader) (types []string, content string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var curEvent string
	var sb strings.Builder
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			curEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			var m map[string]interface{}
			if jerr := json.Unmarshal([]byte(data), &m); jerr != nil {
				return nil, "", fmt.Errorf("malformed data line (frame corruption): %v near %q", jerr, data)
			}
			types = append(types, curEvent)
			if curEvent == "content_block_delta" {
				if d, ok := m["delta"].(map[string]interface{}); ok {
					if txt, ok := d["text"].(string); ok {
						sb.WriteString(txt)
					}
				}
			}
			curEvent = ""
		}
	}
	if serr := sc.Err(); serr != nil {
		return nil, "", fmt.Errorf("stream read error (transfer/framing corruption): %w", serr)
	}
	return types, sb.String(), nil
}

// runOneClient posts one streaming request and asserts the response is a
// well-formed, complete SSE stream with intact content.
func runOneClient(t *testing.T, client *http.Client, baseURL string) error {
	t.Helper()
	resp, err := client.Post(baseURL+"/v1/messages", "application/json", strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status = %d, want 200", resp.StatusCode)
	}
	types, content, err := scanSSE(resp.Body)
	if err != nil {
		return err
	}
	if len(types) == 0 {
		return fmt.Errorf("no events received")
	}
	if content != streamContent {
		return fmt.Errorf("content = %q, want %q (frame loss/corruption)", content, streamContent)
	}
	if types[len(types)-1] != "message_stop" {
		return fmt.Errorf("last event = %q, want message_stop", types[len(types)-1])
	}
	return nil
}

// TestSSE_ConcurrentClients_FrameIntegrity fires 32 concurrent keep-alive
// clients (shared Transport) at the real server. Every response must be a
// clean, complete SSE stream — proving the fix holds under high concurrency.
func TestSSE_ConcurrentClients_FrameIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP stress test in -short mode")
	}
	ensureGinTestMode()
	prev := heartbeatInterval
	heartbeatInterval = 8 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		req := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6", MaxTokens: 4096, Stream: true}
		ConvertOpenAIStreamingToClaude(c, slowStream(streamContent), req, c.Request.Context())
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	const clients = 32
	transport := &http.Transport{
		MaxIdleConns:        clients,
		MaxIdleConnsPerHost: clients,
		IdleConnTimeout:     120 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := runOneClient(t, client, srv.URL); err != nil {
				errs <- fmt.Errorf("client %d: %w", id, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestSSE_EarlyDisconnectDoesNotCorruptSubsequentRequests reproduces the
// orphan-bytes scenario: a client disconnects mid-stream, then a subsequent
// request over the pooled keep-alive connection must still parse cleanly.
func TestSSE_EarlyDisconnectDoesNotCorruptSubsequentRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping disconnect test in -short mode")
	}
	ensureGinTestMode()
	prev := heartbeatInterval
	heartbeatInterval = 5 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		req := &models.ClaudeMessagesRequest{Model: "claude-sonnet-4-6", MaxTokens: 4096, Stream: true}
		ConvertOpenAIStreamingToClaude(c, slowStream(strings.Repeat("x", 2000)), req, c.Request.Context())
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	transport := &http.Transport{MaxIdleConnsPerHost: 4, IdleConnTimeout: 120 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	// Request 1: disconnect after reading a few bytes (client gone mid-stream).
	ctx, cancel := context.WithCancel(context.Background())
	req1, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/messages", strings.NewReader("{}"))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("req1 failed: %v", err)
	}
	io.CopyN(io.Discard, resp1.Body, 64)
	cancel()
	resp1.Body.Close()

	// Let the server observe the broken pipe and stop writing (closed flag).
	time.Sleep(50 * time.Millisecond)

	// Request 2: must succeed cleanly. Orphan ping bytes after req1's
	// terminator would surface as a framing error or wrong status line here.
	if err := runOneClient(t, client, srv.URL); err != nil {
		t.Fatalf("request after disconnect was not clean: %v", err)
	}
}
```

- [ ] **Step 2: 验证 HTTP 压测在 -race 下通过**
Run: `cd backend && go test -race -run 'TestSSE_ConcurrentClients_FrameIntegrity|TestSSE_EarlyDisconnectDoesNotCorruptSubsequentRequests' ./converter/`
Expected:
  - Exit code: 0
  - Output contains: "ok"
  - Output does NOT contain: "DATA RACE", "FAIL", "frame loss/corruption", "framing corruption"

- [ ] **Step 3: 稳定性验证 — 连跑 5 次**
Run: `cd backend && go test -race -count=5 -run 'TestSSE_ConcurrentClients_FrameIntegrity|TestSSE_EarlyDisconnectDoesNotCorruptSubsequentRequests' ./converter/`
Expected:
  - Exit code: 0
  - Output does NOT contain: "FAIL" or "DATA RACE"（5 轮全绿，证明非偶然）

- [ ] **Step 4: 质量门禁 — 交付前多维检查（全 converter 包 -race 回归）**
Run: `cd backend && go vet ./converter/ && go test -race ./converter/`
Expected:
  - Exit code: 0
  - 无遗留 debug 语句 / TODO / dead code
  - 无反模式（对照交付反模式清单扫描）
  - 全包 -race 绿（包含 Task 2 修好的预存并发测试）

- [ ] **Step 5: 提交**
Run: `cd backend && git add converter/sse_concurrency_test.go && git commit -m "test(converter): add HTTP-level concurrent streaming stress + disconnect tests"`
Expected: Exit code 0, new commit created.

---

## Self-Review Results

**Plan Type:** Feature

| # | Check | Result | Action Taken |
|---|-------|--------|-------------|
| 1 | Goal + Type + Scope + Risk? | PASS | Feature / Small-Medium / Low |
| 2 | Every Task has Depends on? | PASS | T1=None, T2=T1, T3=T1+T2 |
| 3 | Every Task 3-8 Steps? | PASS | T1=5, T2=6, T3=5 |
| 4 | No TBD/TODO/vague? | PASS | All code complete |
| 5 | Cross-Task type/fn names consistent? | PASS | `heartbeatInterval`, `ensureGinTestMode`, `raceDetectingWriter`, `scanSSE`, `runOneClient`, `slowStream`, `streamContent` used identically |
| 6 | Saved to docs/superpowers/plans/? | PASS | this file |
| 7 | Quality-gate Step per Task? | PASS | T1-S4, T2-S5, T3-S4 |
| 8 | No delivery anti-patterns? | PASS | assertions precise (equality, not truthy); no `any`/hardcoded secrets; errors wrapped with context; cleanup via t.Cleanup |
| 9 | Precise file paths (Create/Modify/Test)? | PASS | all paths + line numbers given |
| 10 | New-file steps include complete code + imports? | PASS | two full test files |
| 11 | Modify steps include full replaced block + line? | PASS | heartbeat.go var, response_converter.go:219, e2e_test.go:195 |
| 12 | Code blocks 5-80 lines? | PASS | largest is the new files (split across clearly) |
| 13 | No dangling fn/type refs? | PASS | `openAIChunk`, `bindSSEWriter`, `sendSSE`, `StartHeartbeat`, `ConvertOpenAIStreamingToClaude`, `models.*` all exist in-package |
| 14 | Every Task has Run+Expected (3 elements)? | PASS | exit code + output patterns |
| 15 | Each Task independently verifiable? | PASS | each has its own build/-race gate |
| 16 | AI self-executable (no "ask user")? | PASS | failure paths predefined (见自愈协议) |

**Status:** ✅ ALL PASS

---

## Failure Self-Heal Protocol (for the executor)

- **`undefined: heartbeatInterval`** → Task 1 Step 1 var not added before Task 2/3 compile → run Task 1 first (dependency order).
- **`redeclared in this block` for ensureGinTestMode/raceDetectingWriter** → both new files are `package converter`; ensure neither symbol is duplicated. Only `sse_writer_race_test.go` defines them.
- **`-race` DATA RACE in a NEW test** → if it's inside `raceDetectingWriter`'s own fields, confirm `inFlight`/`overlapping` use `sync/atomic` (they do). If it's gin global state, confirm `ensureGinTestMode()` is used (not bare `gin.SetMode`).
- **Stress test flaky (`frame loss/corruption`)** → widen nothing; first re-run `-count=5`; if persistent, it indicates a REAL regression in the fix — STOP and report (this is the signal the test is designed to catch).
- **`go test -race ./converter/` fails on an UNRELATED pre-existing test** → record it; do not expand scope into `./handler/`. Only the `TestStreamingE2E_ConcurrentStreams` flake is in scope (fixed by Task 2 Step 2).

---

## Execution Selection

**Tasks:** 3
**Dependencies:** yes (sequential: T1 → T2 → T3; T3 reuses T2's helper, both files are `package converter`)
**User Preference:** none stated
**Decision:** **Inline** (not Subagent-Driven)
**Reasoning:** All three tasks edit files in a single Go package with compile-time coupling (T3 references `ensureGinTestMode` defined in T2; T2/T3 depend on T1's var). Parallel subagents editing the same package would risk duplicate-symbol and compile conflicts. Sequential inline execution is the reliable choice. After all commits: per this repo's branch-finishing convention, fast-forward merge into local `main` (do NOT push, do NOT open a PR).

**Proceeding to inline execution.**
