# Anthropic Messages API 完整规范

## 参考文档
- https://docs.anthropic.com/en/api/messages
- https://github.com/Wei-Shaw/claude-relay-service
- https://github.com/teremterem/claude-code-gpt-5

## Request 字段

### 必需字段
- `model` (string): 模型名称
- `messages` (array): 对话消息数组
- `max_tokens` (integer): 最大生成 token 数

### 可选字段
- `system` (string|array): 系统提示词
- `temperature` (number): 温度参数 0-1
- `top_p` (number): Top-p 采样
- `top_k` (integer): Top-k 采样
- `stop_sequences` (array): 停止序列
- `stream` (boolean): 是否流式响应
- `metadata` (object): 元数据
  - `user_id` (string): 用户ID
- `tools` (array): 工具定义
- `tool_choice` (object): 工具选择策略
  - `type` (string): "auto" | "any" | "tool"
  - `name` (string): 工具名称（当 type="tool" 时）
- `context_management` (object): 上下文管理（Claude Code 2.x 特有）

### Beta 功能字段
- `thinking` (object): 思维链功能
  - `type` (string): "enabled"
  - `budget_tokens` (integer): 思维 token 预算

## Response 字段

### 非流式响应
```json
{
  "id": "msg_xxx",
  "type": "message",
  "role": "assistant",
  "model": "claude-3-5-sonnet-20241022",
  "content": [
    {
      "type": "text",
      "text": "回复内容"
    }
  ],
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 10,
    "output_tokens": 20,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0
  }
}
```

### 流式响应事件类型
1. `message_start` - 消息开始
2. `content_block_start` - 内容块开始
3. `content_block_delta` - 内容块增量
4. `content_block_stop` - 内容块结束
5. `message_delta` - 消息增量
6. `message_stop` - 消息结束
7. `ping` - 心跳（可选）
8. `error` - 错误

## 特殊请求处理

### 连接测试请求
Claude CLI 启动时会发送测试请求：
```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1,
  "messages": [
    {"role": "user", "content": "test"}
  ]
}
```
或
```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1,
  "messages": [
    {"role": "user", "content": "quota"}
  ]
}
```

需要特殊处理：
1. 识别 max_tokens=1 且 content 为 "test" 或 "quota"
2. 直接返回简单响应，不转发到上游
3. 确保响应格式正确

## 已知问题

### Claude Code 2.x 特性
1. 发送 `context_management` 参数 - 需要接受但不转发给 OpenAI
2. 发送连接测试请求 - 需要特殊处理
3. 在查询参数中添加 `?beta=true` - 需要支持

### 需要实现的功能
- [ ] 完整的 metadata 支持
- [ ] thinking 功能支持
- [ ] top_k 参数支持
- [ ] 缓存相关的 usage 字段
- [ ] ping 事件支持（流式）
- [ ] 工具调用的完整支持
