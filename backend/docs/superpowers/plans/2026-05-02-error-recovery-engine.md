# Error Recovery Engine — 在不稳定上游上构建稳定服务

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 重构 retry package 为带状态流转的错误恢复引擎。每次重试都重新分类错误，按错误类型分配独立重试预算，自适应调整退避策略，让下游（Claude Code）感知到一个极其稳定的服务。

**Architecture:** 请求失败 → ClassifyError → 选择 RecoveryState → 执行重试循环 → 每次重试重新分类 → 状态可动态切换 → 各状态独立预算（429 有 20 次，5xx 有 5 次）→ 退避自适应（连续 429 则自动增加延迟）→ 最终成功或提供结构化错误摘要。核心不变：每次重试是一个全新的错误分类，不是初始分类决定一切。

**Tech Stack:** Go 1.22, context.Context, 无新外部依赖

**Risks:**
- Task 1 重写 retry.go 核心逻辑 → 缓解：保持 ClassifyError / IsRetryable 等公共 API 不变
- Task 3 流式中途断开恢复复杂 → 缓解：stream creation 阶段用 Engine 重试，streaming 中途断开暂不重试（需 SSE 已发送数据不可撤回）

---

### Task 1: Upgrade Retry Package to Error Recovery Engine

**Depends on:** None
**Files:**
- Modify: `retry/retry.go`（在现有代码基础上增强，保留 ClassifyError 和 ErrorCategory 体系）
- Modify: `retry/retry_test.go`（增加 Engine 和状态流转测试）

- [ ] **Step 1: 在 retry.go 中添加 Engine 结构体和 RecoveryState — 状态机核心**

文件: `retry/retry.go`（在文件末尾 `FormatRetryError` 之后追加）

```go
// RecoveryState tracks the state machine for a single request recovery process.
type RecoveryState struct {
	// Per-category retry budgets — each error type gets its own retry count
	categoryAttempts map[ErrorCategory]int
	// Track the error history for structured error reporting
	errorHistory []ErrorRecord
	// Total delay accumulated across all retries
	totalDelay time.Duration
	// Adaptive backoff multiplier — increases when consecutive same-category errors
	backoffMultiplier float64
	// Consecutive same-category error count (resets on category switch or success)
	consecutiveSameCategory int
}

// ErrorRecord captures a single error event during recovery.
type ErrorRecord struct {
	Attempt   int
	Category  ErrorCategory
	Error     error
	Timestamp time.Time
	Delay     time.Duration
}

// Engine provides advanced error recovery with stateful retry management.
type Engine struct {
	strategies map[ErrorCategory]Strategy
}

// NewEngine creates a recovery engine with default strategies.
func NewEngine() *Engine {
	return &Engine{strategies: defaultStrategies}
}

// NewEngineWithStrategies creates a recovery engine with custom strategy overrides.
func NewEngineWithStrategies(strategies map[ErrorCategory]Strategy) *Engine {
	merged := make(map[ErrorCategory]Strategy)
	for k, v := range defaultStrategies {
		merged[k] = v
	}
	for k, v := range strategies {
		merged[k] = v
	}
	return &Engine{strategies: merged}
}

// newRecoveryState creates a fresh recovery state.
func newRecoveryState() *RecoveryState {
	return &RecoveryState{
		categoryAttempts: make(map[ErrorCategory]int),
		errorHistory:     nil,
		totalDelay:       0,
		backoffMultiplier: 1.0,
		consecutiveSameCategory: 0,
	}
}

// Execute runs fn with full state-machine retry logic.
// Key difference from retry.Execute: per-category retry budgets and adaptive backoff.
func (e *Engine) Execute(ctx context.Context, fn func() error) *Result {
	state := newRecoveryState()
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
	state.errorHistory = append(state.errorHistory, ErrorRecord{
		Attempt: 1, Category: category, Error: err, Timestamp: time.Now(),
	})

	strategy := getStrategy(e.strategies, category)
	if !strategy.Retryable {
		return result
	}

	totalAttempt := 1
	for {
		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			result.TotalDelay = state.totalDelay
			return result
		default:
		}

		// Check per-category retry budget
		state.categoryAttempts[category]++
		if state.categoryAttempts[category] > strategy.MaxRetries {
			break
		}

		// Calculate delay with adaptive multiplier
		state.consecutiveSameCategory++
		state.backoffMultiplier = 1.0 + float64(state.consecutiveSameCategory-1)*0.2
		delay := CalculateBackoff(strategy, state.categoryAttempts[category])
		delay = time.Duration(float64(delay) * state.backoffMultiplier)
		if delay > strategy.MaxDelay*2 {
			delay = strategy.MaxDelay * 2
		}

		state.totalDelay += delay

		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			result.Category = CategoryCancelled
			result.TotalDelay = state.totalDelay
			return result
		case <-time.After(delay):
		}

		err = fn()
		totalAttempt++
		result.Attempts = totalAttempt
		result.LastErr = err

		if err == nil {
			result.Succeeded = true
			result.Category = category
			result.TotalDelay = state.totalDelay
			return result
		}

		// Re-classify — state machine can transition
		newCategory := ClassifyError(err)
		state.errorHistory = append(state.errorHistory, ErrorRecord{
			Attempt: totalAttempt, Category: newCategory, Error: err,
			Timestamp: time.Now(), Delay: delay,
		})

		if newCategory != category {
			// State transition — reset consecutive counter
			state.consecutiveSameCategory = 0
			state.backoffMultiplier = 1.0
			category = newCategory
			strategy = getStrategy(e.strategies, category)
			result.Category = category
			if !strategy.Retryable {
				break
			}
		}
	}

	result.TotalDelay = state.totalDelay
	return result
}

// ErrorSummary returns a human-readable summary of all errors during recovery.
func (r *Result) ErrorSummary() string {
	if r.LastErr == nil {
		return ""
	}
	if r.Attempts <= 1 {
		return r.LastErr.Error()
	}
	return fmt.Sprintf("failed after %d attempts (category: %s, total delay: %v): %v",
		r.Attempts, r.Category, r.TotalDelay, r.LastErr)
}
```

