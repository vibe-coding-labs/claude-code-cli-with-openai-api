# Session 终止追踪与协议错误日志增强实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 增强通信日志记录，能够精确追踪 session 异常终止原因，记录协议解析错误的完整上下文。

**Architecture:** 流式响应 → 状态机检测终止原因 → 写入 proxy_errors 表 → 提供 API 查询异常 session。协议错误 → 捕获原始 SSE 数据 → 保存到 error 上下文字段 → 支持事后分析。

**Tech Stack:** Go 1.24, SQLite 3, Gin 1.10, SSE (text/event-stream)

**Risks:**
- 流式响应增加日志可能影响吞吐 → 缓解：使用已有的 AsyncLogger 异步写入
- 日志存储增长 → 缓解：保留 7 天，复用已有 CleanupOldProxyErrors 清理机制
- 修改表结构需要迁移 → 缓解：新增字段使用 DEFAULT 避免破坏性变更

---

### Task 1: 增强流式响应终止原因记录

**Depends on:** None
**Files:**
- Modify: `backend/converter/response_converter.go:461-494`（流式响应终止处理）
- Modify: `backend/database/proxy_errors.go:11-29`（ProxyError 结构体）
- Modify: `backend/database/schema.go`（表结构迁移）

- [ ] **Step 1: 添加 stream_end_reason 字段到 ProxyError 结构体**
文件: `backend/database/proxy_errors.go:11-29`

```go
// ProxyError represents a structured proxy error record
type ProxyError struct {
	ID                int64  `json:"id"`
	ConfigID          string `json:"config_id"`
	ConfigName        string `json:"config_name"`
	SessionID         string `json:"session_id,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	Model             string `json:"model"`
	UpstreamModel     string `json:"upstream_model,omitempty"`
	ErrorType         string `json:"error_type"`
	ErrorCategory     string `json:"error_category"`
	ErrorMessage      string `json:"error_message"`
	UpstreamStatusCode int   `json:"upstream_status_code,omitempty"`
	UpstreamErrorBody string `json:"upstream_error_body,omitempty"`
	RequestStage      string `json:"request_stage"`
	RetryAttempt      int    `json:"retry_attempt"`
	RequestDurationMs int64  `json:"request_duration_ms,omitempty"`
	RequestPreview    string `json:"request_preview,omitempty"`
	StreamEndReason   string `json:"stream_end_reason,omitempty"` // 新增：流式响应终止原因
	SSEContext        string `json:"sse_context,omitempty"`        // 新增：最后几个 SSE chunks
	CreatedAt         string `json:"created_at"`
}
```

- [ ] **Step 2: 添加数据库迁移脚本**
文件: `backend/database/migrations/008_add_stream_end_reason.sql`（新建）

```sql
-- +migrate Up
ALTER TABLE proxy_errors ADD COLUMN stream_end_reason TEXT DEFAULT '';
ALTER TABLE proxy_errors ADD COLUMN sse_context TEXT DEFAULT '';

-- +migrate Down
ALTER TABLE proxy_errors DROP COLUMN sse_context;
ALTER TABLE proxy_errors DROP COLUMN stream_end_reason;
```

- [ ] **Step 3: 在 response_converter 中记录流式终止原因**
文件: `backend/converter/response_converter.go:461-494`（select 语句块）

```go
// 流式终止原因常量
const (
	StreamEndNormal         = "normal"           // 正常完成
	StreamEndClientDisconnect = "client_disconnect" // 客户端断开
	StreamEndUpstreamError  = "upstream_error"   // 上游错误
	StreamEndProtocolError  = "protocol_error"   // 协议解析失败
	StreamEndStallTimeout   = "stall_timeout"    // 上游无响应超时
	StreamEndMaxDuration    = "max_duration"     // 超过最大流式时长
)

// 在 LogProxyError 调用时传入 stream_end_reason 和 sse_context
// 例如在 case <-done: 分支：
// database.LogProxyError(&database.ProxyError{
//     ...
//     StreamEndReason: StreamEndNormal,
// })

// 在 case <-idleTimer.C: 分支：
// database.LogProxyError(&database.ProxyError{
//     ...
//     StreamEndReason: StreamEndStallTimeout,
// })
```

- [ ] **Step 4: 添加 SSE 上下文捕获器**
文件: `backend/converter/sse_context.go`（新建）

```go
package converter

import (
	"container/ring"
	"sync"
)

// SSEContextCapture captures the last N SSE chunks for error context
type SSEContextCapture struct {
	mu     sync.Mutex
	buffer *ring.Ring
	size   int
}

// NewSSEContextCapture creates a new capture buffer
func NewSSEContextCapture(size int) *SSEContextCapture {
	return &SSEContextCapture{
		buffer: ring.New(size),
		size:   size,
	}
}

