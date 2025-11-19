# 测试指南

## 测试环境变量配置

```bash
export ANTHROPIC_BASE_URL=http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76
export ANTHROPIC_API_KEY="test"
```

## 测试方法

### 1. 测试 -p 模式（单次命令）

```bash
claude -p '你好，介绍一下你自己'
```

### 2. 测试交互模式

**重要**：不要使用管道或重定向，直接运行：

```bash
claude
```

然后在交互界面中输入问题并回车。

### 3. 如果提示登录

如果仍然提示登录，检查：

1. 环境变量是否设置正确：
   ```bash
   echo $ANTHROPIC_BASE_URL
   echo $ANTHROPIC_API_KEY
   ```

2. 服务是否正常运行：
   ```bash
   curl http://localhost:8082/health
   ```

3. 查看服务日志中是否有错误

### 4. 已知问题

- ❌ **不要使用** `echo "text" | claude` - 这会导致 stdin 错误
- ✅ **推荐使用** `claude -p '你的问题'` - 单次命令模式
- ✅ **或直接运行** `claude` 然后在交互界面输入

## 修复内容

1. ✅ 添加 `context_management` 参数支持（Claude Code 2.x 会发送此参数）
2. ✅ 添加连接测试请求的特殊处理（max_tokens=1 的 test/quota 请求）
3. ✅ 修复流式响应结束时的 flush 问题
