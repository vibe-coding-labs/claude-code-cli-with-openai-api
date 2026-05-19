# Stream Stall Auto-Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 当上游服务商卡住（长时间不发送数据）时，自动检测并服务端透明重试，避免客户端长时间等待。流式通信通过空闲超时检测卡住状态，在首次数据到达前进行服务端重试（对客户端完全透明），首次数据到达后仍保持现有的 overloaded_error 机制触发客户端重试。

**Architecture:** 客户端请求 → handler 创建流 → **StallDetector 验证首次数据（新增）** → 如果超时无数据 → 关闭流 → 重试（最多 N 次）→ 如果有数据 → 传递给 converter 正常处理。converter 内部的 mid-stream idle timer 保持不变（参数化为可配置）。新增 `StreamStallTimeout` 配置项控制预验证超时，默认 60 秒。

**Tech Stack:** Go 1.22, Gin, SQLite, io.Reader 组合模式

**Risks:**
- Task 3 修改 handler.go 的核心流式处理路径，需确保重试循环不影响正常请求 → 缓解：重试仅在预验证阶段触发，converter 调用不变
- StallDetector 的 goroutine 可能在 reader.Close() 后仍在阻塞读 → 缓解：使用 context 取消机制
- converter 函数签名变更需更新所有调用点 → 缓解：只有一个调用点（handler.go:587）

---

### Task 1: Add StreamStallTimeout Configuration

**Depends on:** None
**Files:**
- Modify: `backend/database/types.go:6-36`
- Modify: `backend/config/config.go:12-36`
- Modify: `backend/database/db.go:344-389`
- Modify: `backend/handler/config_manager.go:104-110`

- [ ] **Step 1: 添加 StreamStallTimeout 字段到 APIConfig — 控制预验证超时时间**
文件: `backend/database/types.go:6-36`

```go
// 替换 backend/database/types.go:6-36 的 APIConfig 结构体
type APIConfig struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Description           string            `json:"description"`
	UserID                int64             `json:"user_id"`
	OpenAIAPIKey          string            `json:"openai_api_key,omitempty"`
	OpenAIAPIKeyEncrypted string            `json:"-"`
	OpenAIAPIKeyMasked    string            `json:"openai_api_key_masked,omitempty"`
	OpenAIBaseURL         string            `json:"openai_base_url"`
	BigModel              string            `json:"big_model"`
	MiddleModel           string            `json:"middle_model"`
	SmallModel            string            `json:"small_model"`
	SupportedModels       []string          `json:"supported_models,omitempty"`
	ModelMappings         map[string]string `json:"model_mappings,omitempty"`
	MaxTokensLimit        int               `json:"max_tokens_limit"`
	RequestTimeout        int               `json:"request_timeout"`
	RetryCount            int               `json:"retry_count"`
	RetryBackoffBase      float64           `json:"retry_backoff_base,omitempty"`
	RetryBackoffMax       int               `json:"retry_backoff_max,omitempty"`
	ProxyURL              string            `json:"proxy_url,omitempty"`
	ReasoningEffort       string            `json:"reasoning_effort,omitempty"`
	BigModelReasoningEffort    string     `json:"big_model_reasoning_effort,omitempty"`
	MiddleModelReasoningEffort string     `json:"middle_model_reasoning_effort,omitempty"`
	SmallModelReasoningEffort  string     `json:"small_model_reasoning_effort,omitempty"`
	StreamStallTimeout    int               `json:"stream_stall_timeout,omitempty"` // 流式预验证超时（秒），默认60秒
	AnthropicAPIKey       string            `json:"anthropic_api_key,omitempty"`
	CustomHeaders         map[string]string `json:"custom_headers,omitempty"`
	Enabled               bool              `json:"enabled"`
	ExpiresAt             *time.Time        `json:"expires_at,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}
