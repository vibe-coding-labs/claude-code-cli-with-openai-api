# Proxy Error Observability — Fix proxy_errors Table + Conversion Error Logging

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修复 proxy_errors 表不存在的问题，添加转换错误日志，建立代理端对格式错误的完整可观测性。

**Architecture:** 迁移文件修复 → 手动建表 → 转换层添加错误计数器+日志 → 状态机添加 panic recovery。数据流：upstream chunk → JSON 解析失败 → LogProxyError() 写入 proxy_errors 表 → response_preview 记录畸形数据 → 前端 dashboard 可查看错误。

**Tech Stack:** Go 1.22, SQLite 3, Gin HTTP framework

**Risks:**
- 修改 034 迁移文件不影响已运行的数据库（因为 schema_migrations 已记录），需手动建表
- 给 response_converter.go 的 silent skip 添加日志可能产生大量噪音 → 缓解：只记录前 N 个错误

---

### Task 1: Fix proxy_errors Table — 修复迁移文件标记 + 手动建表

**Depends on:** None
**Files:**
- Modify: `backend/database/migrations/034_create_proxy_errors_table.sql` (全文)

- [ ] **Step 1: 修复迁移文件标记 — 将 `-- UP` / `-- DOWN` 改为 `-- UP Migration` / `-- DOWN Migration`**

```sql
-- Migration: Create proxy_errors table for structured error logging
-- UP Migration

CREATE TABLE IF NOT EXISTS proxy_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_id TEXT NOT NULL,
    config_name TEXT NOT NULL,
    session_id TEXT,
    request_id TEXT,
    model TEXT NOT NULL,
    upstream_model TEXT,
    error_type TEXT NOT NULL,
    error_category TEXT NOT NULL,
    error_message TEXT NOT NULL,
    upstream_status_code INTEGER,
    upstream_error_body TEXT,
    request_stage TEXT NOT NULL,
    retry_attempt INTEGER DEFAULT 0,
    request_duration_ms INTEGER,
    request_preview TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxy_errors_config_id ON proxy_errors(config_id);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_error_type ON proxy_errors(error_type);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_error_category ON proxy_errors(error_category);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_created_at ON proxy_errors(created_at);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_model ON proxy_errors(model);
CREATE INDEX IF NOT EXISTS idx_proxy_errors_request_stage ON proxy_errors(request_stage);

-- DOWN Migration

DROP INDEX IF EXISTS idx_proxy_errors_config_id;
DROP INDEX IF EXISTS idx_proxy_errors_error_type;
DROP INDEX IF EXISTS idx_proxy_errors_error_category;
DROP INDEX IF EXISTS idx_proxy_errors_created_at;
DROP INDEX IF EXISTS idx_proxy_errors_model;
DROP INDEX IF NOT EXISTS idx_proxy_errors_request_stage;
DROP TABLE IF EXISTS proxy_errors;
```

- [ ] **Step 2: 手动在运行数据库中创建 proxy_errors 表 — 因为迁移已记录为 applied**

Run: `sqlite3 /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/data/proxy.db < /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend/database/migrations/034_create_proxy_errors_table.sql`
Expected:
  - Exit code: 0
  - Verify: `sqlite3 data/proxy.db ".tables"` contains `proxy_errors`

- [ ] **Step 3: 验证 proxy_errors 表已创建且 LogProxyError 可用**

Run: `sqlite3 /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/data/proxy.db "SELECT name FROM sqlite_master WHERE type='table' AND name='proxy_errors';"`
Expected:
  - Exit code: 0
  - Output contains: "proxy_errors"

- [ ] **Step 4: 提交**
Run: `git add backend/database/migrations/034_create_proxy_errors_table.sql && git commit -m "fix(db): correct migration 034 markers to prevent DROP TABLE execution"`

---

