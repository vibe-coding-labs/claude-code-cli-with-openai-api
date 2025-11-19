# ✅ Claude Code CLI 正确配置

根据官方文档，Claude Code CLI需要：
1. Anthropic Messages API 格式
2. Endpoints: `/v1/messages` 和 `/v1/messages/count_tokens`  
3. 必须转发headers: `anthropic-beta`, `anthropic-version`

## 🔍 问题分析

当前路由：`/proxy/:id/v1/messages`
当前配置：
```bash
ANTHROPIC_BASE_URL="http://localhost:10087/proxy/ff40e638-918a-4556-b3c5-4155d1cc4156"
ANTHROPIC_API_KEY="ff40e638-918a-4556-b3c5-4155d1cc4156"
```

**问题**：Claude CLI会在base URL后面自动添加 `/v1/messages`，所以实际请求的是：
```
http://localhost:10087/proxy/ff40e638-918a-4556-b3c5-4155d1cc4156/v1/messages
```
这个路径是正确的！但Claude CLI可能还在验证认证...

## 🎯 正确配置（不带/v1后缀）

```bash
export ANTHROPIC_BASE_URL="http://localhost:10087/proxy/ff40e638-918a-4556-b3c5-4155d1cc4156"
export ANTHROPIC_API_KEY="ff40e638-918a-4556-b3c5-4155d1cc4156"
```

Claude CLI会自动添加 `/v1/messages`，最终请求：
```
http://localhost:10087/proxy/ff40e638-918a-4556-b3c5-4155d1cc4156/v1/messages
```

## 测试
```bash
ANTHROPIC_BASE_URL="http://localhost:10087/proxy/ff40e638-918a-4556-b3c5-4155d1cc4156" \
ANTHROPIC_API_KEY="ff40e638-918a-4556-b3c5-4155d1cc4156" \
claude -p "say hello"
```