```

- [ ] **Step 2: 添加 StreamStallTimeout 到 Config 结构体**
文件: `backend/config/config.go:12-36`

```go
// 替换 backend/config/config.go:12-36 的 Config 结构体
type Config struct {
	ConfigID            string
	ConfigName          string
	OpenAIAPIKey         string
	OpenAIBaseURL        string
	BigModel             string
	MiddleModel          string
	SmallModel           string
	SupportedModels      []string
	MaxTokensLimit       int
	RequestTimeout       int
	RetryCount           int
	RetryBackoffBase     float64
	RetryBackoffMax      int
	ProxyURL            string
	ReasoningEffort      string
	AnthropicAPIKey      string
	AzureAPIVersion      string
	Host                 string
	Port                 int
	LogLevel             string
	MinTokensLimit       int
	CustomHeaders        map[string]string
	EnableRequestLogging bool
	StreamStallTimeout   int // 流式预验证超时（秒），默认60秒
}
```

- [ ] **Step 3: 添加数据库 migration — 新增 stream_stall_timeout 列**
文件: `backend/database/db.go:380`（在最后一个 migration 后追加）

在 `migrations` 切片的最后一个元素（`log_level` 那行）之后添加：

```go
// 迁移10: 为 api_configs 添加流式预验证超时字段
`ALTER TABLE api_configs ADD COLUMN stream_stall_timeout INTEGER DEFAULT 60;`,
```

- [ ] **Step 4: 在配置管理器中设置默认值**
文件: `backend/handler/config_manager.go:104-110`（在 `RequestTimeout` 默认值设置之后添加）

```go
if config.StreamStallTimeout == 0 {
    config.StreamStallTimeout = 60
}
```

- [ ] **Step 5: 在 handler.go 的配置构建中传递 StreamStallTimeout**
文件: `backend/handler/handler.go:467-482`（在 targetConfig 构建中添加字段）

```go
targetConfig = &config.Config{
    ConfigID:           dbConfig.ID,
    ConfigName:         dbConfig.Name,
    OpenAIBaseURL:      dbConfig.OpenAIBaseURL,
    OpenAIAPIKey:       dbConfig.OpenAIAPIKey,
    BigModel:           dbConfig.BigModel,
    MiddleModel:        dbConfig.MiddleModel,
    SmallModel:         dbConfig.SmallModel,
    MaxTokensLimit:     dbConfig.MaxTokensLimit,
    RequestTimeout:     h.config.RequestTimeout,
    RetryCount:         dbConfig.RetryCount,
    RetryBackoffBase:   dbConfig.RetryBackoffBase,
    RetryBackoffMax:    dbConfig.RetryBackoffMax,
    AnthropicAPIKey:    dbConfig.AnthropicAPIKey,
    ReasoningEffort:    reasoningEffort,
    StreamStallTimeout: dbConfig.StreamStallTimeout,
}
```

- [ ] **Step 6: 验证配置编译**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "undefined"

- [ ] **Step 7: 提交**
Run: `git add backend/database/types.go backend/config/config.go backend/database/db.go backend/handler/config_manager.go backend/handler/handler.go && git commit -m "feat(config): add StreamStallTimeout field for configurable pre-stream stall detection"`

---

### Task 2: Create StallDetector Utility

**Depends on:** None
**Files:**
- Create: `backend/handler/stall_detector.go`

- [ ] **Step 1: 创建 StallDetector — 封装预验证逻辑，检测上游是否在超时内发送数据**

```go
// backend/handler/stall_detector.go
package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"backend/utils"
)

// ErrUpstreamStalled indicates the upstream provider did not send any data
// within the configured stall timeout during pre-stream verification.
var ErrUpstreamStalled = fmt.Errorf("upstream stalled: no data received within stall timeout")

// StallReadResult contains the result of a pre-stream read attempt.
type StallReadResult struct {
	// FirstData contains the first chunk of data read from upstream.
	FirstData []byte
	// Reader is an io.Reader that replays FirstData followed by the remaining stream.
	Reader io.Reader
	// Err is set if the read failed (including stall timeout).
	Err error
}

