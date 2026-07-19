# 复制商汤(SenseNova) 5 个 key 配置到本机并配置快捷启动指令 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 从 local-server-002 (192.168.1.90) 复制 5 个商汤 SenseNova key 配置（含加密上游 key、客户端鉴权 UUID、模型映射、代理设置）到当前机器的代理服务（端口 54988）数据库中，并创建 5 个快捷启动指令 `st-cc-001~005`，使本机可直接用这 5 个 key 驱动 Claude Code。

**Architecture:** 002 上的代理数据库 `data/proxy.db` 的 `api_configs` 表存有 5 条 `SenseNova: deepseek-v4-flash` 配置，每条含加密的商汤上游 key（AES-256-GCM，硬编码全局加密 key，可跨实例直接复制密文）和明文客户端鉴权 UUID（`anthropic_api_key`）。数据流：st-cc-00X 脚本 → 用对应 UUID 作为 `ANTHROPIC_API_KEY` 请求 `127.0.0.1:54988` → 代理按 UUID 匹配 `api_configs.anthropic_api_key` 找到 config → 用加密上游 key 解密后请求商汤 `https://token.sensenova.cn/v1`（经本机 127.0.0.1:7890 clash 代理）。本机已有 7890 代理且表结构与 002 兼容，故 `proxy_url` 无需改、加密密文可直接复用。

**Tech Stack:** SQLite 3（proxy.db），Go 代理服务（claude-with-openai-api，54988），Bash 启动脚本，Claude Code CLI，SSH（local-server-002=192.168.1.90）。

**Risks:**
- 本机与 002 的 `api_configs` 表列顺序不同（002 末尾多 `model_mappings` 列，`stream_stall_timeout` 位置不同）→ 缓解：用显式列名 INSERT，不用 `SELECT *` 导出，Task 2 已写出按本机 26 列顺序的精确语句。
- `anthropic_api_key` 有 UNIQUE 约束 → 缓解：5 个 UUID 互不相同，且与本机现有 4 条 config 的 UUID 不冲突（已核对，本机现有为 99c54d04/990fd50b/b45ba6c8/594078c9，导入为 e5e86d37/3506726e/a7d5afae/b3f9a820/2a00321b）。
- 记忆提示：54988 代理可能跑旧二进制，PUT 全量更新会清空未传字段 → 缓解：本方案只 INSERT 新行、不 PUT 更新已有 config，旧二进制问题不影响新增。
- 写 `/usr/local/bin` 需要 sudo → 缓解：Task 3 用 sudo tee 创建，并校验权限 755。
- 本机代理二进制可能滞后于数据库 schema（model_mappings 列本机没有但 002 有）→ 缓解：只导入本机 schema 已有的 26 列，不引入 model_mappings。

---

### Task 1: 备份本机数据库并从 002 导出 5 条配置

**Depends on:** None
**Files:**
- Create: `data/proxy.db.bak-<date>`

- [ ] **Step 1: 备份本机 proxy.db — 防止导入出错可回滚**

```bash
cp data/proxy.db "data/proxy.db.bak-$(date +%Y%m%d-%H%M%S)"
ls -lh data/proxy.db*
```

预期：生成 `data/proxy.db.bak-YYYYMMDD-HHMMSS`，大小与 proxy.db 接近。

- [ ] **Step 2: 确认本机现有 config 不含要导入的 5 个 anthropic_api_key — 防止 UNIQUE 冲突**

```bash
sqlite3 data/proxy.db "SELECT anthropic_api_key, name FROM api_configs WHERE anthropic_api_key IN ('e5e86d37-0bd6-4af0-87e2-f76561fc0a74','3506726e-86a4-4446-ba7c-3dc896a88ffb','a7d5afae-a9ed-4498-8a12-c1304a29f393','b3f9a820-5d1e-4c82-a6f7-3e8b1d4c9050','2a00321b-b0c3-4ba5-9445-2ace27bf7c58');"
```

Expected:
  - Exit code: 0
  - Output: 空（无任何行返回），表示 5 个 key 均未占用，可安全导入。
  - 若有输出，说明已存在对应 config，需先确认是否重复再决定跳过。

