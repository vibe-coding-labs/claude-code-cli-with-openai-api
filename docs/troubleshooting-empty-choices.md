# 问题诊断：API returned empty choices

## 问题描述
Claude CLI 频繁遇到错误：`API returned empty choices after 4 attempts`

## 可能的原因

### 1. OpenAI API 提供商问题
某些 OpenAI API 兼容服务（如 iFlow、智谱等）可能会在某些情况下返回空的 choices 数组：

- **请求过载**：API 提供商服务器过载
- **内容过滤**：请求或响应被内容过滤器拦截
- **Token 限制**：超过 API 配额或限制
- **模型问题**：模型暂时不可用或正在维护

### 2. 请求参数问题
- **max_tokens 设置不当**：设置为 0 或过低
- **temperature 参数**：某些极端值可能导致无输出
- **模型名称错误**：使用了不存在的模型

### 3. 网络问题
- **超时**：请求超时导致响应不完整
- **连接不稳定**：网络波动导致数据传输不完整

## 诊断步骤

### 1. 查看详细日志
运行服务时设置日志级别为 DEBUG：
```bash
LOG_LEVEL=DEBUG ./claude-with-openai-api server
```

### 2. 检查配置
确认以下配置项：
- `max_tokens_limit`: 建议设置为 128000 或更高
- `request_timeout`: 建议至少 300 秒
- `retry_count`: 建议设置为 3-5 次

### 3. 测试 API 直接请求
使用 curl 直接测试 OpenAI API：
```bash
curl -X POST https://apis.iflow.cn/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "glm-4.6",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 2000
  }'
```

查看响应是否包含空的 choices 数组。

### 4. 监控重试次数
如果经常需要重试 3-4 次才成功，说明 API 提供商不稳定。

## 解决方案

### 临时方案
1. **增加重试次数**：将 `retry_count` 设置为 5-10
2. **延长超时时间**：将 `request_timeout` 设置为 600 秒
3. **降低并发**：避免同时发起多个请求

### 长期方案
1. **更换 API 提供商**：选择更稳定的服务
2. **实现请求队列**：限制并发请求数
3. **添加熔断机制**：在连续失败后暂停一段时间

### 代码层面改进（已实施）
1. ✅ 增强日志记录：记录完整响应 JSON
2. ✅ 捕获 API 错误字段：检查响应中的 error 字段
3. ✅ 提供更详细的错误信息

## 配置建议

针对 iFlow glm-4.6 等 API：
```
max_tokens_limit: 128000
request_timeout: 300
retry_count: 5
```

## 监控建议

1. 记录每次请求的重试次数
2. 统计空 choices 错误的发生频率
3. 监控 API 响应时间

## 参考
- OpenAI API 文档：https://platform.openai.com/docs/api-reference
- Claude CLI 文档：https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api