- [ ] **Step 2: 在 retry_test.go 中添加 Engine 状态流转测试**

文件: `retry/retry_test.go`（在文件末尾追加）

```go
func TestEngine_PerCategoryBudget(t *testing.T) {
	// Simulate: first 5xx, then 429, then 5xx — each category has its own budget
	testStrategies := map[ErrorCategory]Strategy{
		CategoryServerError: {MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
		CategoryRateLimit:   {MaxRetries: 5, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	callCount := 0
	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		callCount++
		if callCount <= 2 {
			return fmt.Errorf("status 500: server error")
		}
		if callCount <= 4 {
			return fmt.Errorf("status 429: rate limit")
		}
		if callCount <= 5 {
			return fmt.Errorf("status 500: server error")
		}
		return nil
	})

	if !r.Succeeded {
		t.Errorf("expected success, got: %v", r.LastErr)
	}
	if r.Attempts != 6 {
		t.Errorf("expected 6 attempts, got %d", r.Attempts)
	}
}

func TestEngine_CategoryExhaustion(t *testing.T) {
	// 5xx budget = 2, should exhaust and fail
	testStrategies := map[ErrorCategory]Strategy{
		CategoryServerError: {MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		return fmt.Errorf("status 500: always fails")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Category != CategoryServerError {
		t.Errorf("expected CategoryServerError, got %v", r.Category)
	}
}

func TestEngine_StateTransition_StopsOnNonRetryable(t *testing.T) {
	// Start with 429 (retryable), then get 400 (non-retryable) → should stop immediately
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 10, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	callCount := 0
	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("status 429: rate limit")
		}
		return fmt.Errorf("status 400: bad request")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Attempts != 2 {
		t.Errorf("expected 2 attempts (429 then 400), got %d", r.Attempts)
	}
	if r.Category != CategoryProtocol {
		t.Errorf("final category should be protocol, got %v", r.Category)
	}
}

func TestEngine_AdaptiveBackoff(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 4, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Retryable: true},
	}

	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		return fmt.Errorf("status 429: rate limit")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	// Adaptive backoff should cause total delay to be > simple exponential
	// Base 10ms: attempt1=10ms*1.0, attempt2=20ms*1.2, attempt3=40ms*1.4, attempt4=50ms*1.6
	// Total should be > 100ms
	if r.TotalDelay < 50*time.Millisecond {
		t.Errorf("total delay %v seems too low for adaptive backoff", r.TotalDelay)
	}
}

func TestEngine_SuccessAfterRetries(t *testing.T) {
	testStrategies := map[ErrorCategory]Strategy{
		CategoryRateLimit: {MaxRetries: 10, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Retryable: true},
	}

	callCount := 0
	engine := NewEngineWithStrategies(testStrategies)
	r := engine.Execute(context.Background(), func() error {
		callCount++
		if callCount < 5 {
			return fmt.Errorf("status 429: rate limit")
		}
		return nil
	})

	if !r.Succeeded {
		t.Error("expected success")
	}
	if r.Attempts != 5 {
		t.Errorf("expected 5 attempts, got %d", r.Attempts)
	}
	if r.Category != CategoryRateLimit {
		t.Errorf("expected CategoryRateLimit, got %v", r.Category)
	}
}

func TestEngine_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := NewEngine()
	r := engine.Execute(ctx, func() error {
		return fmt.Errorf("status 429: rate limit")
	})

	if r.Category != CategoryCancelled {
		t.Errorf("expected CategoryCancelled, got %v", r.Category)
	}
}

func TestEngine_NonRetryableImmediateReturn(t *testing.T) {
	engine := NewEngine()
	r := engine.Execute(context.Background(), func() error {
		return fmt.Errorf("status 400: bad request")
	})

	if r.Succeeded {
		t.Error("should not succeed")
	}
	if r.Attempts != 1 {
		t.Errorf("should be 1 attempt for non-retryable, got %d", r.Attempts)
	}
}

func TestResult_ErrorSummary(t *testing.T) {
	r := &Result{Attempts: 1, LastErr: fmt.Errorf("simple error")}
	if !strings.Contains(r.ErrorSummary(), "simple error") {
		t.Error("single attempt should return raw error")
	}

	r2 := &Result{
		Attempts: 5, Category: CategoryRateLimit, TotalDelay: 10 * time.Second,
		LastErr: fmt.Errorf("rate limit"),
	}
	summary := r2.ErrorSummary()
	if !strings.Contains(summary, "5 attempts") || !strings.Contains(summary, "rate_limit") {
		t.Errorf("summary missing context: %s", summary)
	}
}
```

