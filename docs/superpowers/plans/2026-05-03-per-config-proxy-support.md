# Per-Config HTTP Proxy Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 每个 API 配置支持独立的 HTTP 代理设置。访问 Google Gemini 等被墙 API 时走代理，访问国内可达 API 时直连。

**Architecture:** 用户在前端配置 `proxy_url` 字段 → 存入数据库 `api_configs` 表 → 请求时 `APIConfig.ToConfig()` 传递到 `config.Config` → `NewOpenAIClient` 根据 `ProxyURL` 创建带代理或直连的 HTTP Transport → Go `http.Transport.Proxy` 使用 `proxyURLFunc` 动态决定是否走代理。

**Tech Stack:** Go 1.24, Gin, SQLite, React 18, TypeScript, Ant Design

**Risks:**
- Task 1 修改 `api_configs` 表添加列 → 缓解：纯 ADD COLUMN，无破坏性
- Task 2 修改 HTTP Transport 创建逻辑 → 缓解：空 proxy_url 时回退到 `http.ProxyFromEnvironment`，行为向后兼容

---

### Task 1: Database Migration + Backend Model

**Depends on:** None
**Files:**
- Create: `backend/database/migrations/035_add_proxy_url_to_api_configs.sql`
- Modify: `backend/database/types.go` (APIConfig struct, add ProxyURL field)
- Modify: `backend/database/models.go` (GetConfigByAnthropicAPIKey SQL query, ToConfig method)
- Modify: `backend/config/config.go` (Config struct, add ProxyURL field)

- [ ] **Step 1: 创建数据库迁移 — 添加 proxy_url 列到 api_configs 表**

```sql
-- backend/database/migrations/035_add_proxy_url_to_api_configs.sql
ALTER TABLE api_configs ADD COLUMN proxy_url TEXT DEFAULT '';
```

- [ ] **Step 2: 修改 APIConfig struct — 添加 ProxyURL 字段**
文件: `backend/database/types.go`（APIConfig struct，在 RetryBackoffMax 字段之后）

```go
// 在 RetryBackoffMax 字段后面添加:
ProxyURL              string            `json:"proxy_url,omitempty"`
```

- [ ] **Step 3: 修改数据库查询 — 在 SELECT 中加入 proxy_url**
文件: `backend/database/models.go`

在 `GetConfigByAnthropicAPIKey` 函数中（约 Line 158-221），SQL query 的 SELECT 列表中添加 `proxy_url`。

同时在 scan 行中添加 `&dbConfig.ProxyURL`。

在 `GetConfigByID` 函数中做同样修改。

在 `GetAllConfigs` 函数中做同样修改。

在 `CreateConfig` 函数中 INSERT 语句添加 `proxy_url` 列。

在 `UpdateConfig` 函数中 UPDATE 语句添加 `proxy_url` 列。

- [ ] **Step 4: 修改 ToConfig 方法 — 传递 ProxyURL 到 Config**
文件: `backend/database/models.go`（ToConfig 方法）

```go
// 在 ToConfig() 方法中添加:
ProxyURL: c.ProxyURL,
```

- [ ] **Step 5: 修改 Config struct — 添加 ProxyURL 字段**
文件: `backend/config/config.go`（Config struct）

```go
// 在 RetryBackoffMax 字段后面添加:
ProxyURL              string // HTTP proxy URL for upstream API requests
```

- [ ] **Step 6: 验证编译**
Run: `go build -C /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend -tags dev -o ccr-server .`
Expected:
  - Exit code: 0
  - No error output

- [ ] **Step 7: 提交**
Run: `git add backend/database/migrations/035_add_proxy_url_to_api_configs.sql backend/database/types.go backend/database/models.go backend/config/config.go && git commit -m "feat(config): add per-config proxy_url field to database and model"`

---

### Task 2: HTTP Client Proxy Support

**Depends on:** Task 1
**Files:**
- Modify: `backend/client/openai_client.go` (NewOpenAIClient function, transport creation)

- [ ] **Step 1: 修改 NewOpenAIClient — 根据 ProxyURL 创建不同的 Transport**
文件: `backend/client/openai_client.go:257-270`（替换 transport 创建区块）

