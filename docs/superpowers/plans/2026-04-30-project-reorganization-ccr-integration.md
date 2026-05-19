# Project Reorganization & CCR Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 整理项目结构为 frontend/backend 分离，导入 CCR 配置，调通 OpenAI 上游代理，增强日志模块使错误可定位。

**Architecture:** 项目根目录下创建 `backend/` 和 `frontend/` 两个顶层目录。所有 Go 代码移入 `backend/`，React 代码保留在 `frontend/`。CCR 配置从 `~/.claude-code-router/config.json` 读取并导入到数据库。日志模块增加 request_id + 错误原因链 + 上下文字段，确保从日志可直接定位问题根因。

**Tech Stack:** Go 1.22, React 18, TypeScript 5, SQLite, CCR (Claude Code Router)

**Risks:**
- Task 2 目录重组改变所有 Go import paths → 缓解：使用 `gofmt` + `go build` 验证编译
- Task 3 CCR 配置含真实 API key → 缓解：导入时加密存储，日志中脱敏
- Task 4 上游代理可能因 API key 过期失败 → 缓解：启动时验证 key 有效性
- Task 5 日志增强可能影响性能 → 缓解：使用异步日志写入

---

### Task 1: 清理项目根目录 — 移除构建产物和临时文件

**Depends on:** None
**Files:**
- Modify: `.gitignore`
- Delete: `claude-bridge`, `claude-bridge-server`, `claude-code-cli`, `claude-with-openai-api`, `claude_proxy.db`, `coverage.out`, `server.log`, `test-logs.html`, `cc-gateway-darwin-arm64-f96fa0e-20260324-220726/`, `cc-gateway-darwin-arm64-f96fa0e-20260324-220726(1).tar.gz`
- Delete: `CONDUIT_FEATURE_COMPARISON.md`, 根目录下散落的测试文件

- [ ] **Step 1: 更新 .gitignore — 添加构建产物和临时文件排除规则**

```text
# 在现有 .gitignore 末尾追加以下内容

# Build artifacts
claude-bridge
claude-bridge-server
claude-code-cli
claude-with-openai-api
*.db
*.out
*.log
*.html
coverage.*

# CCR gateway archives
cc-gateway-*/

# Test artifacts
test-logs/
*.test

# OS files
.DS_Store
```

- [ ] **Step 2: 删除根目录下的构建产物和二进制文件**
Run: `rm -f claude-bridge claude-bridge-server claude-code-cli claude-with-openai-api claude_proxy.db coverage.out server.log test-logs.html && rm -rf cc-gateway-darwin-arm64-f96fa0e-20260324-220726/ "cc-gateway-darwin-arm64-f96fa0e-20260324-220726(1).tar.gz"`
Expected:
  - Exit code: 0
  - `ls claude-bridge claude-bridge-server claude-code-cli claude-with-openai-api claude_proxy.db coverage.out server.log test-logs.html` returns "No such file or directory"

- [ ] **Step 3: 删除根目录下的临时测试文件**
Run: `rm -f converter/claude_cli_integration_test.go converter/comprehensive_tool_test.go converter/end_to_end_test.go converter/even_more_tests.go converter/more_tool_tests.go converter/real_traffic_db_corpus_test.go converter/real_traffic_db_replay_test.go converter/real_traffic_edge_cases_test.go converter/real_traffic_test.go client/openai_client_test.go`
Expected:
  - Exit code: 0

- [ ] **Step 4: 删除根目录下的散落文档**
Run: `rm -f CONDUIT_FEATURE_COMPARISON.md`
Expected:
  - Exit code: 0

- [ ] **Step 5: 验证清理结果**
Run: `ls -la *.db *.out *.log *.html claude-bridge claude-bridge-server claude-code-cli claude-with-openai-api 2>&1`
Expected:
  - Output contains: "No such file or directory"

- [ ] **Step 6: 提交**
Run: `git add .gitignore && git add -u claude-bridge claude-bridge-server claude-code-cli claude-with-openai-api claude_proxy.db coverage.out server.log test-logs.html CONDUIT_FEATURE_COMPARISON.md converter/claude_cli_integration_test.go converter/comprehensive_tool_test.go converter/end_to_end_test.go converter/even_more_tests.go converter/more_tool_tests.go converter/real_traffic_db_corpus_test.go converter/real_traffic_db_replay_test.go converter/real_traffic_edge_cases_test.go converter/real_traffic_test.go client/openai_client_test.go && git commit -m "chore: clean up build artifacts and temporary test files from root directory"`