- [ ] **Step 3: 提交**
Run: `git add docs/superpowers/plans/2026-07-14-copy-sensenova-keys-and-launch-scripts.md && git commit -m "docs(plans): add plan to copy 5 SenseNova keys and launch scripts from local-server-002"`

---

### Task 2: 导入 5 条配置到本机 proxy.db

**Depends on:** Task 1
**Files:**
- Modify: `data/proxy.db`（`api_configs` 表新增 5 行）

- [ ] **Step 1: 用显式列名 INSERT 5 条商汤 config — 按本机 26 列顺序，密文/UUID/模型/proxy_url 精确对齐**

文件: `data/proxy.db`（api_configs 表）

说明：列名按本机 schema 顺序（id,name,description,user_id,openai_api_key_encrypted,openai_base_url,big_model,middle_model,small_model,max_tokens_limit,request_timeout,retry_count,anthropic_api_key,enabled,created_at,updated_at,supported_models,expires_at,stream_stall_timeout,reasoning_effort,big_model_reasoning_effort,middle_model_reasoning_effort,small_model_reasoning_effort,retry_backoff_base,retry_backoff_max,proxy_url）。值取自 002 导出，加密密文与 UUID 原样保留，proxy_url 保持 `http://127.0.0.1:7890`（本机有同端口 clash）。

```bash
sqlite3 data/proxy.db <<'SQL'
INSERT INTO api_configs (id,name,description,user_id,openai_api_key_encrypted,openai_base_url,big_model,middle_model,small_model,max_tokens_limit,request_timeout,retry_count,anthropic_api_key,enabled,created_at,updated_at,supported_models,expires_at,stream_stall_timeout,reasoning_effort,big_model_reasoning_effort,middle_model_reasoning_effort,small_model_reasoning_effort,retry_backoff_base,retry_backoff_max,proxy_url) VALUES
('fafdffab-2912-47a5-8567-c7a1b17cbf3f','SenseNova: deepseek-v4-flash','SenseNova deepseek-v4-flash (st-cc-001)',2,'LaWs5qj9RaOi4/tA5k8q3mLFIq8iihiiReZert9SZaHvgLPKDfJu9pBBw7ysxsJSekgVDjTv1JP8D52ej+CH','https://token.sensenova.cn/v1','deepseek-v4-flash','deepseek-v4-flash','deepseek-v4-flash',16384,300,10,'e5e86d37-0bd6-4af0-87e2-f76561fc0a74',1,'2026-05-17 05:49:41','2026-05-17 05:49:41','',NULL,60,'','','','',1.0,60,'http://127.0.0.1:7890'),
('ed36d31d-87ac-4869-8f51-dc4ccfbfe92a','SenseNova: deepseek-v4-flash #4','SenseNova deepseek-v4-flash (st-cc-004)',2,'eXodU4Ud1H+SBEvQMH7J6vg3+Y7uJfooPBn3RUZVePmD3+m6TjNPzY9NEvL2Y0Bo5NYDfWc3r4emXjZbU/D','https://token.sensenova.cn/v1','deepseek-v4-flash','deepseek-v4-flash','deepseek-v4-flash',16384,300,10,'3506726e-86a4-4446-ba7c-3dc896a88ffb',1,'2026-07-03 16:23:18','2026-07-03 16:23:18','',NULL,60,'','','','',1.0,60,'http://127.0.0.1:7890'),
('508da186-8aee-4963-ba65-07291678df5c','SenseNova: deepseek-v4-flash #5','SenseNova deepseek-v4-flash (st-cc-005)',2,'O44lAs9S4pTIFdw2HdWi0fEjWQXiLyzUnd17hwRgDhjGdhjesL6+HcLF6vsf4nKAVSRd4+dVosDESarrFRH2','https://token.sensenova.cn/v1','deepseek-v4-flash','deepseek-v4-flash','deepseek-v4-flash',16384,300,10,'a7d5afae-a9ed-4498-8a12-c1304a29f393',1,'2026-07-03 16:23:18','2026-07-03 16:23:18','',NULL,60,'','','','',1.0,60,'http://127.0.0.1:7890'),
('3475433e-33c4-452a-8e30-079a5892daac','SenseNova: deepseek-v4-flash #2','SenseNova deepseek-v4-flash backup (st-cc-002)',2,'svfvvvfi4KUOQ88obpfSY/EvfOK639Ra/YsY/pPatEmaIvEorTXQyrT9nGxXiBxWYK9OQ4v5johGnJIiqTy2','https://token.sensenova.cn/v1','deepseek-v4-flash','deepseek-v4-flash','deepseek-v4-flash',16384,300,10,'b3f9a820-5d1e-4c82-a6f7-3e8b1d4c9050',1,'2026-05-17 15:37:37','2026-05-17 15:48:23','',NULL,60,'','','','',1.0,60,'http://127.0.0.1:7890'),
('7f34e087-2110-438c-90fe-02757cf27802','SenseNova: deepseek-v4-flash #3','Key 3 direct (st-cc-003)',NULL,'ydC5EfEPKV73D4w2CoHVcNmcyYDLXiPTUxMnUwsVz1iQ+0rC0Bq/aMuySxTOW3cwCZi3Xs43vEpAllv88EUK','https://token.sensenova.cn/v1','deepseek-v4-flash','deepseek-v4-flash','deepseek-v4-flash',16384,180,3,'2a00321b-b0c3-4ba5-9445-2ace27bf7c58',1,'2026-05-30 05:35:11','2026-05-30 05:35:11',NULL,NULL,60,'','','','',1.0,60,'http://127.0.0.1:7890');
SQL
```

