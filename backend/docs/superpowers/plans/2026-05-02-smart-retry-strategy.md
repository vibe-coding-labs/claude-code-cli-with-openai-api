# Smart Retry Strategy Module

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 将分散在三处的重试逻辑抽取为独立的 `retry` package，按错误类型应用不同策略：429 限流高重试次数+长退避，5xx 中等重试，400/401 不重试。支持 Retry-After header 和 jitter。

**Architecture:** 请求失败 → `retry.ClassifyError(err)` 得到 ErrorCategory → 按类别选择 RetryStrategy → `retry.Execute()` 执行带退避的重试循环。handler/handler.go 和 client/openai_client.go 均委托给 retry package，消除重复逻辑。

**Tech Stack:** Go 1.22, existing gin/http context, no new external dependencies

**Risks:**
- Task 2 修改 handler 核心路径 → 缓解：保持 executeMessageRequestWithConfig 函数签名不变，只替换内部重试循环
- Task 3 流式重试只在 stream creation 失败时生效（SSE response 未开始写入） → 缓解：检查 response header 是否已发送
- 三处 IsRetryableError 有细微差异（如 isPermanentQuotaError）→ 缓解：统一到 retry package，保留 permanent quota 检测逻辑

---

### Task 1: Create Smart Retry Package

**Depends on:** None
**Files:**
- Create: `retry/retry.go`
- Create: `retry/retry_test.go`

- [ ] **Step 1: 创建 retry package 核心模块 — 错误分类、策略定义、重试引擎**