```go
// 替换 transport 创建区块（约 Line 257-270）
var proxyFunc func(*http.Request) (*url.URL, error)
if cfg.ProxyURL != "" {
	proxyURL, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		logger.Warn("Invalid proxy URL %s: %v, falling back to direct", cfg.ProxyURL, err)
		proxyFunc = http.ProxyFromEnvironment
	} else {
		proxyFunc = http.ProxyURL(proxyURL)
	}
} else {
	proxyFunc = http.ProxyFromEnvironment
}

transport := &http.Transport{
	Proxy:                 proxyFunc,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   100,
	MaxConnsPerHost:       0,
	IdleConnTimeout:       90 * time.Second,
	DisableKeepAlives:     false,
	DisableCompression:    false,
	ForceAttemptHTTP2:     true,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}
```

需要在文件顶部确认 `net/url` 已在 import 中（如果尚未导入则添加）。

- [ ] **Step 2: 验证编译**
Run: `go build -C /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend -tags dev -o ccr-server .`
Expected:
  - Exit code: 0

- [ ] **Step 3: 提交**
Run: `git add backend/client/openai_client.go && git commit -m "feat(client): use per-config proxy URL for upstream HTTP requests"`

---

### Task 3: Frontend UI — Proxy URL Form Field

**Depends on:** Task 1
**Files:**
- Modify: `frontend/src/types/api.ts` (add proxy_url to interfaces)
- Modify: `frontend/src/components/ConfigCreate.tsx` (add form field)
- Modify: `frontend/src/components/ConfigEdit.tsx` (add form field)

- [ ] **Step 1: 更新 TypeScript 类型 — 添加 proxy_url 字段**
文件: `frontend/src/types/api.ts`

在 `APIConfig` interface 中添加:
```typescript
proxy_url?: string;
```

在 `APIConfigRequest` interface 中添加:
```typescript
proxy_url?: string;
```

- [ ] **Step 2: 在 ConfigCreate 组件中添加代理 URL 输入框**
文件: `frontend/src/components/ConfigCreate.tsx`

在 "OpenAI API Configuration" 区块中 `openai_base_url` 字段之后添加代理 URL 输入框：

```tsx
<Form.Item
  label="代理 URL"
  name="proxy_url"
  tooltip="访问上游 API 时使用的 HTTP 代理，如 http://127.0.0.1:7890。留空则直连"
>
  <Input placeholder="http://127.0.0.1:7890（可选）" />
</Form.Item>
```

- [ ] **Step 3: 在 ConfigEdit 组件中添加代理 URL 输入框**
文件: `frontend/src/components/ConfigEdit.tsx`

在 `openai_base_url` 字段之后添加同样的代理 URL 输入框：

```tsx
<Form.Item
  label="代理 URL"
  name="proxy_url"
  tooltip="访问上游 API 时使用的 HTTP 代理，如 http://127.0.0.1:7890。留空则直连"
>
  <Input placeholder="http://127.0.0.1:7890（可选）" />
</Form.Item>
```

确保 `initialValues` 或 `form.setFieldsValue` 中包含 `proxy_url` 字段。

- [ ] **Step 4: 验证前端编译**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected:
  - Exit code: 0
  - No type errors related to proxy_url

- [ ] **Step 5: 提交**
Run: `git add frontend/src/types/api.ts frontend/src/components/ConfigCreate.tsx frontend/src/components/ConfigEdit.tsx && git commit -m "feat(ui): add proxy URL field to config create/edit forms"`

---

### Task 4: Integration Build & Smoke Test

**Depends on:** Task 2, Task 3
**Files:** None (verification only)

- [ ] **Step 1: 完整编译后端**
Run: `go build -C /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend -tags dev -o ccr-server .`
Expected:
  - Exit code: 0

- [ ] **Step 2: 验证前端无编译错误**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/frontend && npx tsc --noEmit`
Expected:
  - Exit code: 0

- [ ] **Step 3: 提交（如有遗漏的文件）**
Run: `git status`