说明：5 条 name 略作区分（加 `#4`/`#5` 与 `(st-cc-00X)` 标注），避免 name 重复时不易辨认；`anthropic_api_key` UUID 保持与 002 完全一致，确保 st-cc 脚本的客户端 key 能匹配。

- [ ] **Step 2: 验证 5 条 config 已入库且字段完整**
Run: `sqlite3 -header -column data/proxy.db "SELECT substr(id,1,8) AS id, name, anthropic_api_key, big_model, enabled, proxy_url FROM api_configs WHERE name LIKE '%SenseNova%' ORDER BY name;"`
Expected:
  - Exit code: 0
  - Output contains: 5 行（st-cc-001~005 对应），anthropic_api_key 列含 e5e86d37/3506726e/a7d5afae/b3f9a820/2a00321b 开头的 UUID
  - big_model 全部为 `deepseek-v4-flash`，enabled 全为 1

- [ ] **Step 3: 提交数据库备份说明（proxy.db 在 .gitignore 内不提交）**
Run: `git status --short`

说明：`data/` 目录在 .gitignore 内，数据库改动不入 git，无需 git commit。此 Step 仅确认工作树状态。

---

### Task 3: 创建 5 个快捷启动脚本 st-cc-001~005

**Depends on:** Task 2
**Files:**
- Create: `/usr/local/bin/st-cc-001`
- Create: `/usr/local/bin/st-cc-002`
- Create: `/usr/local/bin/st-cc-003`
- Create: `/usr/local/bin/st-cc-004`
- Create: `/usr/local/bin/st-cc-005`

说明：脚本基于 002 的 `shangtang-cc-001~005`，唯一改动是 `ANTHROPIC_BASE_URL` 从 `http://192.168.1.90:54988` 改为 `http://127.0.0.1:54988`（指向本机代理）。`ANTHROPIC_API_KEY` 用对应 config 的客户端 UUID。003/004/005 保留 002 的 `exec script -q -c` PTY 垫层（解决终端滚动卡顿）。

- [ ] **Step 1: 创建 st-cc-001 — 客户端 key e5e86d37（对应 config fafdffab）**

```bash
sudo tee /usr/local/bin/st-cc-001 > /dev/null <<'EOF'
#!/bin/bash
# SenseNova (商汤日日新) deepseek-v4-flash via local proxy — key #1 (st-cc-001)
# 复制自 local-server-002 的 shangtang-cc-001，BASE_URL 改为 127.0.0.1 指向本机代理
API_TIMEOUT_MS=6000000 \
CLAUDE_CODE_MAX_RETRIES=1000000 \
NODE_TLS_REJECT_UNAUTHORIZED=0 \
ANTHROPIC_BASE_URL=http://127.0.0.1:54988 \
ANTHROPIC_API_KEY="e5e86d37-0bd6-4af0-87e2-f76561fc0a74" \
CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1 \
CLAUDE_CODE_MAX_OUTPUT_TOKENS=65536 \
ANTHROPIC_MODEL=deepseek-v4-flash \
claude --dangerously-skip-permissions "$@"
EOF
sudo chmod 755 /usr/local/bin/st-cc-001
```