```go
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

// ErrorCategory classifies the type of error for retry strategy selection.
type ErrorCategory int

const (
	// CategoryRateLimit — 429 errors, highly retryable with long backoff
	CategoryRateLimit ErrorCategory = iota
	// CategoryServerError — 5xx errors, retryable with medium backoff
	CategoryServerError
	// CategoryNetwork — connection/timeout errors, retryable with short backoff
	CategoryNetwork
	// CategoryProtocol — 400 errors (our proxy bug or bad request), never retry
	CategoryProtocol
	// CategoryAuth — 401/403 errors, never retry
	CategoryAuth
	// CategoryPermanentQuota — 429 with permanent quota signals, never retry
	CategoryPermanentQuota
	// CategoryCancelled — client disconnected, never retry
	CategoryCancelled
	// CategoryUnknown — unclassified, don't retry by default
	CategoryUnknown
)

// Strategy defines retry parameters for a specific error category.
type Strategy struct {
	MaxRetries   int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	Retryable    bool
}

// Predefined strategies per error category.
var defaultStrategies = map[ErrorCategory]Strategy{
	CategoryRateLimit: {
		MaxRetries: 20, BaseDelay: 5 * time.Second, MaxDelay: 120 * time.Second, Retryable: true,
	},
	CategoryServerError: {
		MaxRetries: 5, BaseDelay: 2 * time.Second, MaxDelay: 30 * time.Second, Retryable: true,
	},
	CategoryNetwork: {
		MaxRetries: 10, BaseDelay: 1 * time.Second, MaxDelay: 15 * time.Second, Retryable: true,
	},
	CategoryProtocol:       {MaxRetries: 0, Retryable: false},
	CategoryAuth:           {MaxRetries: 0, Retryable: false},
	CategoryPermanentQuota: {MaxRetries: 0, Retryable: false},
	CategoryCancelled:      {MaxRetries: 0, Retryable: false},
	CategoryUnknown:        {MaxRetries: 0, Retryable: false},
}

// ClassifyError determines the error category from an error.
func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return CategoryUnknown
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return CategoryCancelled
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return CategoryNetwork
	}

	errStr := strings.ToLower(err.Error())

	// Check for permanent quota signals BEFORE general 429 check
	if isPermanentQuotaError(errStr) {
		return CategoryPermanentQuota
	}

	// HTTP status code classification
	if strings.Contains(errStr, "status 429") || strings.Contains(errStr, "rate limit") {
		return CategoryRateLimit
	}

	if isServerErrorStatus(errStr) {
		return CategoryServerError
	}

	if isNetworkError(errStr) {
		return CategoryNetwork
	}

	if isAuthError(errStr) {
		return CategoryAuth
	}

	if isProtocolError(errStr) {
		return CategoryProtocol
	}

	// Circuit breaker open is retryable (network-level routing)
	if strings.Contains(errStr, "circuit breaker is open") {
		return CategoryNetwork
	}

	// Empty choices or decode errors are server-side issues
	if strings.Contains(errStr, "empty choices") || strings.Contains(errStr, "decode response") {
		return CategoryServerError
	}

	return CategoryUnknown
}

// GetStrategy returns the retry strategy for an error category.
func GetStrategy(category ErrorCategory) Strategy {
	if s, ok := defaultStrategies[category]; ok {
		return s
	}
	return Strategy{MaxRetries: 0, Retryable: false}
}

// IsRetryable returns true if the error category supports retries.
func IsRetryable(err error) bool {
	category := ClassifyError(err)
	return GetStrategy(category).Retryable
}

// CalculateBackoff computes exponential backoff with jitter.
// Formula: min(BaseDelay * 2^attempt + random jitter, MaxDelay)
func CalculateBackoff(strategy Strategy, attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	delay := strategy.BaseDelay * time.Duration(1<<uint(attempt-1))
	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	// Add jitter: random duration in [0, delay/2)
	jitter := time.Duration(rand.Int63n(int64(delay / 2)))
	delay += jitter

	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	return delay
}

// CalculateBackoffWithRetryAfter uses Retry-After value if available,
// otherwise falls back to exponential backoff.
func CalculateBackoffWithRetryAfter(strategy Strategy, attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	return CalculateBackoff(strategy, attempt)
}

// Result contains the outcome of a retry execution.
type Result struct {
	Attempts   int
	LastErr    error
	Category   ErrorCategory
	Succeeded  bool
	TotalDelay time.Duration
}

// Execute runs fn with retry logic based on error classification.
// The fn function should return nil on success, or an error on failure.
// The error will be classified to determine if and how to retry.
func Execute(ctx context.Context, fn func() error) *Result {
	result := &Result{}

	err := fn()
	result.Attempts = 1
	result.LastErr = err

	if err == nil {
		result.Succeeded = true
		return result
	}

	category := ClassifyError(err)
	result.Category = category
	strategy := GetStrategy(category)

	if !strategy.Retryable {
		return result
	}

	for attempt := 1; attempt <= strategy.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			return result
		default:
		}

		delay := CalculateBackoff(strategy, attempt)
		result.TotalDelay += delay

		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			return result
		case <-time.After(delay):
		}

		err = fn()
		result.Attempts++
		result.LastErr = err

		if err == nil {
			result.Succeeded = true
			result.Category = category
			return result
		}

		// Reclassify — the new error might be a different category
		newCategory := ClassifyError(err)
		if newCategory != category {
			newStrategy := GetStrategy(newCategory)
			if !newStrategy.Retryable {
				result.Category = newCategory
				return result
			}
			// Switch to new strategy for remaining retries
			category = newCategory
			strategy = newStrategy
			result.Category = category
		}
	}

	return result
}

// --- Internal helpers ---

var permanentQuotaSignals = []string{
	"daily_limit_exceeded",
	"daily usage limit exceeded",
	"usage_limit_exceeded",
	"insufficient_quota",
	"quota_exceeded",
	"billing_hard_limit_reached",
	"credit_balance_too_low",
}

func isPermanentQuotaError(errStr string) bool {
	for _, signal := range permanentQuotaSignals {
		if strings.Contains(errStr, signal) {
			return true
		}
	}
	return false
}

func isServerErrorStatus(errStr string) bool {
	codes := []string{"status 500", "status 502", "status 503", "status 504", "status 506", "status 507", "status 508"}
	for _, code := range codes {
		if strings.Contains(errStr, code) {
			return true
		}
	}
	return false
}

var networkSignals = []string{
	"connection refused", "connection reset", "connection timeout",
	"no such host", "network is unreachable", "broken pipe",
	"i/o timeout", "temporary failure", "dns error",
	"tls handshake timeout", "temporary error",
}

func isNetworkError(errStr string) bool {
	for _, sig := range networkSignals {
		if strings.Contains(errStr, sig) {
			return true
		}
	}
	return false
}

func isAuthError(errStr string) bool {
	return strings.Contains(errStr, "status 401") ||
		strings.Contains(errStr, "status 403") ||
		strings.Contains(errStr, "invalid_api_key") ||
		strings.Contains(errStr, "unauthorized")
}

func isProtocolError(errStr string) bool {
	return strings.Contains(errStr, "status 400") ||
		strings.Contains(errStr, "status 404") ||
		strings.Contains(errStr, "status 422")
}

// String returns human-readable error category name.
func (c ErrorCategory) String() string {
	switch c {
	case CategoryRateLimit:
		return "rate_limit"
	case CategoryServerError:
		return "server_error"
	case CategoryNetwork:
		return "network"
	case CategoryProtocol:
		return "protocol"
	case CategoryAuth:
		return "auth"
	case CategoryPermanentQuota:
		return "permanent_quota"
	case CategoryCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 2: 创建 retry package 单元测试**

```go
package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestClassifyError_RateLimit(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"status 429", fmt.Errorf("OpenAI API error (status 429): rate limit")},
		{"rate limit message", fmt.Errorf("rate limit exceeded")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != CategoryRateLimit {
				t.Errorf("got %v, want CategoryRateLimit", got)
			}
		})
	}
}