- [ ] **Step 3: 验证 retry package**
Run: `go test ./retry/ -v -count=1 -run "TestEngine" 2>&1 | tail -30`
Expected:
  - Exit code: 0
  - Output contains: "PASS" for all TestEngine_* tests

- [ ] **Step 4: 提交**
Run: `git add retry/retry.go retry/retry_test.go && git commit -m "feat(retry): upgrade to error recovery engine with per-category budgets and adaptive backoff"`

---

### Task 2: Integrate Engine into Handler

**Depends on:** Task 1
**Files:**
- Modify: `handler/handler.go:327-340`（handleMessageWithConfig 改用 Engine）
- Modify: `handler/handler.go:524-538`（stream creation 改用 Engine）

- [ ] **Step 1: 替换 handleMessageWithConfig 使用 Engine — 提供状态机级别重试**

文件: `handler/handler.go:327-340`（替换 handleMessageWithConfig 函数）

将当前：
```go
result := retry.Execute(c.Request.Context(), func() error {
    return h.executeMessageRequestWithConfig(c, dbConfig, nil, betaHeaders)
})
```

替换为使用 Engine：

```go
func (h *Handler) handleMessageWithConfig(c *gin.Context, dbConfig *database.APIConfig, betaHeaders []string) {
	logger := utils.GetLogger()

	engine := retry.NewEngine()
	result := engine.Execute(c.Request.Context(), func() error {
		return h.executeMessageRequestWithConfig(c, dbConfig, nil, betaHeaders)
	})

	if result.Succeeded {
		if result.Attempts > 1 {
			logger.Info("  Request succeeded after %d attempts (category: %s, total delay: %v)",
				result.Attempts, result.Category, result.TotalDelay)
		}
		return
	}

	logger.Error("← [handleMessageWithConfig] %s", result.ErrorSummary())
}
```

- [ ] **Step 2: 替换 stream creation 使用 Engine — 流式启动阶段智能重试**

文件: `handler/handler.go:524-538`（替换 stream creation 的 retry.Execute 为 Engine）

将当前：
```go
createResult := retry.Execute(c.Request.Context(), func() error { ... })
```

替换为：

```go
createResult := retry.NewEngine().Execute(c.Request.Context(), func() error {
    var err error
    reader, err = targetClient.CreateChatCompletionStream(openAIReq)
    return err
})
```

- [ ] **Step 3: 验证编译**
Run: `go build ./handler/ 2>&1 | head -5`
Expected:
  - Exit code: 0

- [ ] **Step 4: 提交**
Run: `git add handler/handler.go && git commit -m "refactor(handler): upgrade retry to Engine with per-category recovery budgets"`

---

### Task 3: Verify Full Pipeline and Rebuild

**Depends on:** Task 2
**Files:**
- None (verification only)

- [ ] **Step 1: 运行全部受影响包的测试**
Run: `go test ./retry/ ./converter/ ./client/ -count=1 2>&1 | tail -10`
Expected:
  - Exit code: 0
  - Output contains: "ok" for retry and client

- [ ] **Step 2: 全量编译**
Run: `go build -tags dev -o claude-proxy-server . 2>&1 | head -5`
Expected:
  - Exit code: 0

- [ ] **Step 3: 重启后端服务**
Run: `lsof -i :54989 -t | xargs kill 2>/dev/null; sleep 1; ./claude-proxy-server server --port 54989 > /tmp/ccr-backend.log 2>&1 &`
Expected: Server starts on port 54989

- [ ] **Step 4: 提交**
Run: `git add -A && git commit -m "chore: rebuild and deploy error recovery engine"`