- [ ] **Step 2: 创建 st-cc-002 — 客户端 key b3f9a820（对应 config 3475433e）**

```bash
sudo tee /usr/local/bin/st-cc-002 > /dev/null <<'EOF'
#!/bin/bash
# SenseNova (商汤日日新) deepseek-v4-flash via local proxy — key #2 (st-cc-002)
# 复制自 local-server-002 的 shangtang-cc-002，BASE_URL 改为 127.0.0.1 指向本机代理
API_TIMEOUT_MS=6000000 \
CLAUDE_CODE_MAX_RETRIES=1000000 \
NODE_TLS_REJECT_UNAUTHORIZED=0 \
ANTHROPIC_BASE_URL=http://127.0.0.1:54988 \
ANTHROPIC_API_KEY="b3f9a820-5d1e-4c82-a6f7-3e8b1d4c9050" \
CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1 \
CLAUDE_CODE_MAX_OUTPUT_TOKENS=65536 \
ANTHROPIC_MODEL=deepseek-v4-flash \
claude --dangerously-skip-permissions "$@"
EOF
sudo chmod 755 /usr/local/bin/st-cc-002
```

- [ ] **Step 3: 创建 st-cc-003 — 客户端 key 2a00321b（对应 config 7f34e087，带 PTY 垫层）**

```bash
sudo tee /usr/local/bin/st-cc-003 > /dev/null <<'EOF'
#!/bin/bash
# SenseNova (商汤日日新) deepseek-v4-flash via local proxy — key #3 (st-cc-003)
# 复制自 local-server-002 的 shangtang-cc-003，BASE_URL 改为 127.0.0.1 指向本机代理
# 修复: 用 script 强制分配 PTY，解决终端 TTY 丢失导致的滚动卡顿
API_TIMEOUT_MS=6000000 \
CLAUDE_CODE_MAX_RETRIES=1000000 \
NODE_TLS_REJECT_UNAUTHORIZED=0 \
ANTHROPIC_BASE_URL=http://127.0.0.1:54988 \
ANTHROPIC_API_KEY="2a00321b-b0c3-4ba5-9445-2ace27bf7c58" \
CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1 \
CLAUDE_CODE_MAX_OUTPUT_TOKENS=65536 \
ANTHROPIC_MODEL=deepseek-v4-flash \
exec script -q -c "claude --dangerously-skip-permissions $*" /dev/null
EOF
sudo chmod 755 /usr/local/bin/st-cc-003
```

- [ ] **Step 4: 创建 st-cc-004 — 客户端 key 3506726e（对应 config ed36d31d，带 PTY 垫层）**

```bash
sudo tee /usr/local/bin/st-cc-004 > /dev/null <<'EOF'
#!/bin/bash
# SenseNova (商汤日日新) deepseek-v4-flash via local proxy — key #4 (st-cc-004)
# 复制自 local-server-002 的 shangtang-cc-004，BASE_URL 改为 127.0.0.1 指向本机代理
# PTY wrapper (script) avoids scroll lag when TTY is lost.
API_TIMEOUT_MS=6000000 \
CLAUDE_CODE_MAX_RETRIES=1000000 \
NODE_TLS_REJECT_UNAUTHORIZED=0 \
ANTHROPIC_BASE_URL=http://127.0.0.1:54988 \
ANTHROPIC_API_KEY="3506726e-86a4-4446-ba7c-3dc896a88ffb" \
CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1 \
CLAUDE_CODE_MAX_OUTPUT_TOKENS=65536 \
ANTHROPIC_MODEL=deepseek-v4-flash \
exec script -q -c "claude --dangerously-skip-permissions $*" /dev/null
EOF
sudo chmod 755 /usr/local/bin/st-cc-004
```

- [ ] **Step 5: 创建 st-cc-005 — 客户端 key a7d5afae（对应 config 508da186，带 PTY 垫层）**