// WaitForFirstData reads the first chunk from upstream with a timeout.
// If no data arrives within stallTimeout, returns ErrUpstreamStalled.
// On success, returns a combined reader that replays the peeked data
// followed by the remaining stream, so no data is lost.
func WaitForFirstData(ctx context.Context, reader io.ReadCloser, stallTimeout time.Duration) StallReadResult {
	logger := utils.GetLogger()
	logger.Info("  [stall-detector] Waiting for first data from upstream (timeout: %v)", stallTimeout)

	type readResult struct {
		data []byte
		err  error
	}

	ch := make(chan readResult, 1)
	buf := make([]byte, 64*1024)

	go func() {
		n, err := reader.Read(buf)
		ch <- readResult{buf[:n], err}
	}()

	select {
	case result := <-ch:
		if result.err != nil {
			logger.Warn("  [stall-detector] Read error before first data: %v", result.err)
			return StallReadResult{Err: fmt.Errorf("read error during pre-stream check: %w", result.err)}
		}
		logger.Info("  [stall-detector] First data received (%d bytes), upstream is responsive", len(result.data))
		combinedReader := io.MultiReader(bytes.NewReader(result.data), reader)
		return StallReadResult{
			FirstData: result.data,
			Reader:    combinedReader,
		}
	case <-time.After(stallTimeout):
		logger.Warn("  [stall-detector] Upstream stalled! No data for %v, will retry", stallTimeout)
		return StallReadResult{Err: ErrUpstreamStalled}
	case <-ctx.Done():
		logger.Info("  [stall-detector] Client disconnected during pre-stream check")
		return StallReadResult{Err: ctx.Err()}
	}
}
```

- [ ] **Step 2: 验证 StallDetector 编译**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "undefined"

- [ ] **Step 3: 提交**
Run: `git add backend/handler/stall_detector.go && git commit -m "feat(handler): add StallDetector utility for pre-stream upstream responsiveness verification"`

---

### Task 3: Implement Server-Side Retry Loop for Streaming

**Depends on:** Task 1, Task 2
**Files:**
- Modify: `backend/handler/handler.go:545-611`

- [ ] **Step 1: 替换流式处理路径 — 添加预验证重试循环**
文件: `backend/handler/handler.go:545-611`（替换整个 `if req.Stream` 分支的流式处理部分，从 `reader, err := targetClient.CreateChatCompletionStream` 开始到 `streamResult = converter.ConvertOpenAIStreamingToClaudeWithMapping` 结束）

```go
		if req.Stream {
			// 流式响应 — 先等待限流器放行，再发起请求（内部已有重试+智能429等待）
			targetClient.BetaHeaders = betaHeaders

			// Rate limiter: wait for any existing cooldown before making the request.
			if remaining := ratelimit.Global.GetCooldown(configID); remaining > 0 {
				if remaining > 90*time.Second {
					logger.Warn("  Rate limiter: cooldown %v too long for config %s, returning overloaded_error", remaining.Round(time.Second), configID)
					c.JSON(http.StatusServiceUnavailable, gin.H{
						"type": "error",
						"error": map[string]interface{}{
							"type":    "overloaded_error",
							"message": fmt.Sprintf("API is temporarily overloaded. Please retry after %v.", remaining.Round(time.Second)),
						},
					})
					h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", "rate_limit_cooldown", &req, nil)
					return fmt.Errorf("rate limit cooldown too long (%v), returned overloaded_error", remaining.Round(time.Second))
				}
				logger.Info("  Rate limiter: waiting %v for config %s cooldown", remaining.Round(time.Second), configID)
				if waitErr := ratelimit.Global.Wait(c.Request.Context(), configID); waitErr != nil {
					return fmt.Errorf("cancelled while waiting for rate limit: %w", waitErr)
				}
			}

			// Pre-stream verification with server-side auto-retry.
			// If upstream doesn't send any data within stallTimeout, close the stream
			// and retry transparently (no data has been sent to the client yet).
			stallTimeout := time.Duration(targetConfig.StreamStallTimeout) * time.Second
			if stallTimeout <= 0 {
				stallTimeout = 60 * time.Second
			}
			maxStallRetries := 3

			var streamResult *converter.StreamingResult
			for stallRetry := 0; stallRetry <= maxStallRetries; stallRetry++ {
				reader, err := targetClient.CreateChatCompletionStream(openAIReq)
				if err != nil {
					logger.Error("← [executeMessageRequestWithConfig] Stream creation failed: %v", err)
					h.responseHandler.SendErrorResponse(c, err)
					h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", err.Error(), &req, nil)
					return fmt.Errorf("stream creation failed: %w", err)
				}

				// Pre-stream verification: check if upstream sends data within stallTimeout.
				// This happens BEFORE any SSE events are sent to the client, so retries are transparent.
				result := WaitForFirstData(c.Request.Context(), reader, stallTimeout)
				if result.Err != nil {
					reader.Close()
					if result.Err == ErrUpstreamStalled {
						if stallRetry < maxStallRetries {
							logger.Warn("  [stall-retry] Attempt %d/%d: upstream stalled, retrying...", stallRetry+1, maxStallRetries)
							continue
						}
						logger.Error("  [stall-retry] All %d retries exhausted, returning overloaded_error to client", maxStallRetries)
						c.JSON(http.StatusServiceUnavailable, gin.H{
							"type": "error",
							"error": map[string]interface{}{
								"type":    "overloaded_error",
								"message": fmt.Sprintf("Upstream provider unresponsive after %d retries. Please try again later.", maxStallRetries),
							},
						})
						h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", "upstream_stalled_after_retries", &req, nil)
						return fmt.Errorf("upstream stalled after %d retries", maxStallRetries)
					}
					// Non-stall error (client disconnect, read error)
					if c.Request.Context().Err() != nil {
						return fmt.Errorf("client disconnected during stall check: %w", result.Err)
					}
					logger.Error("  [stall-retry] Read error during pre-stream check: %v", result.Err)
					h.responseHandler.SendErrorResponse(c, result.Err)
					return result.Err
				}

				// Upstream is responsive — proceed with normal streaming conversion.
				// result.Reader replays the first chunk + remaining data.
				defer reader.Close()
				logger.Info("  Stream verified, processing response (stall retries: %d)...", stallRetry)

				streamResult = converter.ConvertOpenAIStreamingToClaudeWithMapping(c, result.Reader, &req, c.Request.Context(), toolNameMapping)
				break
			}

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

- [ ] **Step 2: 验证 handler 编译**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "undefined"

- [ ] **Step 3: 提交**
Run: `git add backend/handler/handler.go && git commit -m "feat(handler): add server-side auto-retry with pre-stream stall detection for streaming requests"`

---

### Task 4: Make Converter Mid-Stream Stall Timeout Configurable

**Depends on:** Task 1
**Files:**
- Modify: `backend/converter/response_converter.go:186-414`

- [ ] **Step 1: 修改 ConvertOpenAIStreamingToClaudeWithMapping 函数签名 — 添加 stallTimeout 参数**
文件: `backend/converter/response_converter.go:186`（替换函数签名和 stall timeout 常量）

```go
// ConvertOpenAIStreamingToClaudeWithMapping converts OpenAI streaming response to Claude format with tool name mapping.
// stallTimeout controls the mid-stream idle timeout — if upstream sends no data for this duration,
// an overloaded_error is sent to trigger client-side retry. Pass 0 to use the default (120 seconds).
func ConvertOpenAIStreamingToClaudeWithMapping(c *gin.Context, reader io.Reader, originalReq *models.ClaudeMessagesRequest, ctx context.Context, toolNameMapping map[string]string, stallTimeout time.Duration) *StreamingResult {
	state := newStreamingState(originalReq.Model, toolNameMapping)
	var collectedContent strings.Builder

	// Default mid-stream idle timeout
	if stallTimeout <= 0 {
		stallTimeout = 120 * time.Second
	}
```

- [ ] **Step 2: 替换硬编码的 streamIdleTimeout 常量 — 使用函数参数**
文件: `backend/converter/response_converter.go:210-213`（删除常量定义，改为使用参数）

```go
	// Idle timeout for stall detection: if upstream sends nothing for stallTimeout,
	// treat as stalled and return overloaded_error so Claude Code auto-retries.
	idleTimer := time.NewTimer(stallTimeout)
	defer idleTimer.Stop()
```

- [ ] **Step 3: 更新 idleTimer 的 Reset 调用 — 使用相同的 stallTimeout 参数**
文件: `backend/converter/response_converter.go:224`（将 `streamIdleTimeout` 替换为 `stallTimeout`）

```go
				// Reset idle timer — upstream sent data (stall detection)
				idleTimer.Reset(stallTimeout)
```

- [ ] **Step 4: 更新 overloaded_error 消息中的超时时间显示**
文件: `backend/converter/response_converter.go:409`（更新错误消息）

```go
		case <-idleTimer.C:
			sendSSEError(c, "overloaded_error", fmt.Sprintf("Upstream provider stalled (no data for %v). Please retry.", stallTimeout))
			return nil
```

- [ ] **Step 5: 更新 handler.go 中的调用点 — 传递 stallTimeout 参数**
文件: `backend/handler/handler.go`（在 Task 3 中添加的代码里，找到 `converter.ConvertOpenAIStreamingToClaudeWithMapping` 调用）

将调用修改为传递 stallTimeout：
```go
				streamResult = converter.ConvertOpenAIStreamingToClaudeWithMapping(c, result.Reader, &req, c.Request.Context(), toolNameMapping, stallTimeout)
```

- [ ] **Step 6: 验证全部编译通过**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "undefined"

- [ ] **Step 7: 提交**
Run: `git add backend/converter/response_converter.go backend/handler/handler.go && git commit -m "feat(converter): parameterize mid-stream stall timeout for configurable idle detection"`

---

### Task 5: Full Build Verification and Testing

**Depends on:** Task 3, Task 4
**Files:**
- No new files

- [ ] **Step 1: 完整编译 — 确保所有包无错误**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -o /dev/null ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "Error" or "cannot"

- [ ] **Step 2: 运行现有测试 — 确保没有回归**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./... 2>&1 | head -50`
Expected:
  - Output contains: "ok" or "PASS"
  - Output does NOT contain: "FAIL" (excluding known pre-existing failures)

- [ ] **Step 3: 编译最终二进制**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build -o claude-code-cli-with-openai-api .`
Expected:
  - Exit code: 0
  - Binary file exists: `claude-code-cli-with-openai-api`

- [ ] **Step 4: 提交最终验证状态**
Run: `git status`
