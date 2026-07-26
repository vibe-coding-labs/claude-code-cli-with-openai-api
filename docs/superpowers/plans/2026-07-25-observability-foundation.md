# 完善日志记录 - 建立请求与会话的可追溯关联 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 建立 `request_logs` 与 `sessions` 的关联，使每个请求都能追溯到所属会话，为后续异常分析提供数据基础。

**Architecture:** 
1. 数据库层：添加 `session_id` 字段到 `request_logs` 表，添加索引
2. Handler 层：在请求处理开始时获取/生成 session_id，在日志记录时传递
3. 流式路径：确保流式响应完成时 session_id 被正确记录
4. 非流式路径：同上

**Tech Stack:** Go 1.22, SQLite, GORM

**Risks:**
- 迁移可能影响现有数据 — 使用 ALTER TABLE ADD COLUMN（不影响现有记录）
- 现有请求没有 session_id — 新字段允许 NULL，不影响现有数据
- 性能影响 — 添加索引后查询更快，写入略慢但可接受

---

## 设计决策

### 为什么不直接在现有架构上打补丁？

当前的"无头苍蝇"式解决问题：
- 看到 DSML 标记就加检测
- 看到空内容就加重试
- 看到什么问题就修什么

这种做法的问题：
1. **无法回溯** — 不知道哪些请求有问题
2. **无法统计** — 不知道问题发生的频率
3. **无法分析** — 不知道问题的分布和规律
4. **无法验证** — 不知道修复是否有效

### 正确的方法论

```
┌─────────────────────────────────────────────────────────────┐
│ 可观测性基础设施                                            │
│   完整记录 → 关联数据 → 可追溯 → 可分析                        │
├─────────────────────────────────────────────────────────────┤
│ 异常检测机制                                                │
│   定义异常模式 → 定时扫描 → 提取典型案例                        │
├─────────────────────────────────────────────────────────────┤
│ 针对性优化                                                  │
│   基于真实案例 → 分析根因 → 实现修复 → 验证效果                 │
├─────────────────────────────────────────────────────────────┤
│ 持续迭代                                                    │
│   监控指标 → 发现新问题 → 回到第一步                           │
└─────────────────────────────────────────────────────────────┘
```

本次计划只完成 **Phase 1：建立可追溯的数据关联**。

---

### Task 1: 数据库迁移 - 添加 session_id 到 request_logs

**Depends on:** None
**Files:**
- Create: `backend/database/migrations/038_add_session_id_to_request_logs.sql`
- Modify: `backend/database/types.go` (RequestLog 结构体)

- [ ] **Step 1: 创建迁移文件 — 添加 session_id 字段和索引**

```sql
-- backend/database/migrations/038_add_session_id_to_request_logs.sql
-- Add session_id to request_logs for request-session correlation

-- Add session_id column (nullable, existing records will have NULL)
ALTER TABLE request_logs ADD COLUMN session_id TEXT;

-- Create index for efficient session-based queries
CREATE INDEX IF NOT EXISTS idx_request_logs_session_id ON request_logs(session_id);

-- Create composite index for common query patterns
CREATE INDEX IF NOT EXISTS idx_request_logs_config_session ON request_logs(config_id, session_id);
```

- [ ] **Step 2: 修改 RequestLog 结构体 — 添加 SessionID 字段**
文件: `backend/database/types.go` (RequestLog 结构体定义)

在现有字段之后添加 `SessionID` 字段：

```go
// 文件: backend/database/types.go
// 在 RequestLog 结构体中添加

type RequestLog struct {
	ID              int64     `json:"id"`
	ConfigID        string    `json:"config_id"`
	UserID          int64     `json:"user_id"`
	SessionID       *string   `json:"session_id,omitempty"` // 新增：关联会话ID
	Model           string    `json:"model"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	CacheReadTokens int       `json:"cache_read_tokens"`
	CacheWriteTokens int      `json:"cache_write_tokens"`
	TotalTokens     int       `json:"total_tokens"`
	DurationMs      int       `json:"duration_ms"`
	Status          string    `json:"status"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	RequestBody     string    `json:"request_body,omitempty"`
	ResponseBody    string    `json:"response_body,omitempty"`
	RequestSummary  string    `json:"request_summary,omitempty"`
	ResponsePreview string    `json:"response_preview,omitempty"`
	ClientIP        string    `json:"client_ip,omitempty"`
	UserAgent       string    `json:"user_agent,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