---

### Task 2: 重组项目目录 — 创建 backend/ 和 frontend/ 分离结构

**Depends on:** Task 1
**Files:**
- Create: `backend/` 目录，将所有 Go 代码移入
- Modify: `go.mod` (module path)
- Modify: 所有 Go 文件的 import paths
- Move: `claude/`, `client/`, `cmd/`, `config/`, `converter/`, `database/`, `handler/`, `models/`, `security/`, `service/`, `tools/`, `utils/` → `backend/`

- [ ] **Step 1: 创建 backend 目录并移动所有 Go 代码包**
Run: `mkdir -p backend && mv claude client cmd config converter database handler models security service tools utils backend/`
Expected:
  - Exit code: 0
  - `ls backend/claude backend/client backend/cmd` shows files

- [ ] **Step 2: 移动 Go 项目文件到 backend 目录**
Run: `mv go.mod go.sum main.go Makefile env.example backend/`
Expected:
  - Exit code: 0
  - `ls backend/go.mod backend/main.go` shows files

- [ ] **Step 3: 移动数据库迁移文件到 backend 目录**
Run: `mv database backend/ 2>/dev/null; if [ -d database ]; then cp -r database backend/ && rm -rf database; fi; ls backend/database/`
Expected:
  - Exit code: 0

- [ ] **Step 4: 更新 go.mod module path**
文件: `backend/go.mod:1`

```text
module github.com/vibe-coding-labs/claude-code-cli-with-openai-api/backend
```

- [ ] **Step 5: 全局替换 Go import paths**
Run: `cd backend && find . -name "*.go" -exec sed -i '' 's|"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/|"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/backend/|g' {} +`
Expected:
  - Exit code: 0

- [ ] **Step 6: 验证 Go 编译**
Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "cannot find package" or "imported and not used"

- [ ] **Step 7: 更新 Makefile 中的路径引用**
文件: `backend/Makefile`

将 Makefile 中所有对项目根目录的引用更新为适配 backend/ 目录结构。主要修改：
- `BINARY` 输出路径保持 `./claude-code-cli`
- `go build` 命令的 package 路径更新为 `./...`
- `go test` 命令的 package 路径更新为 `./...`
- `clean` 目标删除正确的文件

- [ ] **Step 8: 验证 Makefile 构建**
Run: `cd backend && make build`
Expected:
  - Exit code: 0
  - `ls backend/claude-code-cli` shows binary exists

- [ ] **Step 9: 提交**
Run: `cd backend && git add -A && git commit -m "refactor: reorganize project into backend/ and frontend/ directories"`

---

### Task 3: 导入 CCR 配置 — 读取 Claude Code Router 配置并导入系统

**Depends on:** Task 2
**Files:**
- Create: `backend/config/cco_importer.go`
- Modify: `backend/config/manager_crud.go` (添加批量导入方法)
- Modify: `backend/cmd/server.go` (启动时自动导入)

- [ ] **Step 1: 创建 CCR 配置导入器 — 读取 ~/.claude-code-router/config.json 并转换为系统配置**

```go
// backend/config/cco_importer.go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type CCRConfig struct {
	Port       int               `json:"port"`
	Transforms []CCRTransform    `json:"transforms"`
	LogLevel   string            `json:"logLevel"`
}

type CCRTransform struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Endpoint   string            `json:"endpoint"`
	APIKey     string            `json:"apiKey"`
	Model      string            `json:"model"`
	Headers    map[string]string `json:"headers"`
	MaxTokens  int               `json:"maxTokens"`
	Temperature float64          `json:"temperature"`
}

func ImportCCRConfig(homeDir string) ([]APIConfigCreate, error) {
	configPath := filepath.Join(homeDir, ".claude-code-router", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read CCR config: %w", err)
	}

	var ccr CCRConfig
	if err := json.Unmarshal(data, &ccr); err != nil {
		return nil, fmt.Errorf("parse CCR config: %w", err)
	}

	var configs []APIConfigCreate
	for _, t := range ccr.Transforms {
		configs = append(configs, APIConfigCreate{
			Name:        t.Name,
			Provider:    mapCCRTypeToProvider(t.Type),
			Endpoint:    t.Endpoint,
			APIKey:      t.APIKey,
			Model:       t.Model,
			MaxTokens:   t.MaxTokens,
			Temperature: float32(t.Temperature),
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
	}
	return configs, nil
}

func mapCCRTypeToProvider(ccrType string) string {
	switch ccrType {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "azure":
		return "azure"
	default:
		return "openai"
	}
}
```