// Add adds a chunk to the buffer
func (c *SSEContextCapture) Add(chunk string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buffer.Value = chunk
	c.buffer = c.buffer.Next()
}

// GetLastN returns the last N chunks as a single string
func (c *SSEContextCapture) GetLastN(n int) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var result string
	count := 0
	c.buffer.Do(func(v interface{}) {
		if v != nil && count < n {
			result += v.(string) + "\n"
			count++
		}
	})
	return result
}
```

- [ ] **Step 5: 更新 LogProxyError 函数签名**
文件: `backend/database/proxy_errors.go:53-82`

```go
// LogProxyError inserts a proxy error record into the database
func LogProxyError(err *ProxyError) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	// Truncate long fields
	if len(err.ErrorMessage) > 2000 {
		err.ErrorMessage = err.ErrorMessage[:2000]
	}
	if len(err.UpstreamErrorBody) > 4000 {
		err.UpstreamErrorBody = err.UpstreamErrorBody[:4000]
	}
	if len(err.RequestPreview) > 2000 {
		err.RequestPreview = err.RequestPreview[:2000]
	}
	if len(err.SSEContext) > 4000 {
		err.SSEContext = err.SSEContext[:4000]
	}

	_, dbErr := DB.Exec(`INSERT INTO proxy_errors
		(config_id, config_name, session_id, request_id, model, upstream_model,
		 error_type, error_category, error_message, upstream_status_code,
		 upstream_error_body, request_stage, retry_attempt, request_duration_ms,
		 request_preview, stream_end_reason, sse_context, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		err.ConfigID, err.ConfigName, err.SessionID, err.RequestID,
		err.Model, err.UpstreamModel, err.ErrorType, err.ErrorCategory,
		err.ErrorMessage, err.UpstreamStatusCode, err.UpstreamErrorBody,
		err.RequestStage, err.RetryAttempt, err.RequestDurationMs,
		err.RequestPreview, err.StreamEndReason, err.SSEContext,
		time.Now().UTC().Format(time.RFC3339),
	)
	return dbErr
}
```

- [ ] **Step 6: 验证数据库迁移**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && go build -o bin/proxy ./cmd/proxy`
Expected:
  - Exit code: 0
  - Output contains: "compiled" or no error message

- [ ] **Step 7: 提交**
Run: `git add backend/database/proxy_errors.go backend/database/migrations/ backend/converter/sse_context.go backend/converter/response_converter.go && git commit -m "feat(logging): add stream_end_reason and sse_context for session termination tracking"`

---

### Task 2: 添加异常 Session 查询 API

**Depends on:** Task 1
**Files:**
- Create: `backend/handler/session_debug_api.go`
- Modify: `backend/cmd/proxy/main.go`（路由注册）

- [ ] **Step 1: 创建 session debug API handler**
文件: `backend/handler/session_debug_api.go`（新建）

```go
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// SessionDebugHandler handles session debugging endpoints
type SessionDebugHandler struct{}

// NewSessionDebugHandler creates a new handler
func NewSessionDebugHandler() *SessionDebugHandler {
	return &SessionDebugHandler{}
}

// GetAbnormalSessions returns sessions that ended abnormally
// GET /api/debug/sessions/abnormal?limit=20&hours=24
func (h *SessionDebugHandler) GetAbnormalSessions(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := parseInt(l, 1, 100); err == nil {
			limit = parsed
		}
	}

	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := parseInt(h, 1, 168); err == nil {
			hours = parsed
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	errors, err := database.GetProxyErrors("", "", "", limit, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Filter to only abnormal terminations
	var abnormal []database.ProxyError
	for _, e := range errors {
		if e.StreamEndReason != "" && e.StreamEndReason != "normal" {
			abnormal = append(abnormal, e)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    len(abnormal),
		"hours":    hours,
		"sessions": abnormal,
	})
}

// GetSessionErrors returns all errors for a specific session
// GET /api/debug/sessions/:session_id/errors
func (h *SessionDebugHandler) GetSessionErrors(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	errors, err := database.GetProxyErrorsBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"count":      len(errors),
		"errors":     errors,
	})
}

// GetProtocolErrors returns recent protocol parsing errors
// GET /api/debug/errors/protocol?limit=20
func (h *SessionDebugHandler) GetProtocolErrors(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := parseInt(l, 1, 100); err == nil {
			limit = parsed
		}
	}

	errors, err := database.GetProxyErrors("", "protocol", "", limit, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":  len(errors),
		"errors": errors,
	})
}