func TestClassifyError_PermanentQuota(t *testing.T) {
	err := fmt.Errorf("status 429: insufficient_quota")
	if got := ClassifyError(err); got != CategoryPermanentQuota {
		t.Errorf("got %v, want CategoryPermanentQuota", got)
	}
}

func TestClassifyError_ServerError(t *testing.T) {
	codes := []string{"500", "502", "503", "504"}
	for _, code := range codes {
		err := fmt.Errorf("status %s error", code)
		if got := ClassifyError(err); got != CategoryServerError {
			t.Errorf("status %s: got %v, want CategoryServerError", code, got)
		}
	}
}

func TestClassifyError_Network(t *testing.T) {
	signals := []string{"connection refused", "connection reset", "i/o timeout", "broken pipe"}
	for _, sig := range signals {
		err := fmt.Errorf("%s something", sig)
		if got := ClassifyError(err); got != CategoryNetwork {
			t.Errorf("%q: got %v, want CategoryNetwork", sig, got)
		}
	}
}

func TestClassifyError_Auth(t *testing.T) {
	err := fmt.Errorf("status 401: unauthorized")
	if got := ClassifyError(err); got != CategoryAuth {
		t.Errorf("got %v, want CategoryAuth", got)
	}
}

func TestClassifyError_Protocol(t *testing.T) {
	err := fmt.Errorf("status 400: bad request")
	if got := ClassifyError(err); got != CategoryProtocol {
		t.Errorf("got %v, want CategoryProtocol", got)
	}
}

func TestClassifyError_Cancelled(t *testing.T) {
	if got := ClassifyError(context.Canceled); got != CategoryCancelled {
		t.Errorf("got %v, want CategoryCancelled", got)
	}
}

func TestClassifyError_Nil(t *testing.T) {
	if got := ClassifyError(nil); got != CategoryUnknown {
		t.Errorf("got %v, want CategoryUnknown", got)
	}
}

func TestGetStrategy_RateLimitRetryable(t *testing.T) {
	s := GetStrategy(CategoryRateLimit)
	if !s.Retryable {
		t.Error("rate limit should be retryable")
	}
	if s.MaxRetries != 20 {
		t.Errorf("expected 20 retries, got %d", s.MaxRetries)
	}
}