```

- [ ] **Step 3: 验证迁移**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./database/...`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/database/migrations/038_add_session_id_to_request_logs.sql backend/database/types.go && git commit -m "feat(database): add session_id to request_logs for request-session correlation"`

---

### Task 2: 修改日志记录函数 - 传递 session_id

**Depends on:** Task 1
**Files:**
- Modify: `backend/database/async_logger.go` (LogRequestAsync/LogRequestSync)
- Modify: `backend/database/models.go` (LogRequest 函数)

- [ ] **Step 1: 修改异步日志写入 — 支持传递 session_id**
文件: `backend/database/async_logger.go`

修改 `LogRequestAsync` 函数签名，添加 sessionID 参数：

```go
// 文件: backend/database/async_logger.go
// 修改 LogRequestAsync 函数

func (l *AsyncLogger) LogRequestAsync(configID string, userID int64, sessionID *string, model string,
	inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int,
	durationMs int, status, errorMessage string,
	requestBody, responseBody, requestSummary, responsePreview string,
	clientIP, userAgent string) {
	// ... 现有实现，添加 sessionID 到 RequestLog
	log := &RequestLog{
		ConfigID:        configID,
		UserID:          userID,
		SessionID:       sessionID, // 新增
		Model:           model,
		// ... 其他字段
	}
	l.logQueue <- log
}
```

- [ ] **Step 2: 修改同步日志写入 — 支持传递 session_id**
文件: `backend/database/async_logger.go` (LogRequestSync 函数)

同样修改 `LogRequestSync` 函数：

```go
func LogRequestSync(db *sql.DB, configID string, userID int64, sessionID *string, model string,
	inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int,
	durationMs int, status, errorMessage string,
	requestBody, responseBody, requestSummary, responsePreview string,
	clientIP, userAgent string) error {
	// ... 现有实现，添加 sessionID 到 INSERT 语句
	query := `INSERT INTO request_logs (
		config_id, user_id, session_id, model,
		input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		total_tokens, duration_ms, status, error_message,
		request_body, response_body, request_summary, response_preview,
		client_ip, user_agent
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	_, err := db.Exec(query,
		configID, userID, sessionID, model,
		inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens,
		inputTokens+outputTokens, durationMs, status, errorMessage,
		requestBody, responseBody, requestSummary, responsePreview,
		clientIP, userAgent,
	)
	return err
}
```

- [ ] **Step 3: 验证构建**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./database/...`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/database/async_logger.go && git commit -m "feat(database): pass session_id in log functions"`

---

### Task 3: Handler 层集成 - 在请求处理中传递 session_id

**Depends on:** Task 2
**Files:**
- Modify: `backend/handler/response_handler.go` (logRequestWithDetails, logRequestWithStreamingDetails)
- Modify: `backend/handler/handler.go` (主处理器，获取 session_id)

- [ ] **Step 1: 修改 logRequestWithDetails — 添加 sessionID 参数并传递**
文件: `backend/handler/response_handler.go`

找到 `logRequestWithDetails` 函数，添加 sessionID 参数：

```go
// 文件: backend/handler/response_handler.go
// 修改 logRequestWithDetails 函数签名

func (r *ResponseHandler) logRequestWithDetails(
	c *gin.Context,
	configID string,
	sessionID *string, // 新增参数
	model string,
	inputTokens, outputTokens int,
	startTime time.Time,
	status string,
	errorMessage string,
	claudeReq *models.ClaudeMessagesRequest,
	claudeResp *models.ClaudeResponse,
) {
	duration := time.Since(startTime).Milliseconds()

	// ... 提取 requestBody, responseBody 等现有逻辑

	// 调用异步日志，传递 sessionID
	database.LogRequestAsync(
		configID,
		userID,
		sessionID, // 新增
		model,
		inputTokens,
		outputTokens,
		// ... 其他参数
	)
}
```

- [ ] **Step 2: 修改 logRequestWithStreamingDetails — 添加 sessionID 参数**
文件: `backend/handler/response_handler.go`

同样修改流式日志函数：

```go
func (r *ResponseHandler) logRequestWithStreamingDetails(
	c *gin.Context,
	configID string,
	sessionID *string, // 新增参数
	model string,
	streamResult *converter.StreamingResult,
	startTime time.Time,
	status string,
	errorMessage string,
	claudeReq *models.ClaudeMessagesRequest,
) {
	// ... 现有逻辑，传递 sessionID 到数据库日志
}
```

- [ ] **Step 3: 在 handler.go 中获取并传递 session_id**
文件: `backend/handler/handler.go`

在请求处理流程中，获取 session_id 并传递给日志函数：

```go
// 文件: backend/handler/handler.go
// 在 HandleMessages 函数中

// 获取或创建 session
sessionID := ""
if sessionHandler != nil {
	session, err := sessionHandler.GetOrCreateSession(c, claudeReq, configID, userID)
	if err == nil && session != nil {
		sessionID = session.ID
	}
}

// 传递 sessionID 到响应处理器
var sessionIDPtr *string
if sessionID != "" {
	sessionIDPtr = &sessionID
}

// 调用日志函数时传递 sessionIDPtr
```

- [ ] **Step 4: 验证构建**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./handler/...`
Expected:
  - Exit code: 0

- [ ] **Step 5: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/handler/response_handler.go backend/handler/handler.go && git commit -m "feat(handler): pass session_id through request logging pipeline"`

---

### Task 4: 添加按 session 查询请求日志的功能

**Depends on:** Task 3
**Files:**
- Modify: `backend/database/logs.go` (添加查询函数)
- Create: `backend/database/logs_test.go` (测试)

- [ ] **Step 1: 添加 GetRequestLogsBySession 函数**
文件: `backend/database/logs.go`

```go
// 文件: backend/database/logs.go
// 添加新函数

// GetRequestLogsBySession retrieves all request logs for a given session.
// This is useful for analyzing the request history of a conversation.
func GetRequestLogsBySession(db *sql.DB, sessionID string, limit int) ([]RequestLog, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT id, config_id, user_id, session_id, model,
		input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		total_tokens, duration_ms, status, error_message,
		request_body, response_body, request_summary, response_preview,
		client_ip, user_agent, created_at
		FROM request_logs
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT ?`

	rows, err := db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(
			&log.ID, &log.ConfigID, &log.UserID, &log.SessionID, &log.Model,
			&log.InputTokens, &log.OutputTokens, &log.CacheReadTokens, &log.CacheWriteTokens,
			&log.TotalTokens, &log.DurationMs, &log.Status, &log.ErrorMessage,
			&log.RequestBody, &log.ResponseBody, &log.RequestSummary, &log.ResponsePreview,
			&log.ClientIP, &log.UserAgent, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// GetSessionsWithErrors retrieves sessions that have error requests.
// This is useful for identifying problematic conversations.
func GetSessionsWithErrors(db *sql.DB, since time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT DISTINCT session_id
		FROM request_logs
		WHERE session_id IS NOT NULL
		  AND status = 'error'
		  AND created_at >= ?
		ORDER BY created_at DESC
		LIMIT ?`

	rows, err := db.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		sessionIDs = append(sessionIDs, id)
	}
	return sessionIDs, nil
}
```

- [ ] **Step 2: 添加测试**
文件: `backend/database/logs_test.go`

```go
package database

import (
	"testing"
	"time"
)

func TestGetRequestLogsBySession_Empty(t *testing.T) {
	// This test verifies the function works with empty result
	logs, err := GetRequestLogsBySession(testDB, "nonexistent_session", 10)
	if err != nil {
		t.Fatalf("GetRequestLogsBySession failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("Expected empty result, got %d logs", len(logs))
	}
}

func TestGetSessionsWithErrors_Empty(t *testing.T) {
	sessions, err := GetSessionsWithErrors(testDB, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("GetSessionsWithErrors failed: %v", err)
	}
	// Result may be empty or have sessions depending on test data
	_ = sessions
}
```

- [ ] **Step 3: 验证构建和测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./database/... && go test ./database/ -run TestGetRequestLogsBySession -v`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/database/logs.go backend/database/logs_test.go && git commit -m "feat(database): add session-based request log queries"`

---

### Task 5: 端到端测试和验证

**Depends on:** Task 4
**Files:**
- Modify: `backend/handler/handler_test.go` (如有) 或创建新测试

- [ ] **Step 1: 验证完整流程**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./... && go test ./... 2>&1 | tail -20`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

- [ ] **Step 2: 手动验证数据库迁移**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && sqlite3 data/proxy.db ".schema request_logs" | grep session_id`
Expected:
  - Output contains: "session_id TEXT"

- [ ] **Step 3: 提交最终变更**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git status && git diff --stat`

---

## 后续工作（不在本次计划中）

完成本次计划后，可以继续：

1. **Phase 2: 异常会话分析**
   - 定时扫描 `request_logs` 表，找出 `status='error'` 的会话
   - 提取请求体/响应体进行分析
   - 分类异常类型（超时、截断、退化输出、空内容等）

2. **Phase 3: 针对性优化**
   - 基于真实异常案例，针对性修复
   - 添加更多无效输出检测模式
   - 优化重试策略

3. **Phase 4: 监控和告警**
   - 添加异常率监控
   - 配置告警阈值
   - 建立问题响应流程