### Task 2: Add Conversion Error Logging — 修复静默吞噬畸形 chunk 的问题

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/response_converter.go:258-265` (JSON unmarshal 错误处理)

- [ ] **Step 1: 修改 chunk 解析错误处理 — 将 silent skip 改为计数+日志+条件跳过**

文件: `backend/converter/response_converter.go` (在 StreamConvertOpenAIToClaude 函数内，chunk 解析循环中)

找到以下代码块（约 258-265 行）：

```go
var chunk models.OpenAIResponse
if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
    continue
}
```

替换为：

```go
var chunk models.OpenAIResponse
if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
    state.chunkErrors++
    if state.chunkErrors <= 5 {
        log.Printf("[converter] chunk parse error #%d: %v (data: %.100s)", state.chunkErrors, err, chunkData)
    }
    if state.chunkErrors > 50 {
        log.Printf("[converter] too many chunk errors (%d), aborting stream", state.chunkErrors)
        return
    }
    continue
}
```

- [ ] **Step 2: 在 StreamingState 结构体中添加 chunkErrors 计数字段**

文件: `backend/converter/streaming_state.go` (StreamingState 结构体定义)

在 StreamingState 结构体中添加字段：

```go
chunkErrors int
```

- [ ] **Step 3: 验证编译通过**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "cannot"

- [ ] **Step 4: 提交**
Run: `git add backend/converter/response_converter.go backend/converter/streaming_state.go && git commit -m "feat(converter): log malformed chunk errors instead of silent skip"`

---

### Task 3: Add Panic Recovery in Streaming State Machine — 防止畸形数据导致进程崩溃

**Depends on:** Task 2
**Files:**
- Modify: `backend/converter/response_converter.go:226-232` (streaming goroutine 开头)

- [ ] **Step 1: 在 streaming goroutine 中添加 panic recovery**

文件: `backend/converter/response_converter.go` (StreamConvertOpenAIToClaude 函数内，goroutine 开头)

找到 goroutine 启动位置（约 226 行），在 goroutine 函数体开头添加 defer recover：

在 `go func() {` 之后添加：

```go
defer func() {
    if r := recover(); r != nil {
        log.Printf("[converter] panic recovered in streaming: %v\n%s", r, debug.Stack())
        errChan <- fmt.Errorf("streaming panic: %v", r)
    }
}()
```

确保在 import 中添加 `"runtime/debug"`。

- [ ] **Step 2: 验证编译通过**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "cannot"

- [ ] **Step 3: 重新构建二进制文件并重启服务**

Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -tags dev -o claude-code-cli-with-openai-api . && launchctl unload ~/Library/LaunchAgents/com.vibecoding.claude-proxy.plist && launchctl load ~/Library/LaunchAgents/com.vibecoding.claude-proxy.plist`
Expected:
  - Exit code: 0
  - `ps aux | grep claude-code-cli-with-openai-api` shows running process

- [ ] **Step 4: 提交**
Run: `git add backend/converter/response_converter.go && git commit -m "feat(converter): add panic recovery in streaming goroutine"`

---

## 关于 Claude Code CLI 客户端错误感知

当前架构下，代理端**无法直接感知** Claude Code CLI 的解析/格式错误。原因：

1. **SSE 是单向通道** — 代理发送事件，客户端接收，客户端不会反馈解析错误
2. **HTTP 连接级别** — 代理只能检测到 TCP 断开（context cancellation），无法区分"正常完成"和"客户端解析错误后断开"
3. **Claude Code CLI 的错误** 发生在 CLI 进程内部（如 `tengu_streaming_error`），代理端看不到

### 可行的感知方案（不在本次实施范围内）

| 方案 | 原理 | 复杂度 |
|------|------|--------|
| **客户端心跳扩展** | Claude Code CLI 不支持自定义 SSE 扩展 | 不可行 |
| **日志关联** | 在代理日志中记录 request_id + 完整 SSE 输出，与 CLI 错误日志通过 session_id 关联 | 低复杂度，需要手动 |
| **前端 Dashboard** | 代理 Web UI 展示 proxy_errors 表 + request_logs 表，通过时间+模型+session 筛选 | 中复杂度 |
| **SSE 结束标记** | 在 message_stop 事件后添加自定义 `event: proxy_meta` 包含转换统计 | 低复杂度，CLI 会忽略未知事件 |

最实用的方案是**增强日志记录**（Task 2 已实现）+ **前端 Dashboard**（已有基础框架）。