func TestGetStrategy_ProtocolNotRetryable(t *testing.T) {
	s := GetStrategy(CategoryProtocol)
	if s.Retryable {
		t.Error("protocol errors should not be retryable")
	}
}

func TestGetStrategy_AuthNotRetryable(t *testing.T) {
	s := GetStrategy(CategoryAuth)
	if s.Retryable {
		t.Error("auth errors should not be retryable")
	}
}

func TestCalculateBackoff_Increases(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second}
	prev := time.Duration(0)
	for attempt := 1; attempt <= 6; attempt++ {
		delay := CalculateBackoff(s, attempt)
		// Allow jitter to cause slight variation, but base should increase
		baseDelay := s.BaseDelay * time.Duration(1<<uint(attempt-1))
		if baseDelay > s.MaxDelay {
			baseDelay = s.MaxDelay
		}
		// Actual delay should be >= base (jitter adds) and <= max
		if delay < baseDelay {
			t.Errorf("attempt %d: delay %v < base %v", attempt, delay, baseDelay)
		}
		prev = delay
	}
}

func TestCalculateBackoff_CappedAtMax(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 10 * time.Second}
	for attempt := 1; attempt <= 20; attempt++ {
		delay := CalculateBackoff(s, attempt)
		if delay > s.MaxDelay+5*time.Second { // jitter tolerance
			t.Errorf("attempt %d: delay %v exceeds max+tolerance", attempt, delay)
		}
	}
}

func TestCalculateBackoffWithRetryAfter(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second}
	retryAfter := 30 * time.Second
	delay := CalculateBackoffWithRetryAfter(s, 1, retryAfter)
	if delay != retryAfter {
		t.Errorf("expected Retry-After %v, got %v", retryAfter, delay)
	}
}

func TestCalculateBackoffWithRetryAfter_Fallback(t *testing.T) {
	s := Strategy{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second}
	delay := CalculateBackoffWithRetryAfter(s, 1, 0)
	if delay == 0 {
		t.Error("expected non-zero backoff when Retry-After is 0")
	}
}

func TestExecute_Success(t *testing.T) {
	r := Execute(context.Background(), func() error { return nil })
	if !r.Succeeded {
		t.Error("expected success")
	}
	if r.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", r.Attempts)
	}
}

func TestExecute_NonRetryable(t *testing.T) {
	r := Execute(context.Background(), func() error {
		return fmt.Errorf("status 400: bad request")
	})
	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Attempts != 1 {
		t.Errorf("non-retryable should only attempt once, got %d", r.Attempts)
	}
}

func TestExecute_RateLimitRetries(t *testing.T) {
	callCount := 0
	r := Execute(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("status 429: rate limit")
		}
		return nil
	})
	if !r.Succeeded {
		t.Error("expected success after retries")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestExecute_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := Execute(ctx, func() error {
		return fmt.Errorf("status 429: rate limit")
	})
	if r.Category != CategoryCancelled {
		t.Errorf("expected CategoryCancelled, got %v", r.Category)
	}
}