func parseInt(s string, min, max int) (int, error) {
	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return 0, err
	}
	if result < min {
		return min, nil
	}
	if result > max {
		return max, nil
	}
	return result, nil
}
```

- [ ] **Step 2: 添加 GetProxyErrorsBySessionID 数据库函数**
文件: `backend/database/proxy_errors.go`（追加）

```go
// GetProxyErrorsBySessionID returns all proxy errors for a specific session
func GetProxyErrorsBySessionID(sessionID string) ([]ProxyError, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, config_id, config_name, COALESCE(session_id,''), COALESCE(request_id,''),
		model, COALESCE(upstream_model,''), error_type, error_category, error_message,
		COALESCE(upstream_status_code,0), COALESCE(upstream_error_body,''), request_stage,
		retry_attempt, COALESCE(request_duration_ms,0), COALESCE(request_preview,''),
		COALESCE(stream_end_reason,''), COALESCE(sse_context,''), created_at
		FROM proxy_errors WHERE session_id = ? ORDER BY created_at ASC`

	rows, err := DB.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy errors by session_id: %w", err)
	}
	defer rows.Close()

	var errors []ProxyError
	for rows.Next() {
		var e ProxyError
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ConfigID, &e.ConfigName, &e.SessionID, &e.RequestID,
			&e.Model, &e.UpstreamModel, &e.ErrorType, &e.ErrorCategory, &e.ErrorMessage,
			&e.UpstreamStatusCode, &e.UpstreamErrorBody, &e.RequestStage, &e.RetryAttempt,
			&e.RequestDurationMs, &e.RequestPreview, &e.StreamEndReason, &e.SSEContext, &createdAt); err != nil {
			continue
		}
		e.CreatedAt = createdAt
		errors = append(errors, e)
	}
	return errors, nil
}
```

- [ ] **Step 3: 注册路由到 main.go**
文件: `backend/cmd/proxy/main.go:XXX`（在路由注册区块添加）

```go
// Session debug endpoints
debugHandler := handler.NewSessionDebugHandler()
api.GET("/debug/sessions/abnormal", debugHandler.GetAbnormalSessions)
api.GET("/debug/sessions/:session_id/errors", debugHandler.GetSessionErrors)
api.GET("/debug/errors/protocol", debugHandler.GetProtocolErrors)
```

- [ ] **Step 4: 验证 API 端点**
Run: `curl -s http://localhost:54988/api/debug/sessions/abnormal?limit=5 | jq '.total'`
Expected:
  - Exit code: 0
  - Output contains: a number (may be 0 if no abnormal sessions)

- [ ] **Step 5: 提交**
Run: `git add backend/handler/session_debug_api.go backend/database/proxy_errors.go backend/cmd/proxy/main.go && git commit -m "feat(api): add session debug endpoints for abnormal termination tracking"`

---

### Task 3: 增强协议错误日志上下文

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/response_converter.go:309-325`（chunk parse error 处理）
- Modify: `backend/handler/response_handler.go`（错误日志记录）

- [ ] **Step 1: 在 chunk parse error 时捕获 SSE 上下文**
文件: `backend/converter/response_converter.go:309-325`

```go
// 在 json.Unmarshal 失败时，记录完整的 chunk 数据
if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
	state.chunkErrors++
	// 记录协议错误到数据库（仅首次）
	if state.chunkErrors == 1 {
		database.LogProxyError(&database.ProxyError{
			ConfigID:       originalReq.Metadata.SessionID, // 从 metadata 获取
			Model:          originalReq.Model,
			ErrorType:      "parse_error",
			ErrorCategory:  database.ErrorCategoryProtocol,
			ErrorMessage:   fmt.Sprintf("SSE chunk parse error: %v", err),
			RequestStage:   database.StageStreaming,
			SSEContext:     collectedContent.String(), // 已收集的内容
		})
	}
	if state.chunkErrors <= 5 {
		logger.Warn("[converter] chunk parse error #%d: %v (data: %.100s)", state.chunkErrors, err, chunkData)
	}
	if state.chunkErrors > 50 {
		logger.Warn("[converter] too many chunk errors (%d), aborting stream", state.chunkErrors)
		return
	}
	continue
}
```

- [ ] **Step 2: 在流式响应结束时统一记录错误**
文件: `backend/converter/response_converter.go:461-494`（修改每个 case 分支）

```go
select {
case <-done:
	// 正常完成
	logger.Info("[stream] upstream completed normally after %v", time.Since(streamStart))
	// 如果有 chunk errors，记录为 protocol error
	if state.chunkErrors > 0 {
		database.LogProxyError(&database.ProxyError{
			ConfigID:         getConfigIDFromContext(c),
			ConfigName:       getConfigNameFromContext(c),
			SessionID:        c.GetHeader("X-Session-ID"),
			Model:            originalReq.Model,
			ErrorType:        "chunk_errors",
			ErrorCategory:    database.ErrorCategoryProtocol,
			ErrorMessage:     fmt.Sprintf("%d SSE chunk parse errors during stream", state.chunkErrors),
			RequestStage:     database.StageStreaming,
			StreamEndReason:  StreamEndNormal,
			RequestDurationMs: time.Since(streamStart).Milliseconds(),
		})
	}

case err := <-errChan:
	// 上游或扫描错误
	streamEndReason := StreamEndUpstreamError
	if strings.Contains(err.Error(), "client disconnected") {
		streamEndReason = StreamEndClientDisconnect
	}
	database.LogProxyError(&database.ProxyError{
		ConfigID:         getConfigIDFromContext(c),
		ConfigName:       getConfigNameFromContext(c),
		SessionID:        c.GetHeader("X-Session-ID"),
		Model:            originalReq.Model,
		ErrorType:        "stream_error",
		ErrorCategory:    classifyStreamError(err),
		ErrorMessage:     err.Error(),
		RequestStage:     database.StageStreaming,
		StreamEndReason:  streamEndReason,
		RequestDurationMs: time.Since(streamStart).Milliseconds(),
		SSEContext:       collectedContent.String(), // 已收集内容
	})

	// ... 发送错误响应 ...
}
```

- [ ] **Step 3: 添加辅助函数提取 config 信息**
文件: `backend/converter/response_converter.go`（文件末尾追加）

```go
// getConfigIDFromContext extracts config ID from gin context
func getConfigIDFromContext(c *gin.Context) string {
	if configID, exists := c.Get("config_id"); exists {
		if id, ok := configID.(string); ok {
			return id
		}
	}
	return ""
}