- [ ] **Step 2: 在 Manager 中添加批量导入方法**
文件: `backend/config/manager_crud.go`

在 Manager 结构体中添加 `ImportFromCCR` 方法：

```go
// 在 backend/config/manager_crud.go 中添加以下方法

func (m *Manager) ImportFromCCR(homeDir string) (int, error) {
	configs, err := ImportCCRConfig(homeDir)
	if err != nil {
		return 0, fmt.Errorf("import CCR config: %w", err)
	}

	imported := 0
	for _, cfg := range configs {
		existing, _ := m.GetByName(cfg.Name)
		if existing != nil {
			continue
		}
		if err := m.Create(cfg); err != nil {
			continue
		}
		imported++
	}
	return imported, nil
}
```

- [ ] **Step 3: 在 server.go 启动时自动导入 CCR 配置**
文件: `backend/cmd/server.go`

在 server 启动流程中，数据库初始化之后、路由注册之前，添加 CCR 配置导入逻辑：

```go
// 在 backend/cmd/server.go 的启动函数中，数据库初始化之后添加

homeDir, _ := os.UserHomeDir()
if imported, err := configMgr.ImportFromCCR(homeDir); err != nil {
	log.Printf("[WARN] CCR config import skipped: %v", err)
} else if imported > 0 {
	log.Printf("[INFO] Imported %d configs from CCR", imported)
}
```

- [ ] **Step 4: 验证 CCR 配置导入**
Run: `cd backend && go build ./cmd/server.go`
Expected:
  - Exit code: 0

- [ ] **Step 5: 提交**
Run: `git add backend/config/cco_importer.go backend/config/manager_crud.go backend/cmd/server.go && git commit -m "feat(config): add CCR config importer to auto-import Claude Code Router settings"`

---

### Task 4: 调通 OpenAI 上游代理 — 确保两个 OpenAI 上游可正常代理请求

**Depends on:** Task 3
**Files:**
- Modify: `backend/client/openai_client.go` (增强错误处理和重试)
- Modify: `backend/handler/retry_handler.go` (优化重试策略)
- Modify: `backend/handler/lb_manager.go` (负载均衡健康检查)

- [ ] **Step 1: 增强 OpenAI 客户端错误处理 — 添加详细的错误原因链**
文件: `backend/client/openai_client.go`

修改 `OpenAIClient` 的请求方法，在错误返回中包含完整的上下文信息：

```go
// 在 backend/client/openai_client.go 中修改错误处理

type UpstreamError struct {
	Provider    string
	Endpoint    string
	Model       string
	StatusCode  int
	RequestBody string
	ResponseBody string
	Err         error
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream error: provider=%s endpoint=%s model=%s status=%d: %s | response=%s",
		e.Provider, e.Endpoint, e.Model, e.StatusCode, e.Err, e.ResponseBody)
}

func (e *UpstreamError) Unwrap() error {
	return e.Err
}
```

- [ ] **Step 2: 修改 OpenAI 客户端请求方法 — 使用 UpstreamError 包装所有错误**
文件: `backend/client/openai_client.go`

在所有 HTTP 请求的错误返回点，使用 `UpstreamError` 包装，确保包含完整的请求上下文：

```go
// 在 HTTP 请求失败时返回 UpstreamError
if resp.StatusCode >= 400 {
	body, _ := io.ReadAll(resp.Body)
	return nil, &UpstreamError{
		Provider:     c.Provider,
		Endpoint:     c.Endpoint,
		Model:        req.Model,
		StatusCode:   resp.StatusCode,
		ResponseBody: string(body),
		Err:          fmt.Errorf("HTTP %d", resp.StatusCode),
	}
}
```