```bash
sudo tee /usr/local/bin/st-cc-005 > /dev/null <<'EOF'
#!/bin/bash
# SenseNova (商汤日日新) deepseek-v4-flash via local proxy — key #5 (st-cc-005)
# 复制自 local-server-002 的 shangtang-cc-005，BASE_URL 改为 127.0.0.1 指向本机代理
# PTY wrapper (script) avoids scroll lag when TTY is lost.
API_TIMEOUT_MS=6000000 \
CLAUDE_CODE_MAX_RETRIES=1000000 \
NODE_TLS_REJECT_UNAUTHORIZED=0 \
ANTHROPIC_BASE_URL=http://127.0.0.1:54988 \
ANTHROPIC_API_KEY="a7d5afae-a9ed-4498-8a12-c1304a29f393" \
CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1 \
CLAUDE_CODE_MAX_OUTPUT_TOKENS=65536 \
ANTHROPIC_MODEL=deepseek-v4-flash \
exec script -q -c "claude --dangerously-skip-permissions $*" /dev/null
EOF
sudo chmod 755 /usr/local/bin/st-cc-005
```

- [ ] **Step 6: 验证 5 个脚本已创建且可执行**
Run: `ls -la /usr/local/bin/st-cc-00* && for n in 001 002 003 004 005; do echo "--- st-cc-$n ---"; grep ANTHROPIC_API_KEY /usr/local/bin/st-cc-$n; done`
Expected:
  - Exit code: 0
  - `ls -la` 显示 5 个文件，权限 `-rwxr-xr-x`
  - 每个脚本的 `ANTHROPIC_API_KEY` 依次为 e5e86d37/b3f9a820/2a00321b/3506726e/a7d5afae 开头
  - 每个脚本的 `ANTHROPIC_BASE_URL=http://127.0.0.1:54988`

---

### Task 4: 验证配置生效与快捷指令可用

**Depends on:** Task 2, Task 3
**Files:** None（仅验证）

- [ ] **Step 1: 验证本机 54988 服务在运行**
Run: `ss -tlnp 2>/dev/null | grep 54988`
Expected:
  - Exit code: 0
  - Output contains: `54988` 且 users 含 `claude-with-ope`

- [ ] **Step 2: 验证本机 7890 代理在运行（商汤请求依赖）**
Run: `ss -tlnp 2>/dev/null | grep 7890`
Expected:
  - Exit code: 0
  - Output contains: `7890`

- [ ] **Step 3: 用 st-cc-001 的客户端 key 测试代理鉴权通过 — 发一个最小请求验证端到端链路**
Run: `curl -sS -X POST http://127.0.0.1:54988/v1/messages -H "Content-Type: application/json" -H "x-api-key: e5e86d37-0bd6-4af0-87e2-f76561fc0a74" -H "anthropic-version: 2023-06-01" -d '{"model":"deepseek-v4-flash","max_tokens":32,"messages":[{"role":"user","content":"reply with the single word: ok"}]}' | head -c 800`
Expected:
  - Exit code: 0
  - Output: JSON 响应，含 `"role":"assistant"` 与回复内容，或含 `"stop_reason"`
  - Output does NOT contain: `"authentication_error"` 或 `"Invalid API key"`（若出现说明 UUID 未匹配 config）

- [ ] **Step 4: 用 st-cc-005 的客户端 key 复测 — 确认 5 个 key 均可用（取首尾两个为代表）**
Run: `curl -sS -X POST http://127.0.0.1:54988/v1/messages -H "Content-Type: application/json" -H "x-api-key: a7d5afae-a9ed-4498-8a12-c1304a29f393" -H "anthropic-version: 2023-06-01" -d '{"model":"deepseek-v4-flash","max_tokens":32,"messages":[{"role":"user","content":"reply with the single word: ok"}]}' | head -c 800`
Expected:
  - Exit code: 0
  - Output: JSON 响应含 assistant 回复，无 authentication_error

- [ ] **Step 5: 确认 st-cc 脚本可被 shell 解析调用（dry-run，不实际启动交互式 claude）**
Run: `for n in 001 002 003 004 005; do command -v st-cc-$n; done`
Expected:
  - Exit code: 0
  - Output: 5 行，依次为 `/usr/local/bin/st-cc-001` ~ `/usr/local/bin/st-cc-005`
