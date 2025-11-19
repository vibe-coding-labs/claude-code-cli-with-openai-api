# Claude CLI 交互模式测试指南

## ⚠️ 重要提示

**不要通过管道或脚本测试交互模式！** Claude CLI 需要真实的 TTY 环境。

## 正确的测试方法

### 1. 在新终端窗口中设置环境变量

```bash
export ANTHROPIC_BASE_URL="http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76"
export ANTHROPIC_API_KEY="test"
```

### 2. 确认代理服务器正在运行

```bash
curl http://localhost:8082/health
```

应该返回：
```json
{
  "status": "healthy",
  "timestamp": "..."
}
```

### 3. 测试 -p 模式（不需要 TTY）

```bash
claude -p "hello"
```

如果这个成功，说明连接和认证都是正常的。

### 4. 启动交互模式

**在真实终端中直接运行：**
```bash
claude
```

**不要这样做：**
```bash
# ❌ 错误 - 会导致 "Raw mode not supported" 错误
echo "hi" | claude

# ❌ 错误 - 同样会失败
claude <<< "hi"

# ❌ 错误 - 脚本中运行也会失败
./script.sh
```

## 如果看到 "Checking connectivity..." 然后提示登录

### 可能的原因和解决方案

#### 1. 环境变量未正确设置

检查：
```bash
echo $ANTHROPIC_BASE_URL
echo $ANTHROPIC_API_KEY
```

如果为空，重新设置。

#### 2. Claude CLI 缓存了旧的登录状态

```bash
claude logout
```

然后重新设置环境变量并再次运行。

#### 3. API Key 不匹配

检查你的配置ID对应的 API Key：
```bash
curl http://localhost:8082/api/v1/configs
```

确保使用的 `ANTHROPIC_API_KEY` 与配置中的 `anthropic_api_key` 匹配。

#### 4. 检查服务器日志

当你运行 `claude` 时，查看服务器终端的日志：

**成功的日志应该显示：**
```
📥 [Request Details]
   Model: claude-*
   MaxTokens: 32000
   Stream: true
   ...
✅ [Streaming] Stream completed
```

**如果没有任何请求日志，说明：**
- 环境变量设置错误
- 或者 Claude CLI 根本没有尝试连接到代理

## 验证连接测试端点

手动测试连接测试端点：

```bash
curl -X POST http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1,
    "messages": [{"role": "user", "content": "test"}]
  }'
```

应该返回：
```json
{
  "id": "msg_test_...",
  "type": "message",
  "role": "assistant",
  "model": "claude-3-5-sonnet-20241022",
  "content": [{"type": "text", "text": "OK"}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 1, "output_tokens": 1}
}
```

## 已知的 Claude CLI 行为

从日志中可以看到 Claude CLI 在交互模式下的实际行为：

1. **首次启动** - 发送多个测试请求（haiku, sonnet 模型）
2. **工具加载** - 请求包含 16 个工具定义
3. **上下文管理** - 使用 `context_management` 参数
4. **Beta 功能** - URL 包含 `?beta=true`
5. **元数据传递** - 包含 session 信息

所有这些都已被代理服务器正确处理！

## 调试清单

如果交互模式仍然有问题：

- [ ] 代理服务器正在运行？
- [ ] 环境变量正确设置？（检查 echo $ANTHROPIC_BASE_URL）
- [ ] -p 模式可以工作？
- [ ] 已经运行 `claude logout`？
- [ ] 在**真实终端**（不是管道/脚本）中测试？
- [ ] 服务器日志显示收到请求？
- [ ] API Key 匹配配置？

## 成功标志

当交互模式正常工作时，你应该看到：

1. **终端中：** Claude CLI 的欢迎界面和提示符
2. **服务器日志：** 多个成功的 POST 请求（200 状态码）
3. **能够输入并获得响应**

## 示例：完整的测试流程

```bash
# 终端 1 - 启动代理服务器
cd /path/to/project
make run

# 终端 2 - 测试 Claude CLI
export ANTHROPIC_BASE_URL="http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76"
export ANTHROPIC_API_KEY="test"

# 确保没有旧的登录状态
claude logout

# 测试 -p 模式
claude -p "测试"

# 如果上面成功，启动交互模式
claude
```

---

**记住：交互模式必须在真实的 TTY 终端中运行，不能通过管道或脚本！**