- [ ] **Step 3: 优化重试策略 — 区分可重试和不可重试错误**
文件: `backend/handler/retry_handler.go`

修改 `isRetryable` 函数，根据 `UpstreamError` 的状态码判断是否可重试：

```go
func isRetryable(err error) bool {
	var ue *client.UpstreamError
	if errors.As(err, &ue) {
		switch ue.StatusCode {
		case 429, 500, 502, 503, 504:
			return true
		case 401, 403, 404:
			return false
		}
	}
	return false
}
```

- [ ] **Step 4: 添加上游健康检查 — 启动时验证 API key 有效性**
文件: `backend/handler/lb_manager.go`

在负载均衡管理器中添加健康检查方法：

```go
func (m *LBManager) HealthCheck(ctx context.Context) map[string]bool {
	results := make(map[string]bool)
	for name, c := range m.clients {
		results[name] = c.Ping(ctx)
	}
	return results
}
```

- [ ] **Step 5: 验证编译**
Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 6: 提交**
Run: `git add backend/client/openai_client.go backend/handler/retry_handler.go backend/handler/lb_manager.go && git commit -m "feat(proxy): enhance OpenAI upstream error handling with detailed error chains and health checks"`

---

### Task 5: 增强日志模块 — 添加 request_id + 错误原因链 + 上下文字段

**Depends on:** Task 2
**Files:**
- Modify: `backend/utils/logger.go` (核心日志增强)
- Modify: `backend/handler/handler.go` (请求日志中间件)
- Modify: `backend/handler/response_handler.go` (响应日志)

- [ ] **Step 1: 重构 Logger — 添加结构化日志字段支持**
文件: `backend/utils/logger.go`

```go
// backend/utils/logger.go
package utils

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

type LogEntry struct {
	Timestamp  string                 `json:"timestamp"`
	Level      string                 `json:"level"`
	RequestID  string                 `json:"request_id,omitempty"`
	Message    string                 `json:"message"`
	Caller     string                 `json:"caller,omitempty"`
	Error      string                 `json:"error,omitempty"`
	ErrorCause string                 `json:"error_cause,omitempty"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

type Logger struct {
	mu        sync.Mutex
	file      *os.File
	level     LogLevel
	enableJSON bool
}

var defaultLogger *Logger

func InitLogger(logPath string, level string, jsonFormat bool) error {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}

	l := &LogLevel(INFO)
	switch level {
	case "DEBUG":
		l = newLogLevel(DEBUG)
	case "WARN":
		l = newLogLevel(WARN)
	case "ERROR":
		l = newLogLevel(ERROR)
	}

	defaultLogger = &Logger{
		file:      f,
		level:     *l,
		enableJSON: jsonFormat,
	}
	return nil
}

func newLogLevel(l LogLevel) *LogLevel {
	return &l
}

func (l *Logger) log(level LogLevel, requestID string, msg string, err error, ctx map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02T15:04:05.000Z07:00"),
		Level:     levelString(level),
		RequestID: requestID,
		Message:   msg,
		Context:   ctx,
	}

	if err != nil {
		entry.Error = err.Error()
		if unwrapped := errors.Unwrap(err); unwrapped != nil {
			entry.ErrorCause = unwrapped.Error()
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.enableJSON {
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.file, string(data))
	} else {
		l.formatText(entry)
	}
}