// getConfigNameFromContext extracts config name from gin context
func getConfigNameFromContext(c *gin.Context) string {
	if configName, exists := c.Get("config_name"); exists {
		if name, ok := configName.(string); ok {
			return name
		}
	}
	return ""
}

// classifyStreamError maps streaming errors to error categories
func classifyStreamError(err error) string {
	errMsg := err.Error()
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline") {
		return database.ErrorCategoryTimeout
	}
	if strings.Contains(errMsg, "connection") || strings.Contains(errMsg, "network") {
		return database.ErrorCategoryNetwork
	}
	return database.ErrorCategoryUpstream
}
```

- [ ] **Step 4: 验证协议错误日志**
Run: `curl -s http://localhost:54988/api/debug/errors/protocol?limit=3 | jq '.errors[0].sse_context'`
Expected:
  - Exit code: 0
  - Output contains: SSE context data (may be empty if no recent errors)

- [ ] **Step 5: 提交**
Run: `git add backend/converter/response_converter.go && git commit -m "feat(logging): capture SSE context on protocol errors for debugging"`

---

### Task 4: 添加 Session 停止原因统计视图

**Depends on:** Task 2
**Files:**
- Modify: `backend/handler/session_debug_api.go`（添加统计端点）
- Modify: `database/proxy_errors.go`（添加统计查询）

- [ ] **Step 1: 添加停止原因统计函数**
文件: `backend/database/proxy_errors.go`（追加）

```go
// GetStreamEndReasonStats returns statistics on stream termination reasons
func GetStreamEndReasonStats(since time.Time) ([]map[string]interface{}, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT stream_end_reason, COUNT(*) as count
		FROM proxy_errors
		WHERE created_at >= ? AND stream_end_reason != ''
		GROUP BY stream_end_reason
		ORDER BY count DESC`

	rows, err := DB.Query(query, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to query stream end reason stats: %w", err)
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var reason string
		var count int
		if err := rows.Scan(&reason, &count); err != nil {
			continue
		}
		stats = append(stats, map[string]interface{}{
			"stream_end_reason": reason,
			"count":             count,
		})
	}
	return stats, nil
}
```

- [ ] **Step 2: 添加统计 API 端点**
文件: `backend/handler/session_debug_api.go`（追加方法）

```go
// GetStreamEndStats returns statistics on why streams ended
// GET /api/debug/stats/stream-end?hours=24
func (h *SessionDebugHandler) GetStreamEndStats(c *gin.Context) {
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := parseInt(h, 1, 168); err == nil {
			hours = parsed
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := database.GetStreamEndReasonStats(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"hours":     hours,
		"generated": time.Now().Format(time.RFC3339),
		"stats":     stats,
	})
}
```

- [ ] **Step 3: 注册统计路由**
文件: `backend/cmd/proxy/main.go`（在路由注册区块添加）

```go
api.GET("/debug/stats/stream-end", debugHandler.GetStreamEndStats)
```

- [ ] **Step 4: 验证统计端点**
Run: `curl -s http://localhost:54988/api/debug/stats/stream-end?hours=24 | jq '.'`
Expected:
  - Exit code: 0
  - Output contains: JSON with "stats" array

- [ ] **Step 5: 提交**
Run: `git add backend/database/proxy_errors.go backend/handler/session_debug_api.go backend/cmd/proxy/main.go && git commit -m "feat(stats): add stream end reason statistics endpoint"`