func TestExecute_CategorySwitchMidRetry(t *testing.T) {
	callCount := 0
	r := Execute(context.Background(), func() error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("status 429: rate limit")
		}
		return fmt.Errorf("status 400: bad request")
	})
	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Category != CategoryProtocol {
		t.Errorf("final category should be protocol, got %v", r.Category)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
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

func TestErrorCategory_String(t *testing.T) {
	tests := []struct {
		cat  ErrorCategory
		want string
	}{
		{CategoryRateLimit, "rate_limit"},
		{CategoryServerError, "server_error"},
		{CategoryNetwork, "network"},
		{CategoryProtocol, "protocol"},
		{CategoryAuth, "auth"},
		{CategoryCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if got := tt.cat.String(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestExecute_ServerErrorRetryCount(t *testing.T) {
	callCount := 0
	r := Execute(context.Background(), func() error {
		callCount++
		return fmt.Errorf("status 500: internal error")
	})
	if r.Succeeded {
		t.Error("should not succeed")
	}
	expected := GetStrategy(CategoryServerError).MaxRetries + 1 // +1 for initial attempt
	if callCount != expected {
		t.Errorf("expected %d calls, got %d", expected, callCount)
	}
}
```

- [ ] **Step 3: 验证 retry package 编译和测试**
Run: `go test ./retry/ -v -count=1 2>&1 | tail -30`
Expected:
  - Exit code: 0
  - Output contains: "PASS"
  - Output does NOT contain: "FAIL"

- [ ] **Step 4: 提交**
Run: `git add retry/retry.go retry/retry_test.go go.mod && git commit -m "feat(retry): add smart retry package with error classification and category-based strategies"`

---

### Task 2: Integrate Retry Package into Handler

**Depends on:** Task 1
**Files:**
- Modify: `handler/handler.go:326-375`（替换 handleMessageWithConfig 中的内联重试）
- Modify: `handler/handler.go:553-562`（流式 stream creation 失败时重试）

- [ ] **Step 1: 替换 handleMessageWithConfig 内联重试为 retry.Execute — 统一重试逻辑**

文件: `handler/handler.go:326-375`（替换整个 handleMessageWithConfig 函数体）

```go
	// handleMessageWithConfig 处理消息请求的核心逻辑（智能重试）
func (h *Handler) handleMessageWithConfig(c *gin.Context, dbConfig *database.APIConfig, betaHeaders []string) {
	logger := utils.GetLogger()

	result := retry.Execute(c.Request.Context(), func() error {
		return h.executeMessageRequestWithConfig(c, dbConfig, nil, betaHeaders)
	})

	if result.Succeeded {
		if result.Attempts > 1 {
			logger.Info("  Request succeeded after %d attempts (category: %s, total delay: %v)",
				result.Attempts, result.Category, result.TotalDelay)
		}
		return
	}

	logger.Error("← [handleMessageWithConfig] Failed after %d attempts (category: %s): %v",
		result.Attempts, result.Category, result.LastErr)
}
```

**Why:** 原来硬编码 maxRetries=20、所有错误同一退避策略。新逻辑自动按错误类型选择策略：429 最多 20 次长退避，5xx 最多 5 次中退避，400 立即失败。context cancel 自动中断。

- [ ] **Step 2: 为流式路径的 stream creation 增加重试 — 429/5xx 时重建 stream**

文件: `handler/handler.go:553-562`（替换 stream creation 错误处理）

在现有的 `if req.Stream {` 区块内，stream creation 失败处，用 retry.Execute 包裹 stream creation 调用：

```go
		if req.Stream {
			targetClient.BetaHeaders = betaHeaders

			// Stream creation 失败时重试（429/5xx），SSE response 尚未开始写入
			var reader *http.Response
			streamResult := retry.Execute(c.Request.Context(), func() error {
				var err error
				reader, err = targetClient.CreateChatCompletionStream(openAIReq)
				if err != nil {
					logger.Warn("  Stream creation failed (will retry if retryable): %v", err)
					return err
				}
				return nil
			})

			if !streamResult.Succeeded {
				logger.Error("← [executeMessageRequestWithConfig] Stream creation failed after %d attempts: %v",
					streamResult.Attempts, streamResult.LastErr)
				h.responseHandler.SendErrorResponse(c, streamResult.LastErr)
				h.responseHandler.logRequestWithDetails(c, configID, openAIReq.Model, 0, 0, startTime, "error", streamResult.LastErr.Error(), &req, nil)
				return fmt.Errorf("stream creation failed after %d attempts: %w", streamResult.Attempts, streamResult.LastErr)
			}
			defer reader.Body.Close()
```

**注意：** 原代码 `reader` 类型是 `*http.Response`（有 Body 字段），如果 `CreateChatCompletionStream` 返回的是自定义类型则需调整。执行者需检查实际返回类型。

- [ ] **Step 3: 验证编译**
Run: `go build ./handler/ 2>&1 | head -10`
Expected:
  - Exit code: 0
  - No error output

- [ ] **Step 4: 运行 handler 测试**
Run: `go test ./handler/ -run "Test.*Retry" -count=1 -v 2>&1 | tail -20`
Expected:
  - Exit code: 0
  - Existing retry tests still pass

- [ ] **Step 5: 提交**
Run: `git add handler/handler.go && git commit -m "feat(handler): integrate smart retry package with category-based retry strategies"`

---

### Task 3: Delegate Client Retry Logic to Retry Package

**Depends on:** Task 1
**Files:**
- Modify: `client/openai_client.go:62-167`（删除重复的 IsRetryableError、CalculateBackoff、isPermanentQuotaError，委托给 retry package）

- [ ] **Step 1: 替换 client/openai_client.go 中的重复重试逻辑 — 委托给 retry package**

文件: `client/openai_client.go:62-167`（替换 RetryConfig 常量 + CalculateBackoff + IsRetryableError + isPermanentQuotaError 为委托）

将以下代码区块：
```go
// RetryConfig holds retry configuration
const (
	DefaultRetryCount     = 20
	...
)

// CalculateBackoff ...
func (c *OpenAIClient) CalculateBackoff(attempt int) time.Duration { ... }

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool { ... }

func isPermanentQuotaError(errorDetail string) bool { ... }
```

替换为：

```go
import "github.com/vibe-coding-labs/claude-code-cli-with-openai-api/retry"

// IsRetryableError delegates to the retry package for consistent error classification.
// Kept as package-level function for backward compatibility.
func IsRetryableError(err error) bool {
	return retry.IsRetryable(err)
}
```

**注意：** `CalculateBackoff` 方法在 OpenAIClient 上目前可能被其他代码引用。执行者需 `grep -rn "CalculateBackoff\|DefaultRetryCount\|BaseBackoffDelay\|MaxBackoffDelay" --include="*.go"` 确认无遗漏引用，如有则也需更新。保留 `IsRetryableError` 函数签名不变（委托实现）以避免大范围改动。

- [ ] **Step 2: 验证编译**
Run: `go build ./client/ 2>&1 | head -10`
Expected:
  - Exit code: 0

- [ ] **Step 3: 运行 client 测试**
Run: `go test ./client/ -count=1 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

- [ ] **Step 4: 提交**
Run: `git add client/openai_client.go && git commit -m "refactor(client): delegate retry logic to unified retry package"`

---

### Task 4: Delegate LB RetryHandler to Retry Package

**Depends on:** Task 1, Task 3
**Files:**
- Modify: `handler/retry_handler.go:112-176`（IsRetryableError 委托给 retry package）

- [ ] **Step 1: 替换 DefaultRetryHandler.IsRetryableError — 委托给 retry.ClassifyError**

文件: `handler/retry_handler.go:112-176`（替换 IsRetryableError 方法）

将整个 `func (rh *DefaultRetryHandler) IsRetryableError(err error) bool { ... }` 方法替换为：

```go
// IsRetryableError delegates to the retry package for consistent error classification.
func (rh *DefaultRetryHandler) IsRetryableError(err error) bool {
	category := retry.ClassifyError(err)
	return retry.GetStrategy(category).Retryable
}
```

同步在文件顶部添加 import：
```go
import "github.com/vibe-coding-labs/claude-code-cli-with-openai-api/retry"
```

- [ ] **Step 2: 验证编译和测试**
Run: `go test ./handler/ -run "Test.*Retry|TestIsRetryable" -count=1 -v 2>&1 | tail -20`
Expected:
  - Exit code: 0
  - Existing retry handler tests pass

- [ ] **Step 3: 全量编译验证**
Run: `go build ./... 2>&1 | head -10`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add handler/retry_handler.go && git commit -m "refactor(handler): delegate LB retry handler to unified retry package"`