func (l *Logger) formatText(e LogEntry) {
	parts := []string{e.Timestamp, e.Level}
	if e.RequestID != "" {
		parts = append(parts, "["+e.RequestID+"]")
	}
	parts = append(parts, e.Message)
	if e.Error != "" {
		parts = append(parts, "error="+e.Error)
	}
	if e.ErrorCause != "" {
		parts = append(parts, "cause="+e.ErrorCause)
	}
	for k, v := range e.Context {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	fmt.Fprintln(l.file, strings.Join(parts, " "))
}

func levelString(l LogLevel) string {
	switch l {
	case DEBUG: return "DEBUG"
	case INFO:  return "INFO"
	case WARN:  return "WARN"
	case ERROR: return "ERROR"
	default:    return "UNKNOWN"
	}
}

// Public API
func Info(msg string)                              { defaultLogger.log(INFO, "", msg, nil, nil) }
func InfoCtx(ctx context.Context, msg string)       { defaultLogger.log(INFO, requestIDFromCtx(ctx), msg, nil, nil) }
func Warn(msg string)                              { defaultLogger.log(WARN, "", msg, nil, nil) }
func WarnCtx(ctx context.Context, msg string)       { defaultLogger.log(WARN, requestIDFromCtx(ctx), msg, nil, nil) }
func Error(msg string, err error)                  { defaultLogger.log(ERROR, "", msg, err, nil) }
func ErrorCtx(ctx context.Context, msg string, err error) { defaultLogger.log(ERROR, requestIDFromCtx(ctx), msg, err, nil) }
func ErrorWithCtx(ctx context.Context, msg string, err error, fields map[string]interface{}) {
	defaultLogger.log(ERROR, requestIDFromCtx(ctx), msg, err, fields)
}
func Debug(msg string)                             { defaultLogger.log(DEBUG, "", msg, nil, nil) }

func requestIDFromCtx(ctx context.Context) string {
	if v := ctx.Value("request_id"); v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
```

- [ ] **Step 2: 添加请求日志中间件 — 为每个请求生成 request_id 并注入 context**
文件: `backend/handler/handler.go`

在 HTTP handler 链中添加请求 ID 中间件：

```go
// 在 backend/handler/handler.go 中添加请求 ID 中间件

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
		}
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

- [ ] **Step 3: 修改响应处理器 — 记录完整的请求/响应日志**
文件: `backend/handler/response_handler.go`

在响应处理的关键路径添加结构化日志，包含 request_id、上游信息、错误原因：

```go
// 在响应处理的关键路径添加日志
utils.InfoCtx(ctx, fmt.Sprintf("request completed: method=%s path=%s status=%d upstream=%s model=%s latency=%dms",
	r.Method, r.URL.Path, statusCode, upstreamName, model, latencyMs))
```

- [ ] **Step 4: 验证编译**
Run: `cd backend && go build ./...`
Expected:
  - Exit code: 0

- [ ] **Step 5: 提交**
Run: `git add backend/utils/logger.go backend/handler/handler.go backend/handler/response_handler.go && git commit -m "feat(log): enhance logger with request_id, error cause chain, and structured context fields"`

---

### Task 6: 启动验证 — 启动前端和后端，验证整体功能

**Depends on:** Task 2, Task 3, Task 4, Task 5
**Files:**
- Modify: `backend/cmd/server.go` (确保启动流程完整)
- Modify: `frontend/src/services/api.ts` (确保 API 地址正确)

- [ ] **Step 1: 启动后端服务**
Run: `cd backend && PORT=54988 go run ./cmd/server.go &`
Expected:
  - Output contains: "server started" or "listening"
  - No panic or fatal error

- [ ] **Step 2: 验证后端健康检查**
Run: `curl -s http://localhost:54988/health | head -5`
Expected:
  - Exit code: 0
  - Output contains: "ok" or "healthy"

- [ ] **Step 3: 启动前端开发服务器**
Run: `cd frontend && npm start &`
Expected:
  - Output contains: "Compiled successfully" or "webpack compiled"
  - No compilation errors

- [ ] **Step 4: 验证前端可访问**
Run: `curl -s http://localhost:54990 | head -10`
Expected:
  - Exit code: 0
  - Output contains: "<!DOCTYPE html>" or "<html"

- [ ] **Step 5: 验证 CCR 配置已导入**
Run: `curl -s http://localhost:54988/api/configs | python3 -m json.tool | head -20`
Expected:
  - Exit code: 0
  - Output contains: JSON array with config entries

- [ ] **Step 6: 验证 OpenAI 上游代理连通性**
Run: `curl -s -X POST http://localhost:54988/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}' | head -5`
Expected:
  - Exit code: 0
  - 返回正常的 OpenAI 响应或可读的错误信息（不是 500 内部错误）

- [ ] **Step 7: 提交**
Run: `git add -A && git commit -m "chore: verify full stack startup and CCR integration"`
